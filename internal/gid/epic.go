package gid

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// EpicManifest is one parsed Epic Games Launcher manifest (.item or an
// in-dir .egstore/*.manifest). The fields map to the launcher JSON
// verbatim; AppCategories is flattened to plain strings for classification.
type EpicManifest struct {
	AppName         string
	DisplayName     string
	InstallLocation string
	AppCategories   []string
}

// IsGame reports whether the manifest describes a game (as opposed to a
// plugin, addon, or tool the launcher also tracks).
func (m EpicManifest) IsGame() bool {
	for _, c := range m.AppCategories {
		if strings.Contains(strings.ToLower(c), "games") {
			return true
		}
	}
	return false
}

// ParseEGStoreManifest parses the newer in-dir .egstore manifest format
// by streaming the document header: real markers carry a multi-megabyte
// FileManifestList, so the fields are read token by token and the walk
// stops as soon as they are all found.
func ParseEGStoreManifest(r io.Reader) (appName, launchExe string, err error) {
	dec := json.NewDecoder(io.LimitReader(r, 64<<20))
	tok, err := dec.Token()
	if err != nil {
		return "", "", fmt.Errorf("egstore manifest: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return "", "", fmt.Errorf("egstore manifest: not an object")
	}
	var version string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			break
		}
		key, _ := keyTok.(string)
		var decErr error
		switch key {
		case "ManifestFileVersion":
			decErr = dec.Decode(&version)
		case "AppNameString":
			decErr = dec.Decode(&appName)
		case "LaunchExeString":
			decErr = dec.Decode(&launchExe)
		default:
			var raw json.RawMessage
			decErr = dec.Decode(&raw)
		}
		if decErr != nil {
			return "", "", fmt.Errorf("egstore manifest: %w", decErr)
		}
		if version != "" && appName != "" && launchExe != "" {
			break
		}
	}
	if version == "" || appName == "" {
		return "", "", fmt.Errorf("egstore manifest: not the in-dir format (AppNameString=%q)", appName)
	}
	return appName, launchExe, nil
}

type epicManifestJSON struct {
	AppName         string `json:"AppName"`
	DisplayName     string `json:"DisplayName"`
	InstallLocation string `json:"InstallLocation"`
	AppCategories   []struct {
		Category string `json:"Category"`
		Path     string `json:"Path"`
	} `json:"AppCategories"`
}

// ParseEpicManifest parses one Epic manifest from r.
func ParseEpicManifest(r io.Reader) (EpicManifest, error) {
	var raw epicManifestJSON
	if err := json.NewDecoder(r).Decode(&raw); err != nil {
		return EpicManifest{}, fmt.Errorf("epic manifest: %w", err)
	}
	if raw.AppName == "" || raw.InstallLocation == "" {
		return EpicManifest{}, fmt.Errorf("epic manifest: missing required field (AppName=%q InstallLocation=%q)", raw.AppName, raw.InstallLocation)
	}
	m := EpicManifest{
		AppName:         raw.AppName,
		DisplayName:     raw.DisplayName,
		InstallLocation: raw.InstallLocation,
	}
	for _, c := range raw.AppCategories {
		if c.Category != "" {
			m.AppCategories = append(m.AppCategories, c.Category)
		}
		if c.Path != "" {
			m.AppCategories = append(m.AppCategories, c.Path)
		}
	}
	if m.DisplayName == "" {
		m.DisplayName = raw.AppName
	}
	return m, nil
}
