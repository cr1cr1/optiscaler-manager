package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

const epicGameItem = `{
	"FormatVersion": 0,
	"AppName": "Fortnite",
	"AppVersionString": "1.0",
	"LaunchExecutable": "FortniteGame/Binaries/Win64/FortniteClient-Win64-Shipping.exe",
	"DisplayName": "Fortnite",
	"InstallLocation": "C:\\Games\\Fortnite",
	"AppCategories": [
		{"Category": "games"},
		{"Category": "applications"}
	]
}`

const epicPluginItem = `{
	"AppName": "SomePlugin",
	"DisplayName": "Some Plugin",
	"InstallLocation": "C:\\Games\\SomePlugin",
	"AppCategories": [
		{"Category": "plugins"},
		{"Path": "addons"}
	]
}`

func TestParseEpicManifest_Fixtures(t *testing.T) {
	t.Run("game manifest", func(t *testing.T) {
		m, err := ParseEpicManifest(strings.NewReader(epicGameItem))
		if err != nil {
			t.Fatalf("ParseEpicManifest: %v", err)
		}
		if m.AppName != "Fortnite" || m.DisplayName != "Fortnite" || m.InstallLocation != `C:\Games\Fortnite` {
			t.Fatalf("got %+v", m)
		}
		if !m.IsGame() {
			t.Fatalf("games category not recognised: %+v", m.AppCategories)
		}
		t.Logf("game manifest: %+v", m)
	})

	t.Run("plugin manifest is not a game", func(t *testing.T) {
		m, err := ParseEpicManifest(strings.NewReader(epicPluginItem))
		if err != nil {
			t.Fatalf("ParseEpicManifest: %v", err)
		}
		if m.IsGame() {
			t.Fatalf("plugins/addons must not classify as game: %+v", m.AppCategories)
		}
		t.Logf("non-game categories: %v", m.AppCategories)
	})

	t.Run("invalid JSON errors", func(t *testing.T) {
		if _, err := ParseEpicManifest(strings.NewReader("{not json")); err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		} else {
			t.Logf("invalid JSON error: %v", err)
		}
	})
}

func TestScanEpicManifests(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "installed", "CoolGame")
	if err := os.MkdirAll(gameDir, 0o755); err != nil {
		t.Fatal(err)
	}
	pluginDir := filepath.Join(root, "installed", "CoolPlugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}

	manifests := filepath.Join(root, "Manifests")
	writeFile(t, filepath.Join(manifests, "cool.item"), `{
		"AppName": "CoolGameApp",
		"DisplayName": "Cool Game",
		"InstallLocation": "`+strings.ReplaceAll(gameDir, `\`, `\\`)+`",
		"AppCategories": [{"Category": "games"}]
	}`)
	writeFile(t, filepath.Join(manifests, "plugin.item"), `{
		"AppName": "CoolPluginApp",
		"DisplayName": "Cool Plugin",
		"InstallLocation": "`+strings.ReplaceAll(pluginDir, `\`, `\\`)+`",
		"AppCategories": [{"Category": "plugins"}]
	}`)
	writeFile(t, filepath.Join(manifests, "broken.item"), "{not json")
	writeFile(t, filepath.Join(manifests, "ghost.item"), `{
		"AppName": "GhostApp",
		"DisplayName": "Ghost",
		"InstallLocation": "`+strings.ReplaceAll(filepath.Join(root, "installed", "Missing"), `\`, `\\`)+`",
		"AppCategories": [{"Category": "games"}]
	}`)

	games, err := ScanEpicManifests(manifests)
	if err != nil {
		t.Fatalf("ScanEpicManifests: %v", err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games %+v, want 1", len(games), games)
	}
	g := games[0]
	t.Logf("epic game: %+v", g)
	if g.Store != domain.StoreEpic || g.Name != "Cool Game" || g.AppName != "CoolGameApp" || g.InstallDir != gameDir {
		t.Fatalf("unexpected game: %+v", g)
	}
}
