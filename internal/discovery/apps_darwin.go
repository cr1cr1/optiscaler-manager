//go:build darwin

package discovery

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// storeApps scans /Applications and ~/Applications (depth 1) for .app
// bundles. Bundle display name and executable come from Contents/Info.plist
// when it is an XML plist; binary plists and unreadable metadata fall back
// to the bundle name with no resolved executable.
func storeApps() []domain.Game {
	var bases []string
	bases = append(bases, "/Applications")
	if home, err := os.UserHomeDir(); err == nil {
		bases = append(bases, filepath.Join(home, "Applications"))
	}
	var games []domain.Game
	seen := map[string]bool{}
	for _, base := range bases {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || !strings.HasSuffix(e.Name(), ".app") {
				continue
			}
			bundle := canonicalPath(filepath.Join(base, e.Name()))
			if seen[bundle] {
				continue
			}
			seen[bundle] = true
			name := strings.TrimSuffix(e.Name(), ".app")
			exe := ""
			if kv, err := readInfoPlist(bundle); err == nil {
				if n := kv["CFBundleName"]; n != "" {
					name = n
				}
				if x := kv["CFBundleExecutable"]; x != "" {
					cand := filepath.Join(bundle, "Contents", "MacOS", x)
					if st, err := os.Stat(cand); err == nil && !st.IsDir() {
						exe = cand
					}
				}
			} else {
				log.Debug().Err(err).Str("bundle", bundle).Msg("Info.plist unreadable, using bundle name")
			}
			games = append(games, domain.Game{
				AppID:      "app_" + name,
				Name:       name,
				InstallDir: bundle,
				Store:      domain.StoreManual,
				ExePath:    exe,
			})
		}
	}
	return games
}

func readInfoPlist(bundle string) (map[string]string, error) {
	f, err := os.Open(filepath.Join(bundle, "Contents", "Info.plist"))
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return parseXMLPlist(f)
}
