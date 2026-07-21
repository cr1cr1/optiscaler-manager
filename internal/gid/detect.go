// Package gid identifies games from hard evidence inside their install
// directories: store id files (steam_appid.txt), store metadata
// (goggame-*.info, .egstore manifests), and engine metadata (Unity
// app.info). It is the offline half of the v0.8 identification stack —
// online canonical resolution lives in the enrich phase. gid sits below
// discovery: it must never import it.
package gid

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// Result is what offline detection found in a game directory: a Steam app
// id when one is recorded (name resolution is the enrich phase's job, so
// an appid alone never sets Title), and the title when an in-dir metadata
// file yields one directly.
type Result struct {
	SteamAppID string
	Title      string
	Source     domain.TitleSource
}

// Detect scans dir for identity files and returns the best evidence.
// Title precedence: goggame-*.info > .egstore manifest > Unity app.info
// (a steam_appid.txt is orthogonal and always reported). exe may be ""
// and only breaks ties between multiple Unity *_Data dirs.
func Detect(dir, exe string) Result {
	r := Result{SteamAppID: findSteamAppID(dir)}
	if name := gogName(dir); name != "" {
		r.Title, r.Source = name, domain.SourceGOGInfo
		return r
	}
	if name := egstoreName(dir); name != "" {
		r.Title, r.Source = name, domain.SourceEGStore
		return r
	}
	if name := unityName(dir, exe); name != "" {
		r.Title, r.Source = name, domain.SourceUnity
		return r
	}
	return r
}

// findSteamAppID returns the app id recorded in a steam_appid.txt at the
// root or up to two levels below (repack tooling nests them under
// steam_settings/). Shallowest file wins, lexicographic order breaks ties;
// hidden dirs are skipped. The test/demo appid 480 and anything that is
// not a plain positive integer are rejected.
func findSteamAppID(dir string) string {
	type candidate struct {
		depth int
		path  string
	}
	var found []candidate
	if st, err := os.Stat(filepath.Join(dir, "steam_appid.txt")); err == nil && st.Mode().IsRegular() {
		found = append(found, candidate{0, filepath.Join(dir, "steam_appid.txt")})
	}
	l1, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e1 := range l1 {
		if !e1.IsDir() || strings.HasPrefix(e1.Name(), ".") {
			continue
		}
		d1 := filepath.Join(dir, e1.Name())
		if st, err := os.Stat(filepath.Join(d1, "steam_appid.txt")); err == nil && st.Mode().IsRegular() {
			found = append(found, candidate{1, filepath.Join(d1, "steam_appid.txt")})
		}
		l2, err := os.ReadDir(d1)
		if err != nil {
			continue
		}
		for _, e2 := range l2 {
			if !e2.IsDir() || strings.HasPrefix(e2.Name(), ".") {
				continue
			}
			d2 := filepath.Join(d1, e2.Name())
			if st, err := os.Stat(filepath.Join(d2, "steam_appid.txt")); err == nil && st.Mode().IsRegular() {
				found = append(found, candidate{2, filepath.Join(d2, "steam_appid.txt")})
			}
		}
	}
	if len(found) == 0 {
		return ""
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].depth != found[j].depth {
			return found[i].depth < found[j].depth
		}
		return found[i].path < found[j].path
	})
	return readAppID(found[0].path)
}

// readAppID parses the first line of a steam_appid.txt: digits only after
// trimming BOM/whitespace/CR; 480 (Steam's test app) is not a real id.
func readAppID(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	line := string(data)
	if i := strings.IndexAny(line, "\r\n"); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimPrefix(line, "\xef\xbb\xbf")
	line = strings.TrimSpace(line)
	line = strings.TrimLeft(line, "0")
	if line == "" || line == "480" {
		return ""
	}
	for _, r := range line {
		if r < '0' || r > '9' {
			return ""
		}
	}
	return line
}

// gogName returns the game name from the first goggame-*.info at the root.
func gogName(dir string) string {
	matches, err := filepath.Glob(filepath.Join(dir, "goggame-*.info"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	f, err := os.Open(matches[0])
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	info, err := ParseGOGGameInfo(f)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(info.Name)
}

// egstoreName returns the DisplayName of a game manifest under .egstore,
// but only when its InstallLocation points at this very directory — the
// marker survives uninstalls, so a stale manifest is not evidence.
func egstoreName(dir string) string {
	matches, err := filepath.Glob(filepath.Join(dir, ".egstore", "*.manifest"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	for _, m := range matches {
		f, err := os.Open(m)
		if err != nil {
			continue
		}
		manifest, err := ParseEpicManifest(f)
		_ = f.Close()
		if err != nil || !manifest.IsGame() || manifest.DisplayName == "" {
			continue
		}
		if !sameDir(manifest.InstallLocation, dir) {
			continue
		}
		return manifest.DisplayName
	}
	return ""
}

// sameDir compares two paths after cleaning and symlink resolution.
func sameDir(a, b string) bool {
	clean := func(p string) string {
		if resolved, err := filepath.EvalSymlinks(p); err == nil {
			p = resolved
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return filepath.Clean(p)
		}
		return abs
	}
	return clean(a) == clean(b)
}

// unityGenericTitles are app.info products that carry no identity.
var unityGenericTitles = map[string]bool{
	"unity": true, "game": true, "app": true, "template": true,
	"default": true, "product": true, "project": true, "test": true,
	"sample": true, "demo": true,
}

// unityName returns the product line of a Unity *_Data/app.info file (two
// lines: company, product). With several *_Data dirs, the one whose stem
// matches the exe stem wins; otherwise the lexicographic first.
func unityName(dir, exe string) string {
	matches, err := filepath.Glob(filepath.Join(dir, "*_Data", "app.info"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	pick := matches[0]
	if exe != "" {
		stem := strings.ToLower(strings.TrimSuffix(filepath.Base(exe), filepath.Ext(exe)))
		for _, m := range matches {
			dataDir := filepath.Base(filepath.Dir(m))
			if strings.EqualFold(strings.TrimSuffix(dataDir, "_Data"), stem) {
				pick = m
				break
			}
		}
	}
	st, err := os.Stat(pick)
	if err != nil || st.Size() > 1<<20 {
		return ""
	}
	data, err := os.ReadFile(pick)
	if err != nil {
		return ""
	}
	text := strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(strings.TrimSuffix(l, "\r"))
		if l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) < 2 {
		return ""
	}
	company, product := lines[0], lines[1]
	if len(product) < 3 || product == company || unityGenericTitles[strings.ToLower(product)] {
		return ""
	}
	for _, r := range product {
		if !unicode.IsPrint(r) {
			return ""
		}
	}
	return product
}
