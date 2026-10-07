package gid

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// HeroicEntry is one installed game from a Heroic Games Launcher
// installed.json file (legendary/installed.json for Epic,
// gog_store/installed.json for GOG). Both files are a JSON object keyed by
// app name whose values share this field subset; legendary uses snake_case
// app_name, the GOG store camelCase appName, so both are accepted.
type HeroicEntry struct {
	AppName     string
	Title       string
	InstallPath string
	Executable  string // relative to InstallPath; "" when not recorded
	Platform    string // "Windows"/"windows"/"Mac"/...; "" in older files
}

// IsWindows reports whether the entry is a Windows build — the only kind
// OptiScaler can target. An empty platform (older legendary files) counts
// as Windows: those files predate cross-platform installs.
func (e HeroicEntry) IsWindows() bool {
	return e.Platform == "" || strings.EqualFold(e.Platform, "windows")
}

type heroicEntryJSON struct {
	AppNameSnake string `json:"app_name"`
	AppNameCamel string `json:"appName"`
	Title        string `json:"title"`
	InstallPath  string `json:"install_path"`
	Executable   string `json:"executable"`
	Platform     string `json:"platform"`
}

// ParseHeroicInstalled parses one Heroic installed.json document from r.
// Entries without an install path are dropped (nothing to manage); a
// missing app name falls back to the map key, a missing title to the app
// name. The result is sorted by app name for deterministic scans.
func ParseHeroicInstalled(r io.Reader) ([]HeroicEntry, error) {
	var raw map[string]heroicEntryJSON
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return nil, fmt.Errorf("heroic installed: %w", err)
	}
	entries := make([]HeroicEntry, 0, len(raw))
	for key, e := range raw {
		appName := e.AppNameSnake
		if appName == "" {
			appName = e.AppNameCamel
		}
		if appName == "" {
			appName = key
		}
		if e.InstallPath == "" {
			continue
		}
		title := e.Title
		if title == "" {
			title = appName
		}
		entries = append(entries, HeroicEntry{
			AppName:     appName,
			Title:       title,
			InstallPath: e.InstallPath,
			Executable:  e.Executable,
			Platform:    e.Platform,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].AppName < entries[j].AppName })
	return entries, nil
}
