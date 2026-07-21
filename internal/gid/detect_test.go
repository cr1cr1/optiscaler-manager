package gid

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gogInfoJSON(id, name string) string {
	return `{"gameId":"` + id + `","name":"` + name + `","playTasks":[]}`
}

func egstoreJSON(appName, displayName, installLocation string) string {
	return `{"AppName":"` + appName + `","DisplayName":"` + displayName + `","InstallLocation":"` + installLocation + `","AppCategories":[{"Category":"games"}]}`
}

func TestDetect_SteamAppIDTxt(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "steam_appid.txt"), "2322010\n")
	got := Detect(root, "")
	if got.SteamAppID != "2322010" || got.Title != "" || got.Source != "" {
		t.Errorf("Detect = %+v, want appid only", got)
	}
}

func TestDetect_SteamAppIDTxtNestedDepths(t *testing.T) {
	for _, rel := range []string{"steam_settings/steam_appid.txt", "a/b/steam_appid.txt"} {
		root := t.TempDir()
		write(t, filepath.Join(root, rel), "1693980")
		if got := Detect(root, ""); got.SteamAppID != "1693980" {
			t.Errorf("%s: SteamAppID = %q, want found", rel, got.SteamAppID)
		}
	}
}

func TestDetect_SteamAppIDTxtTooDeep(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a/b/c/steam_appid.txt"), "2322010")
	if got := Detect(root, ""); got.SteamAppID != "" {
		t.Errorf("depth-3 appid file must be ignored, got %q", got.SteamAppID)
	}
}

func TestDetect_SteamAppIDRejectedValues(t *testing.T) {
	for _, content := range []string{"480", "abc", "", "12 34", "  ", "00480"} {
		root := t.TempDir()
		write(t, filepath.Join(root, "steam_appid.txt"), content)
		if got := Detect(root, ""); got.SteamAppID != "" {
			t.Errorf("content %q: SteamAppID = %q, want rejected", content, got.SteamAppID)
		}
	}
}

func TestDetect_SteamAppIDShallowestWins(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "steam_appid.txt"), "111")
	write(t, filepath.Join(root, "steam_settings", "steam_appid.txt"), "222")
	if got := Detect(root, ""); got.SteamAppID != "111" {
		t.Errorf("SteamAppID = %q, want shallowest (root) file", got.SteamAppID)
	}
}

func TestDetect_GOGInfo(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "goggame-1552771812.info"), gogInfoJSON("1552771812", "A Plague Tale: Requiem"))
	got := Detect(root, "")
	if got.Title != "A Plague Tale: Requiem" || got.Source != domain.SourceGOGInfo {
		t.Errorf("Detect = %+v, want goggame title", got)
	}
}

func TestDetect_EGStore(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".egstore", "ABC123.manifest"), egstoreJSON("Prey", "Prey", root))
	got := Detect(root, "")
	if got.Title != "Prey" || got.Source != domain.SourceEGStore {
		t.Errorf("Detect = %+v, want egstore title", got)
	}
}

// .egstore markers linger after uninstalls: a manifest pointing at a
// different install location is not evidence for this dir.
func TestDetect_EGStoreStaleRejected(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".egstore", "ABC123.manifest"), egstoreJSON("Prey", "Prey", filepath.Join(t.TempDir(), "elsewhere")))
	if got := Detect(root, ""); got.Title != "" {
		t.Errorf("stale egstore: Title = %q, want rejected", got.Title)
	}
}

func TestDetect_UnityAppInfo(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "TOI_Data", "app.info"), "Odd Bug Studio\nTails of Iron\n")
	got := Detect(root, "")
	if got.Title != "Tails of Iron" || got.Source != domain.SourceUnity {
		t.Errorf("Detect = %+v, want unity title", got)
	}
}

func TestDetect_UnityRejected(t *testing.T) {
	for _, content := range []string{"Unity\nGame", "Acme\nAcme", "Acme\nab", "\n\n", "Acme"} {
		root := t.TempDir()
		write(t, filepath.Join(root, "X_Data", "app.info"), content)
		if got := Detect(root, ""); got.Title != "" {
			t.Errorf("content %q: Title = %q, want rejected", content, got.Title)
		}
	}
}

func TestDetect_UnityMultipleDataDirsPrefersExeStem(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "Other_Data", "app.info"), "A\nWrong Game")
	write(t, filepath.Join(root, "MyGame_Data", "app.info"), "B\nRight Game")
	got := Detect(root, filepath.Join(root, "MyGame.exe"))
	if got.Title != "Right Game" {
		t.Errorf("Title = %q, want the exe-matching _Data dir", got.Title)
	}
}

func TestDetect_Precedence(t *testing.T) {
	// appid recorded alongside a goggame title; goggame beats egstore; egstore beats unity.
	root := t.TempDir()
	write(t, filepath.Join(root, "steam_appid.txt"), "111")
	write(t, filepath.Join(root, "goggame-1.info"), gogInfoJSON("1", "GOG Name"))
	write(t, filepath.Join(root, ".egstore", "M.manifest"), egstoreJSON("E", "Epic Name", root))
	write(t, filepath.Join(root, "X_Data", "app.info"), "C\nUnity Name")
	got := Detect(root, "")
	if got.SteamAppID != "111" || got.Title != "GOG Name" || got.Source != domain.SourceGOGInfo {
		t.Errorf("Detect = %+v, want appid + goggame title", got)
	}

	root2 := t.TempDir()
	write(t, filepath.Join(root2, ".egstore", "M.manifest"), egstoreJSON("E", "Epic Name", root2))
	write(t, filepath.Join(root2, "X_Data", "app.info"), "C\nUnity Name")
	if got := Detect(root2, ""); got.Title != "Epic Name" || got.Source != domain.SourceEGStore {
		t.Errorf("Detect = %+v, want egstore over unity", got)
	}
}

func TestDetect_Nothing(t *testing.T) {
	if got := Detect(t.TempDir(), ""); got != (Result{}) {
		t.Errorf("Detect = %+v, want zero", got)
	}
}

// Real Epic installs write a newer .egstore manifest format (no
// DisplayName, no InstallLocation — AppNameString is the catalog id and
// LaunchExeString the game exe). It identifies the install without
// naming it: the id is captured, the title falls through the chain.
func TestDetect_EGStoreNewFormat(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".egstore", "X.manifest"), `{"ManifestFileVersion":"013000000000","AppNameString":"6b9b4207048a4a5eb98a0803ebbfe7fa","LaunchExeString":"Binaries/Danielle/x64-Epic/Release/Prey.exe","FileManifestList":[]}`)
	got := Detect(root, "")
	if got.EpicAppName != "6b9b4207048a4a5eb98a0803ebbfe7fa" {
		t.Errorf("EpicAppName = %q, want the catalog id", got.EpicAppName)
	}
	if got.Title != "" {
		t.Errorf("Title = %q, want none from the new format", got.Title)
	}
}

// The older .item-shaped manifest still yields its DisplayName, and the
// epic id is captured either way.
func TestDetect_EGStoreItemFormatCapturesID(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".egstore", "X.manifest"), egstoreJSON("PreyApp", "Prey", root))
	got := Detect(root, "")
	if got.Title != "Prey" || got.Source != domain.SourceEGStore || got.EpicAppName != "PreyApp" {
		t.Errorf("Detect = %+v, want egstore title + id", got)
	}
}

// Real .egstore manifests carry a multi-megabyte FileManifestList after
// the header fields: parsing must read the header without requiring the
// whole document.
func TestDetect_EGStoreHugeManifest(t *testing.T) {
	root := t.TempDir()
	var b strings.Builder
	b.WriteString(`{"ManifestFileVersion":"013000000000","AppNameString":"6b9b4207048a4a5eb98a0803ebbfe7fa","LaunchExeString":"Binaries/Prey.exe","FileManifestList":[`)
	for i := 0; i < 4000; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(`{"Filename":"f","FileHash":"h","FileChunkParts":[]}`)
	}
	b.WriteString(`]}`)
	write(t, filepath.Join(root, ".egstore", "X.manifest"), b.String())
	got := Detect(root, "")
	if got.EpicAppName != "6b9b4207048a4a5eb98a0803ebbfe7fa" {
		t.Errorf("EpicAppName = %q, want the catalog id from a huge manifest", got.EpicAppName)
	}
}

// A junk shallow file must not mask a real nested id: repack tooling can
// leave a 480 placeholder at the root while the true id sits in
// steam_settings/.
func TestDetect_SteamAppIDRejectedShallowDoesNotMaskNested(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "steam_appid.txt"), "480")
	write(t, filepath.Join(root, "steam_settings", "steam_appid.txt"), "2322010")
	if got := Detect(root, ""); got.SteamAppID != "2322010" {
		t.Errorf("SteamAppID = %q, want the nested real id (480 must not mask)", got.SteamAppID)
	}
}

// A garbage shallow file masks nothing either.
func TestDetect_SteamAppIDGarbageShallowDoesNotMaskNested(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "steam_appid.txt"), "not-a-number")
	write(t, filepath.Join(root, "cfg", "steam_appid.txt"), "1693980")
	if got := Detect(root, ""); got.SteamAppID != "1693980" {
		t.Errorf("SteamAppID = %q, want the nested real id", got.SteamAppID)
	}
}

// A multi-megabyte steam_appid.txt must not be slurped: the reader is
// bounded and still parses the first line.
func TestDetect_SteamAppIDHugeFileBounded(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "steam_appid.txt"), "2322010\n"+strings.Repeat("x", 4<<20))
	if got := Detect(root, ""); got.SteamAppID != "2322010" {
		t.Errorf("SteamAppID = %q, want parsed from the bounded read", got.SteamAppID)
	}
}
