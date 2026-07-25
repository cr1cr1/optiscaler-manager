//go:build linux

package umu

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// writeFakeUmuRun drops a shell script named "umu-run" into dir/bin that
// prints the supplied stdout when called with --version. The script is
// made executable and dir/bin is returned so the caller can prepend it
// to PATH.
func writeFakeUmuRun(t *testing.T, dir, versionOut string) string {
	t.Helper()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", bin, err)
	}
	path := filepath.Join(bin, "umu-run")
	body := "#!/bin/sh\n" +
		"if [ \"$1\" = \"--version\" ]; then\n" +
		"  printf '%s\\n' " + quoteSh(versionOut) + "\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return bin
}

// quoteSh wraps s in single quotes, escaping any embedded single quotes.
func quoteSh(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func TestDetect_ParsesUmuRunVersion(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeUmuRun(t, dir, "umu-launcher version 1.4.4 (Python 3.11.6)")
	t.Setenv("PATH", bin+string(filepath.ListSeparator)+"/usr/bin")

	path, ver, err := Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: unexpected err %v", err)
	}
	if want := filepath.Join(bin, "umu-run"); path != want {
		t.Errorf("path = %q, want %q", path, want)
	}
	if ver != "1.4.4" {
		t.Errorf("version = %q, want %q", ver, "1.4.4")
	}
}

func TestDetect_AcceptsVersionLineWithExtraWhitespace(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeUmuRun(t, dir, "  umu-launcher  version   2.0.1   (Python 3.13.0)  ")
	t.Setenv("PATH", bin)

	_, ver, err := Detect(context.Background())
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if ver != "2.0.1" {
		t.Errorf("version = %q, want %q", ver, "2.0.1")
	}
}

func TestDetect_ReturnsErrUnavailableWhenNotOnPath(t *testing.T) {
	t.Setenv("PATH", "")
	_, _, err := Detect(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestDetect_ReturnsErrUnavailableWhenVersionMalformed(t *testing.T) {
	dir := t.TempDir()
	bin := writeFakeUmuRun(t, dir, "this is not a version string")
	t.Setenv("PATH", bin)

	_, _, err := Detect(context.Background())
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable when --version output is unparseable", err)
	}
}

func TestDetectVersionRegex(t *testing.T) {
	cases := []struct{ in, want string }{
		{"umu-launcher version 1.4.4 (Python 3.11.6)", "1.4.4"},
		{"  umu-launcher  version   2.0.1   (Python 3.13.0)  ", "2.0.1"},
		{"version 0.0.1", "0.0.1"},
		{"nope here", ""},
		{"umu-launcher version 1.2.3.4", "1.2.3"}, // first three dotted numbers only
	}
	for _, c := range cases {
		got := versionRegex.FindStringSubmatch(c.in)
		if c.want == "" {
			if got != nil {
				t.Errorf("in=%q: got %v, want no match", c.in, got)
			}
			continue
		}
		if got == nil || got[1] != c.want {
			t.Errorf("in=%q: got %v, want %q", c.in, got, c.want)
		}
	}
}

var _ = regexp.MustCompile
