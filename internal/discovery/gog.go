package discovery

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/gid"
)

// GOG metadata types and their parser moved to internal/gid (v0.8); the
// aliases keep this package's API stable while letting gid stay below
// discovery in the layering.
type GOGGameInfo = gid.GOGGameInfo
type GOGPlayTask = gid.GOGPlayTask

var ParseGOGGameInfo = gid.ParseGOGGameInfo

// GOGExePath locates the goggame-*.info file inside gameDir and resolves the
// game's primary executable to an absolute path that exists on disk.
// Windows-style separators in task paths are normalised. "" when no info
// file or executable can be resolved.
func GOGExePath(gameDir string) string {
	infos, err := filepath.Glob(filepath.Join(gameDir, "goggame-*.info"))
	if err != nil || len(infos) == 0 {
		return ""
	}
	f, err := os.Open(infos[0])
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	info, err := ParseGOGGameInfo(f)
	if err != nil {
		log.Debug().Err(err).Str("info", infos[0]).Msg("unparseable goggame info")
		return ""
	}
	rel := info.PrimaryExe()
	if rel == "" {
		return ""
	}
	rel = strings.ReplaceAll(rel, `\`, string(filepath.Separator))
	primary, ok := joinWithin(gameDir, rel)
	if !ok {
		log.Warn().Str("dir", gameDir).Str("path", rel).Msg("goggame task path escapes game dir, rejected")
		return ""
	}
	candidates := []string{primary}
	for _, pt := range info.PlayTasks {
		if pt.WorkingDir != "" && pt.WorkingDir != "." {
			wd := strings.ReplaceAll(pt.WorkingDir, `\`, string(filepath.Separator))
			if c, ok := joinWithin(gameDir, filepath.Join(wd, filepath.Base(rel))); ok {
				candidates = append(candidates, c)
			}
		}
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	log.Debug().Str("dir", gameDir).Str("rel", rel).Msg("goggame primary exe not found on disk")
	return ""
}

// joinWithin joins rel onto root and reports whether the cleaned result
// stays inside root. goggame info files are third-party input: absolute
// paths, volume names, and `..` breakouts are rejected, never resolved.
func joinWithin(root, rel string) (string, bool) {
	if rel == "" || filepath.IsAbs(rel) || filepath.VolumeName(rel) != "" {
		return "", false
	}
	root = filepath.Clean(root)
	j := filepath.Join(root, rel)
	if j != root && !strings.HasPrefix(j, root+string(os.PathSeparator)) {
		return "", false
	}
	return j, true
}
