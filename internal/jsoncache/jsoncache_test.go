package jsoncache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

type entry struct {
	Name      string    `json:"name"`
	FetchedAt time.Time `json:"fetched_at"`
}

func TestReadWriteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "e.json") // sub dir must be created by Write

	want := entry{Name: "hades", FetchedAt: time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)}
	if err := Write(path, want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, ok := Read[entry](path)
	if !ok {
		t.Fatal("Read: miss on written file")
	}
	if got.Name != want.Name || !got.FetchedAt.Equal(want.FetchedAt) {
		t.Fatalf("Read: got %+v, want %+v", got, want)
	}
	t.Log("round trip ok:", got)
}

func TestReadMisses(t *testing.T) {
	dir := t.TempDir()

	if _, ok := Read[entry](filepath.Join(dir, "absent.json")); ok {
		t.Fatal("Read: hit on missing file")
	}

	corrupt := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(corrupt, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Read[entry](corrupt); ok {
		t.Fatal("Read: hit on corrupt file")
	}
	t.Log("missing and corrupt files both miss")
}

func TestCooldownWindow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cooldown.json")
	now := time.Date(2026, 8, 28, 12, 0, 0, 0, time.UTC)
	window := 5 * time.Minute

	if InCooldown(path, now, window) {
		t.Fatal("InCooldown: true with no file")
	}
	if err := WriteCooldown(path, now); err != nil {
		t.Fatalf("WriteCooldown: %v", err)
	}
	if !InCooldown(path, now.Add(window-time.Second), window) {
		t.Fatal("InCooldown: false inside window")
	}
	if InCooldown(path, now.Add(window), window) {
		t.Fatal("InCooldown: true at window edge")
	}
	t.Log("cooldown window boundaries ok")
}
