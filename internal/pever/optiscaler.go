package pever

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
)

// logBanner extracts the version from the OptiScaler log banner line,
// e.g. "OptiScaler v0.9.4-pre2 (build abc)".
var logBanner = regexp.MustCompile(`OptiScaler v([0-9][^\s]*)`)

// logScanLines caps how far into OptiScaler.log the banner may appear.
const logScanLines = 10

// OptiScalerVersion reports the OptiScaler version installed in dir using
// a chain of evidence:
//
//  1. manifest.json ("version" field);
//  2. OptiScaler.log banner within the first 10 lines;
//  3. OptiScaler.ini presence → installed but version unknown;
//
// It returns "" whenever the version cannot be determined.
func OptiScalerVersion(dir string) string {
	if v := manifestVersion(filepath.Join(dir, "manifest.json")); v != "" {
		return v
	}
	if v := logVersion(filepath.Join(dir, "OptiScaler.log")); v != "" {
		return v
	}
	return "" // OptiScaler.ini presence only proves an install, not a version.
}

func manifestVersion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var m struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		return ""
	}
	return m.Version
}

func logVersion(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	for i := 0; i < logScanLines && sc.Scan(); i++ {
		if m := logBanner.FindStringSubmatch(sc.Text()); m != nil {
			return m[1]
		}
	}
	return ""
}
