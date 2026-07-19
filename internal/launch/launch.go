// Package launch builds and fires per-store, per-OS game-launch commands.
//
// Command is the pure, table-testable core: given a Target it returns the
// working directory and exact argv for the store's canonical launch verb
// (steam://rungameid, Galaxy /command=runGame, the Epic launcher URL, a
// direct DRM-free exe, or the user's manual template). Launch builds the
// command and hands it to the injected Runner; the platform default
// (spawn_<goos>.go) spawns games detached (Start + Process.Release, never
// Wait) and waits on URL openers under a 10s cap so failures surface.
//
// The package never constructs proton/wine/umu-run invocations itself —
// umu-run appears in argv only when the user's manual Template says so —
// and never routes through a shell: argv is exec'd directly.
package launch

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// Sentinel errors for unlaunchable targets; wrapped with %w.
var (
	ErrNoStore        = errors.New("unknown or unsupported store")
	ErrMissingExe     = errors.New("no executable path")
	ErrMissingAppID   = errors.New("no store app id")
	ErrMissingAppName = errors.New("no epic app name (from .item manifest)")
)

// Store identifies the storefront a game is launched through.
type Store int

const (
	StoreUnknown Store = iota
	StoreSteam
	StoreGOG
	StoreEpic
	StoreManual
)

// String names the store for logs.
func (s Store) String() string {
	switch s {
	case StoreSteam:
		return "steam"
	case StoreGOG:
		return "gog"
	case StoreEpic:
		return "epic"
	case StoreManual:
		return "manual"
	default:
		return "unknown"
	}
}

// Target describes one launchable game. Which fields matter depends on
// Store: Steam needs AppID; GOG needs ExePath (AppID + Dir enable the
// optional Windows Galaxy form); Epic needs AppName (ExePath is the
// fallback); Manual needs ExePath and honors Template.
type Target struct {
	Store    Store
	Name     string   // display name, logs only
	AppID    string   // steam appid / gog galaxy game id
	AppName  string   // epic catalog app name (from .item manifest, not display name)
	ExePath  string   // direct executable (gog/manual canonical, epic fallback)
	Dir      string   // game dir (gog galaxy /path, manual {dir})
	Args     []string // user launch arguments
	Template string   // manual store only; default `"{exe}" {args}`
}

// Runner executes one built command. dir is the working directory, name the
// binary, args its argv tail. The platform default spawns games detached.
type Runner func(ctx context.Context, dir, name string, args ...string) error

// Launcher builds launch commands for one GOOS and fires them via its
// Runner. Construct with New; the zero value is not usable.
type Launcher struct {
	run      Runner
	goos     string
	lookPath func(string) (string, error)
}

// New returns a Launcher. Nil r selects the platform detached-spawn runner,
// empty goos selects runtime.GOOS, nil lookPath selects exec.LookPath.
func New(r Runner, goos string, lookPath func(string) (string, error)) *Launcher {
	if r == nil {
		r = platformRunner()
	}
	if goos == "" {
		goos = runtime.GOOS
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	return &Launcher{run: r, goos: goos, lookPath: lookPath}
}

// Command builds the launch command for t. It is pure: no processes are
// spawned and the only external consultation is lookPath for binary
// fallback chains. Returns the working directory and exact argv.
func (l *Launcher) Command(t Target) (dir string, argv []string, err error) {
	switch t.Store {
	case StoreSteam:
		return l.steamCommand(t)
	case StoreGOG:
		return l.gogCommand(t)
	case StoreEpic:
		return l.epicCommand(t)
	case StoreManual:
		return manualCommand(t)
	default:
		return "", nil, fmt.Errorf("launch %q: %w (store=%d)", t.Name, ErrNoStore, int(t.Store))
	}
}

// urlOpenTimeout caps how long Launch waits on URL openers (xdg-open, open,
// rundll32). They return promptly, so waiting surfaces failures; games
// themselves are spawned detached and never waited on.
const urlOpenTimeout = 10 * time.Second

// Launch builds the command for t and fires it through the Runner. A
// successful return means the launch was requested — a detached spawn
// proves nothing about the game actually running.
func (l *Launcher) Launch(ctx context.Context, t Target) error {
	dir, argv, err := l.Command(t)
	if err != nil {
		return err
	}
	if isURLOpener(argv[0]) {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, urlOpenTimeout)
		defer cancel()
	}
	log.Info().Str("store", t.Store.String()).Str("game", t.Name).
		Str("bin", argv[0]).Int("argc", len(argv)-1).Msg("launch requested")
	return l.run(ctx, dir, argv[0], argv[1:]...)
}

// isURLOpener reports whether name is a fire-and-return URL opener rather
// than a game process: xdg-open (linux), open (darwin), rundll32 (windows).
func isURLOpener(name string) bool {
	switch strings.ToLower(filepath.Base(name)) {
	case "xdg-open", "open", "rundll32", "rundll32.exe":
		return true
	}
	return false
}

// steamURL is the fire-and-forget launch verb; it auto-respects the game's
// configured Proton. The run form carries user args.
func steamURL(appID string, args []string) string {
	if len(args) == 0 {
		return "steam://rungameid/" + appID
	}
	return "steam://run/" + appID + "//" + strings.Join(args, " ") + "/"
}

func (l *Launcher) steamCommand(t Target) (string, []string, error) {
	if t.AppID == "" {
		return "", nil, fmt.Errorf("launch %q: %w", t.Name, ErrMissingAppID)
	}
	switch l.goos {
	case "linux":
		// Native steam can take -applaunch; flatpak and xdg-open only speak URLs.
		if _, err := l.lookPath("steam"); err == nil {
			if len(t.Args) > 0 {
				return "", append([]string{"steam", "-applaunch", t.AppID}, t.Args...), nil
			}
			return "", []string{"steam", steamURL(t.AppID, nil)}, nil
		}
		if _, err := l.lookPath("flatpak"); err == nil {
			return "", []string{"flatpak", "run", "com.valvesoftware.Steam", steamURL(t.AppID, t.Args)}, nil
		}
		return "", []string{"xdg-open", steamURL(t.AppID, t.Args)}, nil
	case "windows":
		if _, err := l.lookPath("steam.exe"); err == nil {
			return "", append([]string{"steam.exe", "-applaunch", t.AppID}, t.Args...), nil
		}
		return "", []string{"rundll32", "url.dll,FileProtocolHandler", steamURL(t.AppID, t.Args)}, nil
	default: // darwin and anything else: open handles steam:// URLs
		return "", []string{"open", steamURL(t.AppID, t.Args)}, nil
	}
}

func (l *Launcher) gogCommand(t Target) (string, []string, error) {
	// GOG games are DRM-free: the direct exe is canonical on every OS.
	if t.ExePath != "" {
		return filepath.Dir(t.ExePath), append([]string{t.ExePath}, t.Args...), nil
	}
	// Optional Galaxy form (windows only). goggalaxy://openGameView is a
	// library view, not a launch verb — never emitted.
	if l.goos == "windows" && t.AppID != "" {
		return t.Dir, []string{"GalaxyClient.exe", "/command=runGame", "/gameId=" + t.AppID, "/path=" + t.Dir}, nil
	}
	return "", nil, fmt.Errorf("launch %q: %w", t.Name, ErrMissingExe)
}

func (l *Launcher) epicCommand(t Target) (string, []string, error) {
	if t.AppName == "" {
		// Best-effort fallback: run the exe directly.
		if t.ExePath != "" {
			return filepath.Dir(t.ExePath), append([]string{t.ExePath}, t.Args...), nil
		}
		return "", nil, fmt.Errorf("launch %q: %w", t.Name, ErrMissingAppName)
	}
	url := "com.epicgames.launcher://apps/" + t.AppName + "?action=launch&silent=true"
	switch l.goos {
	case "windows":
		return "", []string{"rundll32", "url.dll,FileProtocolHandler", url}, nil
	case "darwin":
		return "", []string{"open", url}, nil
	default: // linux: best-effort via the desktop's URL handler
		return "", []string{"xdg-open", url}, nil
	}
}

// defaultTemplate runs the game exe with its args, nothing else.
const defaultTemplate = `"{exe}" {args}`

func manualCommand(t Target) (string, []string, error) {
	if t.ExePath == "" {
		return "", nil, fmt.Errorf("launch %q: %w", t.Name, ErrMissingExe)
	}
	tmpl := t.Template
	if tmpl == "" {
		tmpl = defaultTemplate
	}
	dir := t.Dir
	if dir == "" {
		dir = filepath.Dir(t.ExePath)
	}
	repl := map[string]string{
		"{exe}":   t.ExePath,
		"{dir}":   dir,
		"{appid}": t.AppID,
	}
	var argv []string
	for _, tok := range splitQuoted(tmpl) {
		if tok == "{args}" {
			// Each user arg is spliced as its own argv element, verbatim.
			argv = append(argv, t.Args...)
			continue
		}
		if strings.Contains(tok, "{args}") {
			tok = strings.ReplaceAll(tok, "{args}", strings.Join(t.Args, " "))
		}
		for ph, val := range repl {
			tok = strings.ReplaceAll(tok, ph, val)
		}
		argv = append(argv, tok)
	}
	if len(argv) == 0 {
		return "", nil, fmt.Errorf("launch %q: template %q produced an empty command", t.Name, tmpl)
	}
	return filepath.Dir(t.ExePath), argv, nil
}

// splitQuoted splits a template on whitespace; double quotes group tokens
// and are stripped. There is no escape handling, no variable expansion, no
// globbing — metacharacters pass through as literal argv text.
func splitQuoted(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote, hasCur := false, false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			hasCur = true
		case (r == ' ' || r == '\t' || r == '\n') && !inQuote:
			if hasCur {
				tokens = append(tokens, cur.String())
				cur.Reset()
				hasCur = false
			}
		default:
			cur.WriteRune(r)
			hasCur = true
		}
	}
	if hasCur {
		tokens = append(tokens, cur.String())
	}
	return tokens
}
