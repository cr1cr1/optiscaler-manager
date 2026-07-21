// Package gid identifies games from hard evidence inside their install
// directories: store id files (steam_appid.txt), store metadata
// (goggame-*.info, .egstore manifests), and engine metadata (Unity
// app.info). It is the offline half of the v0.8 identification stack —
// online canonical resolution lives in the enrich phase. gid sits below
// discovery: it must never import it.
package gid

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// Result is what offline detection found in a game directory: a Steam app
// id when one is recorded (name resolution is the enrich phase's job, so
// an appid alone never sets Title), the title when an in-dir metadata
// file yields one directly, and the Epic catalog id when an .egstore
// marker is present (the newer manifest format carries no display name —
// the title falls through the chain).
type Result struct {
	SteamAppID  string
	Title       string
	Source      domain.TitleSource
	EpicAppName string
}

// Detect scans dir for identity files and returns the best evidence.
// Title precedence: goggame-*.info > .egstore DisplayName > Unity
// app.info; a steam_appid.txt and an .egstore catalog id are orthogonal
// captures that are always reported. exe may be "" and only breaks ties
// between multiple Unity *_Data dirs.
func Detect(dir, exe string) Result {
	r := Result{SteamAppID: findSteamAppID(dir), EpicAppName: egstoreID(dir)}
	if name := gogName(dir); name != "" {
		r.Title, r.Source = name, domain.SourceGOGInfo
		return r
	}
	if name := egstoreTitle(dir); name != "" {
		r.Title, r.Source = name, domain.SourceEGStore
		return r
	}
	if name := unityName(dir, exe); name != "" {
		r.Title, r.Source = name, domain.SourceUnity
		return r
	}
	return r
}

// egstoreID returns the Epic catalog id from a .egstore marker, in either
// manifest format (the newer in-dir format's AppNameString, or an older
// .item manifest's AppName).
func egstoreID(dir string) string {
	for _, data := range egstoreFiles(dir) {
		if appID, _, err := ParseEGStoreManifest(bytes.NewReader(data)); err == nil {
			return appID
		}
		if m, err := ParseEpicManifest(bytes.NewReader(data)); err == nil {
			return m.AppName
		}
	}
	return ""
}

// egstoreTitle returns the DisplayName of an .item-shaped .egstore
// manifest, but only when its InstallLocation points at this very
// directory — the marker survives uninstalls, so a stale one is not
// evidence.
func egstoreTitle(dir string) string {
	for _, data := range egstoreFiles(dir) {
		manifest, err := ParseEpicManifest(bytes.NewReader(data))
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

// egstoreFiles reads the (bounded) contents of the first few
// .egstore/*.manifest files in deterministic order: each file is capped
// at 1MiB and the count is capped, so a hostile dir cannot turn
// identification into a memory blowout.
func egstoreFiles(dir string) [][]byte {
	matches, err := filepath.Glob(filepath.Join(dir, ".egstore", "*.manifest"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	if len(matches) > 8 {
		matches = matches[:8]
	}
	var out [][]byte
	for _, m := range matches {
		f, err := os.Open(m)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, 1<<20))
		_ = f.Close()
		if err == nil {
			out = append(out, data)
		}
	}
	return out
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
	// A rejected parse (a 480 placeholder, garbage) must not mask a real
	// id in a deeper file.
	for _, c := range found {
		if id := readAppID(c.path); id != "" {
			return id
		}
	}
	return ""
}

// readAppID parses the first line of a steam_appid.txt: digits only after
// trimming BOM/whitespace/CR; 480 (Steam's test app) is not a real id.
// The read is bounded — a giant file is not slurped for one line.
func readAppID(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, 4096))
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
	info, err := ParseGOGGameInfo(io.LimitReader(f, 1<<20))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(info.Name)
}

// egstoreName reads .egstore manifests: the newer in-dir format yields
// its catalog id (no display name exists there), an older .item-shaped
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
