// Package launchlog writes per-launch log files (issue 061): the exact
// argv, the env delta the launcher set, and the child's full
// stdout/stderr. Launch failures used to surface only a stderr tail in
// a toast; the per-launch file under <data root>/logs is the debugging
// source of truth.
package launchlog

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

// Create makes dir on demand and opens a new log file named
// <slug(game)>-<timestamp>.log inside it, returning its path and the
// open file (the caller closes). An empty dir is an error — "no
// logging" is expressed by not calling Create at all.
func Create(dir, game string) (path string, f *os.File, err error) {
	if dir == "" {
		return "", nil, errors.New("launchlog: empty dir")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	// Microsecond precision keeps rapid consecutive launches of the
	// same game collision-free while staying sortable by name.
	name := Slug(game) + "-" + time.Now().Format("20060102-150405.000000") + ".log"
	path = filepath.Join(dir, name)
	f, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", nil, err
	}
	return path, f, nil
}

// WriteHeader writes the launch preamble: timestamp, the exact argv,
// and the env delta the launcher set (never the full inherited
// environment — that would leak unrelated secrets into log files).
func WriteHeader(w io.Writer, argv, envDelta []string) {
	fmt.Fprintf(w, "# optiscaler-manager launch log\n# time: %s\n# argv: %s\n",
		time.Now().Format(time.RFC3339), strings.Join(argv, " "))
	if len(envDelta) > 0 {
		io.WriteString(w, "# env:\n")
		for _, kv := range envDelta {
			fmt.Fprintf(w, "%s\n", kv)
		}
	}
	io.WriteString(w, "# --- output ---\n")
}

// Slug lowercases s and folds every run of non-alphanumerics into a
// single dash, trimmed at both ends. Empty-in-every-sense inputs fall
// back to "game".
func Slug(s string) string {
	var b strings.Builder
	lastDash := true // suppresses leading dashes
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if out == "" {
		return "game"
	}
	return out
}
