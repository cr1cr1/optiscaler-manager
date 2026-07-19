// Package launch_test locks the public contract of internal/launch:
// the per-store per-OS command table (pure, exact argv), the steam
// fallback chain, manual-template splitting, and detached launch wiring.
package launch_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/launch"
)

// lookPathStub returns a lookPath func succeeding only for the given names.
func lookPathStub(found ...string) func(string) (string, error) {
	set := map[string]bool{}
	for _, f := range found {
		set[f] = true
	}
	return func(name string) (string, error) {
		if set[name] {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found: " + name)
	}
}

const epicURL = "com.epicgames.launcher://apps/Fortnite?action=launch&silent=true"

func TestCommandTable_AllStoresAllOS(t *testing.T) {
	steam := launch.Target{Store: launch.StoreSteam, Name: "Half-Life", AppID: "70"}
	steamArgs := launch.Target{Store: launch.StoreSteam, Name: "Half-Life", AppID: "70", Args: []string{"-novid", "+console"}}
	gog := launch.Target{Store: launch.StoreGOG, Name: "Cyberpunk", ExePath: "/games/cp77/bin/x64/cyberpunk.exe", Args: []string{"-skipStartScreen"}}
	gogGalaxy := launch.Target{Store: launch.StoreGOG, Name: "Cyberpunk", AppID: "1423049311", Dir: "/games/cp77"}
	epic := launch.Target{Store: launch.StoreEpic, Name: "Fortnite", AppName: "Fortnite"}
	epicExe := launch.Target{Store: launch.StoreEpic, Name: "Fortnite", ExePath: "/games/fn/Fortnite.exe", Args: []string{"-epicapp=fn"}}
	manual := launch.Target{Store: launch.StoreManual, Name: "Modded", ExePath: "/games/mod/game.exe", Args: []string{"--foo"}, Template: `umu-run "{exe}" {args}`}

	tests := []struct {
		name     string
		goos     string
		found    []string // binaries visible to lookPath
		target   launch.Target
		wantDir  string
		wantArgv []string
	}{
		// --- Steam ---
		{"steam linux url", "linux", []string{"steam"}, steam, "", []string{"steam", "steam://rungameid/70"}},
		{"steam linux args", "linux", []string{"steam"}, steamArgs, "", []string{"steam", "-applaunch", "70", "-novid", "+console"}},
		{"steam windows", "windows", []string{"steam.exe"}, steam, "", []string{"steam.exe", "-applaunch", "70"}},
		{"steam windows args", "windows", []string{"steam.exe"}, steamArgs, "", []string{"steam.exe", "-applaunch", "70", "-novid", "+console"}},
		{"steam darwin", "darwin", nil, steam, "", []string{"open", "steam://rungameid/70"}},
		{"steam darwin args", "darwin", nil, steamArgs, "", []string{"open", "steam://run/70//-novid +console/"}},
		// --- GOG: direct exe is canonical on every OS ---
		{"gog linux", "linux", nil, gog, "/games/cp77/bin/x64", []string{"/games/cp77/bin/x64/cyberpunk.exe", "-skipStartScreen"}},
		{"gog windows", "windows", nil, gog, "/games/cp77/bin/x64", []string{"/games/cp77/bin/x64/cyberpunk.exe", "-skipStartScreen"}},
		{"gog darwin", "darwin", nil, gog, "/games/cp77/bin/x64", []string{"/games/cp77/bin/x64/cyberpunk.exe", "-skipStartScreen"}},
		{"gog windows galaxy fallback", "windows", nil, gogGalaxy, "/games/cp77",
			[]string{"GalaxyClient.exe", "/command=runGame", "/gameId=1423049311", "/path=/games/cp77"}},
		// --- Epic: launcher URL; exe fallback when AppName empty ---
		{"epic linux", "linux", nil, epic, "", []string{"xdg-open", epicURL}},
		{"epic windows", "windows", nil, epic, "", []string{"rundll32", "url.dll,FileProtocolHandler", epicURL}},
		{"epic darwin", "darwin", nil, epic, "", []string{"open", epicURL}},
		{"epic linux exe fallback", "linux", nil, epicExe, "/games/fn", []string{"/games/fn/Fortnite.exe", "-epicapp=fn"}},
		{"epic windows exe fallback", "windows", nil, epicExe, "/games/fn", []string{"/games/fn/Fortnite.exe", "-epicapp=fn"}},
		{"epic darwin exe fallback", "darwin", nil, epicExe, "/games/fn", []string{"/games/fn/Fortnite.exe", "-epicapp=fn"}},
		// --- Manual: template expansion (umu-run only because the user says so) ---
		{"manual linux umu template", "linux", nil, manual, "/games/mod", []string{"umu-run", "/games/mod/game.exe", "--foo"}},
		{"manual windows umu template", "windows", nil, manual, "/games/mod", []string{"umu-run", "/games/mod/game.exe", "--foo"}},
		{"manual darwin umu template", "darwin", nil, manual, "/games/mod", []string{"umu-run", "/games/mod/game.exe", "--foo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := launch.New(nil, tt.goos, lookPathStub(tt.found...))
			dir, argv, err := l.Command(tt.target)
			if err != nil {
				t.Fatalf("Command: %v", err)
			}
			if !reflect.DeepEqual(argv, tt.wantArgv) {
				t.Errorf("argv = %q, want %q", argv, tt.wantArgv)
			}
			if dir != tt.wantDir {
				t.Errorf("dir = %q, want %q", dir, tt.wantDir)
			}
		})
	}
}

func TestCommandFallbackChain(t *testing.T) {
	target := launch.Target{Store: launch.StoreSteam, Name: "Half-Life", AppID: "70"}
	tests := []struct {
		name     string
		found    []string
		wantArgv []string
	}{
		{"native steam wins", []string{"steam", "flatpak", "xdg-open"}, []string{"steam", "steam://rungameid/70"}},
		{"no steam -> flatpak", []string{"flatpak"}, []string{"flatpak", "run", "com.valvesoftware.Steam", "steam://rungameid/70"}},
		{"no steam no flatpak -> xdg-open", nil, []string{"xdg-open", "steam://rungameid/70"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := launch.New(nil, "linux", lookPathStub(tt.found...))
			_, argv, err := l.Command(target)
			if err != nil {
				t.Fatalf("Command: %v", err)
			}
			if !reflect.DeepEqual(argv, tt.wantArgv) {
				t.Errorf("argv = %q, want %q", argv, tt.wantArgv)
			}
		})
	}

	t.Run("flatpak args form", func(t *testing.T) {
		l := launch.New(nil, "linux", lookPathStub("flatpak"))
		_, argv, err := l.Command(launch.Target{Store: launch.StoreSteam, AppID: "70", Args: []string{"-novid"}})
		if err != nil {
			t.Fatalf("Command: %v", err)
		}
		want := []string{"flatpak", "run", "com.valvesoftware.Steam", "steam://run/70//-novid/"}
		if !reflect.DeepEqual(argv, want) {
			t.Errorf("argv = %q, want %q", argv, want)
		}
	})

	t.Run("windows without steam.exe -> rundll32", func(t *testing.T) {
		l := launch.New(nil, "windows", lookPathStub())
		_, argv, err := l.Command(target)
		if err != nil {
			t.Fatalf("Command: %v", err)
		}
		want := []string{"rundll32", "url.dll,FileProtocolHandler", "steam://rungameid/70"}
		if !reflect.DeepEqual(argv, want) {
			t.Errorf("argv = %q, want %q", argv, want)
		}
	})
}

func TestTemplateSplitAndPlaceholders(t *testing.T) {
	target := launch.Target{
		Store: launch.StoreManual, Name: "Modded", AppID: "42",
		ExePath: "/games/mod/my game.exe", Dir: "/games/mod",
		Args: []string{"--res=4k", "--hdr"},
	}
	tests := []struct {
		name     string
		template string
		wantArgv []string
	}{
		{"default template", "", []string{"/games/mod/my game.exe", "--res=4k", "--hdr"}},
		{"explicit default", `"{exe}" {args}`, []string{"/games/mod/my game.exe", "--res=4k", "--hdr"}},
		{"prefix binary", `umu-run "{exe}" {args}`, []string{"umu-run", "/games/mod/my game.exe", "--res=4k", "--hdr"}},
		{"quoted wrapper pairs", `gamemoderun "mangohud" "{exe}"`, []string{"gamemoderun", "mangohud", "/games/mod/my game.exe"}},
		{"all placeholders", `run --id={appid} --cwd="{dir}" "{exe}" {args}`, []string{"run", "--id=42", "--cwd=/games/mod", "/games/mod/my game.exe", "--res=4k", "--hdr"}},
		{"no args placeholder", `"{exe}" --fixed`, []string{"/games/mod/my game.exe", "--fixed"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tg := target
			tg.Template = tt.template
			l := launch.New(nil, "linux", lookPathStub())
			_, argv, err := l.Command(tg)
			if err != nil {
				t.Fatalf("Command: %v", err)
			}
			if !reflect.DeepEqual(argv, tt.wantArgv) {
				t.Errorf("argv = %q, want %q", argv, tt.wantArgv)
			}
		})
	}
}

func TestTemplateNoShellExpansion(t *testing.T) {
	// Shell metacharacters must survive verbatim as literal argv elements:
	// no sh -c, no glob, no command substitution, no variable expansion.
	target := launch.Target{
		Store: launch.StoreManual, Name: "Evil", ExePath: "/games/e/game.exe",
		Args:     []string{"$(touch /tmp/pwned)", "*", "; rm -rf /", "$HOME"},
		Template: `env FOO=$BAR "{exe}" {args} ; echo done`,
	}
	l := launch.New(nil, "linux", lookPathStub())
	_, argv, err := l.Command(target)
	if err != nil {
		t.Fatalf("Command: %v", err)
	}
	want := []string{"env", "FOO=$BAR", "/games/e/game.exe", "$(touch /tmp/pwned)", "*", "; rm -rf /", "$HOME", ";", "echo", "done"}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %q, want %q", argv, want)
	}
	for _, a := range argv {
		if a == "sh" || a == "-c" || a == "/bin/sh" {
			t.Errorf("argv contains shell invocation: %q", argv)
		}
	}
}

func TestLaunchCapturesArgvViaInjectedRunner(t *testing.T) {
	target := launch.Target{Store: launch.StoreGOG, Name: "Cyberpunk", ExePath: "/games/cp77/cyberpunk.exe", Args: []string{"-skipStartScreen"}}
	var gotDir, gotName string
	var gotArgs []string
	runner := func(_ context.Context, dir, name string, args ...string) error {
		gotDir, gotName, gotArgs = dir, name, args
		return nil
	}
	l := launch.New(runner, "linux", lookPathStub())
	if err := l.Launch(context.Background(), target); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if gotName != "/games/cp77/cyberpunk.exe" {
		t.Errorf("name = %q, want exe path", gotName)
	}
	if !reflect.DeepEqual(gotArgs, []string{"-skipStartScreen"}) {
		t.Errorf("args = %q", gotArgs)
	}
	if gotDir != "/games/cp77" {
		t.Errorf("dir = %q, want /games/cp77", gotDir)
	}
	t.Log("injected runner captured exact argv; no process was spawned")
}

func TestLaunchURLTimeout(t *testing.T) {
	// URL openers (xdg-open/open/rundll32) return promptly, so Launch caps
	// the context at 10s and waits — failures must surface, not hang.
	target := launch.Target{Store: launch.StoreEpic, Name: "Fortnite", AppName: "Fortnite"}
	var deadline time.Time
	var hasDeadline bool
	runner := func(ctx context.Context, _, _ string, _ ...string) error {
		deadline, hasDeadline = ctx.Deadline()
		return nil
	}
	l := launch.New(runner, "linux", lookPathStub())
	start := time.Now()
	if err := l.Launch(context.Background(), target); err != nil {
		t.Fatalf("Launch: %v", err)
	}
	if !hasDeadline {
		t.Fatal("URL opener context has no deadline; want 10s cap")
	}
	if d := time.Until(deadline); d > 10*time.Second || d <= 0 {
		t.Errorf("deadline %v from start is outside (0, 10s]", d)
	}
	if deadline.Before(start.Add(9 * time.Second)) {
		t.Errorf("deadline %v suspiciously early vs start %v", deadline, start)
	}
	t.Logf("URL opener capped at %v", deadline.Sub(start).Round(time.Millisecond))
}

func TestCommandErrors(t *testing.T) {
	tests := []struct {
		name   string
		goos   string
		target launch.Target
		want   error
	}{
		{"unknown store", "linux", launch.Target{Store: launch.StoreUnknown, Name: "x"}, launch.ErrNoStore},
		{"steam missing appid", "linux", launch.Target{Store: launch.StoreSteam, Name: "x"}, launch.ErrMissingAppID},
		{"gog missing exe", "linux", launch.Target{Store: launch.StoreGOG, Name: "x"}, launch.ErrMissingExe},
		{"gog windows galaxy missing appid", "windows", launch.Target{Store: launch.StoreGOG, Name: "x"}, launch.ErrMissingExe},
		{"epic missing appname and exe", "linux", launch.Target{Store: launch.StoreEpic, Name: "x"}, launch.ErrMissingAppName},
		{"manual missing exe", "linux", launch.Target{Store: launch.StoreManual, Name: "x"}, launch.ErrMissingExe},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := launch.New(nil, tt.goos, lookPathStub("steam"))
			_, _, err := l.Command(tt.target)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want errors.Is %v", err, tt.want)
			}
		})
	}
}
