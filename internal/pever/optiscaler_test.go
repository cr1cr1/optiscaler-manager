package pever

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOptiScalerVersionChain(t *testing.T) {
	write := func(dir, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("manifest wins over log and ini", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "manifest.json", `{"name":"OptiScaler","version":"0.9.4"}`)
		write(dir, "OptiScaler.log", "OptiScaler v0.9.3\n")
		write(dir, "OptiScaler.ini", "[Upscalers]\n")
		if got := OptiScalerVersion(dir); got != "0.9.4" {
			t.Errorf("got %q, want %q", got, "0.9.4")
		}
	})

	t.Run("log first-ten-lines when no manifest", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "OptiScaler.log", "some preamble\nOptiScaler v0.9.4-pre2 (build abc)\nmore\n")
		write(dir, "OptiScaler.ini", "[Upscalers]\n")
		if got := OptiScalerVersion(dir); got != "0.9.4-pre2" {
			t.Errorf("got %q, want %q", got, "0.9.4-pre2")
		}
	})

	t.Run("version banner past ten lines is not found", func(t *testing.T) {
		dir := t.TempDir()
		log := ""
		for i := 0; i < 11; i++ {
			log += "noise line\n"
		}
		log += "OptiScaler v9.9.9\n"
		write(dir, "OptiScaler.log", log)
		write(dir, "OptiScaler.ini", "[Upscalers]\n")
		if got := OptiScalerVersion(dir); got != "" {
			t.Errorf("got %q, want %q", got, "")
		}
	})

	t.Run("ini presence means unknown", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "OptiScaler.ini", "[Upscalers]\n")
		if got := OptiScalerVersion(dir); got != "" {
			t.Errorf("got %q, want empty (unknown)", got)
		}
	})

	t.Run("empty dir is unknown", func(t *testing.T) {
		if got := OptiScalerVersion(t.TempDir()); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	t.Run("manifest with empty version falls through to log", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "manifest.json", `{"name":"OptiScaler"}`)
		write(dir, "OptiScaler.log", "OptiScaler v0.8.1\n")
		if got := OptiScalerVersion(dir); got != "0.8.1" {
			t.Errorf("got %q, want %q", got, "0.8.1")
		}
	})

	t.Run("oversized manifest is not slurped and falls through to log", func(t *testing.T) {
		dir := t.TempDir()
		write(dir, "manifest.json", `{"version":"9.9.9","pad":"`+strings.Repeat("x", maxManifestSize)+`"}`)
		write(dir, "OptiScaler.log", "OptiScaler v0.8.1\n")
		if got := OptiScalerVersion(dir); got != "0.8.1" {
			t.Errorf("got %q, want log fallback %q", got, "0.8.1")
		}
	})

	t.Run("manifest path that is a directory falls through to log", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.Mkdir(filepath.Join(dir, "manifest.json"), 0o755); err != nil {
			t.Fatal(err)
		}
		write(dir, "OptiScaler.log", "OptiScaler v0.8.2\n")
		if got := OptiScalerVersion(dir); got != "0.8.2" {
			t.Errorf("got %q, want log fallback %q", got, "0.8.2")
		}
	})
}
