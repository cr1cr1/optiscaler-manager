//go:build linux

package umu

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeWineserver installs a script recording its argv and WINEPREFIX.
func fakeWineserver(t *testing.T, runner, rel string) (capture string) {
	t.Helper()
	capture = filepath.Join(t.TempDir(), "ws-capture")
	ws := filepath.Join(runner, rel)
	if err := os.MkdirAll(filepath.Dir(ws), 0o755); err != nil {
		t.Fatalf("mkdir wineserver dir: %v", err)
	}
	script := "#!/bin/sh\n" +
		`printf '%s|%s\n' "$1" "$WINEPREFIX" > "` + capture + `"` + "\n"
	if err := os.WriteFile(ws, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake wineserver: %v", err)
	}
	return capture
}

// TestKillWineserver: the runner's wineserver is invoked with -k and
// the prefix in WINEPREFIX (issue 069 — group kills miss wineserver
// clients, which re-session themselves; both the GE-Proton bin/ and
// UMU-Proton files/bin/ layouts are found).
func TestKillWineserver(t *testing.T) {
	for _, rel := range []string{"bin/wineserver", "files/bin/wineserver"} {
		t.Run(rel, func(t *testing.T) {
			runner := t.TempDir()
			capture := fakeWineserver(t, runner, rel)
			KillWineserver(context.Background(), runner, "/some/prefix")

			var data []byte
			for deadline := time.Now().Add(2 * time.Second); ; {
				var err error
				data, err = os.ReadFile(capture)
				if err == nil && len(data) > 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("wineserver never invoked (%s)", rel)
				}
				time.Sleep(10 * time.Millisecond)
			}
			if got := strings.TrimRight(string(data), "\n"); got != "-k|/some/prefix" {
				t.Errorf("wineserver invocation = %q, want %q", got, "-k|/some/prefix")
			}
		})
	}
}

// TestKillWineserverNoBinary: no wineserver in the runner is a silent
// no-op (best effort cleanup, never a stop failure).
func TestKillWineserverNoBinary(t *testing.T) {
	KillWineserver(context.Background(), t.TempDir(), "/some/prefix") // must not panic or hang
}
