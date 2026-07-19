package discovery

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

// fakeRegistry is an in-memory registryReader: maps "PATH\0name" to values
// and PATH to subkey names.
type fakeRegistry struct {
	values  map[string]string
	subkeys map[string][]string
}

func (f *fakeRegistry) Subkeys(path string) ([]string, error) {
	subs, ok := f.subkeys[path]
	if !ok {
		return nil, errors.New("key not found: " + path)
	}
	return subs, nil
}

func (f *fakeRegistry) ReadString(path, name string) (string, error) {
	v, ok := f.values[path+"\x00"+name]
	if !ok {
		return "", errors.New("value not found: " + path + `\` + name)
	}
	return v, nil
}

func TestRegistryFake_DrivesGOGDiscovery(t *testing.T) {
	gameDir := t.TempDir()
	writeFile(t, filepath.Join(gameDir, "goggame-1207658930.info"), gogGameInfoFixture)
	writeSized(t, filepath.Join(gameDir, "CoolGame.exe"), 1<<20)

	missingDir := filepath.Join(t.TempDir(), "gone")

	const base = `SOFTWARE\WOW6432Node\GOG.com\Games`
	reg := &fakeRegistry{
		subkeys: map[string][]string{
			base: {"1207658930", "999"},
		},
		values: map[string]string{
			base + `\1207658930` + "\x00gameName": "Cool GOG Game",
			base + `\1207658930` + "\x00path":     gameDir,
			base + `\1207658930` + "\x00gameID":   "1207658930",
			// Entry pointing at a missing install dir must be skipped.
			base + `\999` + "\x00gameName": "Ghost Game",
			base + `\999` + "\x00path":     missingDir,
			base + `\999` + "\x00gameID":   "999",
		},
	}

	games := gogGamesFromRegistry(reg, base)
	if len(games) != 1 {
		t.Fatalf("got %d games %+v, want 1", len(games), games)
	}
	g := games[0]
	t.Logf("gog game from fake registry: %+v", g)
	if g.Store != domain.StoreGOG {
		t.Errorf("store = %v, want GOG", g.Store)
	}
	if g.AppID != "1207658930" || g.Name != "Cool GOG Game" || g.InstallDir != gameDir {
		t.Errorf("unexpected identity: %+v", g)
	}
	wantExe := filepath.Join(gameDir, "CoolGame.exe")
	if g.ExePath != wantExe {
		t.Errorf("ExePath = %q, want %q (enriched from goggame info)", g.ExePath, wantExe)
	}
}
