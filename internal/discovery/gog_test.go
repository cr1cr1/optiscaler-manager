package discovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const gogGameInfoFixture = `{
	"gameId": "1207658930",
	"name": "Cool GOG Game",
	"playTasks": [
		{
			"name": "Cool GOG Game",
			"path": "CoolGame.exe",
			"workingDir": ".",
			"isPrimary": true,
			"category": "game",
			"type": "FileTask"
		},
		{
			"name": "Settings",
			"path": "tools\\settings.exe",
			"workingDir": "tools",
			"category": "tool",
			"type": "FileTask"
		}
	]
}`

func TestParseGOGGameInfo_PlayTasks(t *testing.T) {
	t.Run("primary task wins", func(t *testing.T) {
		info, err := ParseGOGGameInfo(strings.NewReader(gogGameInfoFixture))
		if err != nil {
			t.Fatalf("ParseGOGGameInfo: %v", err)
		}
		if info.GameID != "1207658930" || info.Name != "Cool GOG Game" {
			t.Fatalf("got %+v", info)
		}
		exe := info.PrimaryExe()
		if exe != "CoolGame.exe" {
			t.Fatalf("primary exe = %q, want CoolGame.exe", exe)
		}
		t.Logf("primary task: %+v -> %s", info.PlayTasks, exe)
	})

	t.Run("falls back to game category then first path", func(t *testing.T) {
		catOnly := `{"gameId": "1", "playTasks": [
			{"name": "Manual", "path": "docs\\manual.exe", "category": "document"},
			{"name": "Play", "path": "game64.exe", "category": "game"}
		]}`
		info, err := ParseGOGGameInfo(strings.NewReader(catOnly))
		if err != nil {
			t.Fatalf("ParseGOGGameInfo: %v", err)
		}
		if exe := info.PrimaryExe(); exe != `game64.exe` {
			t.Fatalf("category fallback = %q, want game64.exe", exe)
		}

		firstOnly := `{"gameId": "2", "playTasks": [
			{"name": "Only", "path": "only.exe", "category": "tool"}
		]}`
		info, err = ParseGOGGameInfo(strings.NewReader(firstOnly))
		if err != nil {
			t.Fatalf("ParseGOGGameInfo: %v", err)
		}
		if exe := info.PrimaryExe(); exe != "only.exe" {
			t.Fatalf("first-path fallback = %q, want only.exe", exe)
		}
	})

	t.Run("no play tasks yields empty exe", func(t *testing.T) {
		info, err := ParseGOGGameInfo(strings.NewReader(`{"gameId": "3", "name": "Bare"}`))
		if err != nil {
			t.Fatalf("ParseGOGGameInfo: %v", err)
		}
		if exe := info.PrimaryExe(); exe != "" {
			t.Fatalf("expected empty exe, got %q", exe)
		}
	})

	t.Run("invalid JSON errors", func(t *testing.T) {
		if _, err := ParseGOGGameInfo(strings.NewReader("{nope")); err == nil {
			t.Fatal("expected error for invalid JSON, got nil")
		} else {
			t.Logf("invalid JSON error: %v", err)
		}
	})
}

func TestGOGExePath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "goggame-1207658930.info"), gogGameInfoFixture)
	writeSized(t, filepath.Join(dir, "CoolGame.exe"), 1<<20)

	exe := GOGExePath(dir)
	want := filepath.Join(dir, "CoolGame.exe")
	if exe != want {
		t.Fatalf("GOGExePath = %q, want %q", exe, want)
	}
	t.Logf("resolved exe: %s", exe)

	// Windows-style separators in the task path are normalised.
	writeFile(t, filepath.Join(dir, "goggame-1207658930.info"),
		`{"gameId": "1", "playTasks": [{"path": "bin\\game.exe", "isPrimary": true}]}`)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSized(t, filepath.Join(dir, "bin", "game.exe"), 1<<20)
	if exe := GOGExePath(dir); exe != filepath.Join(dir, "bin", "game.exe") {
		t.Fatalf("separator normalisation: got %q", exe)
	}

	if exe := GOGExePath(t.TempDir()); exe != "" {
		t.Fatalf("empty dir must yield no exe, got %q", exe)
	}
}

// TestGOGExePathRejectsTraversal: goggame info files are third-party input;
// task paths must never resolve outside the game directory.
func TestGOGExePathRejectsTraversal(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "game")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The hostile target exists on disk, one level above the game dir.
	writeSized(t, filepath.Join(parent, "evil.exe"), 1<<20)

	cases := []struct {
		name string
		info string
	}{
		{"dotdot breakout", `{"gameId":"1","playTasks":[{"path":"../evil.exe","isPrimary":true}]}`},
		{"dotdot breakout windows separators", `{"gameId":"1","playTasks":[{"path":"..\\evil.exe","isPrimary":true}]}`},
		{"absolute path", `{"gameId":"1","playTasks":[{"path":"` + filepath.Join(parent, "evil.exe") + `","isPrimary":true}]}`},
		{"workingdir breakout", `{"gameId":"1","playTasks":[{"path":"evil.exe","workingDir":"..","isPrimary":true}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writeFile(t, filepath.Join(dir, "goggame-1.info"), tc.info)
			if exe := GOGExePath(dir); exe != "" {
				t.Fatalf("hostile task path resolved to %q; must be rejected", exe)
			}
		})
	}

	// A benign nested path inside the game dir still resolves.
	writeFile(t, filepath.Join(dir, "goggame-1.info"),
		`{"gameId":"1","playTasks":[{"path":"bin/game.exe","isPrimary":true}]}`)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeSized(t, filepath.Join(dir, "bin", "game.exe"), 1<<20)
	if exe := GOGExePath(dir); exe != filepath.Join(dir, "bin", "game.exe") {
		t.Fatalf("benign nested path: got %q", exe)
	}
}
