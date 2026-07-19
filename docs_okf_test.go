package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// okfReserved are docs/ files exempt from the OKF `type` frontmatter rule.
var okfReserved = map[string]bool{
	"index.md": true,
	"log.md":   true,
}

// TestDocsOKFFrontmatter asserts OKF compliance for docs/:
// every non-reserved .md file under docs/ must start with YAML frontmatter
// that declares a non-empty `type:` key. Reserved files (index.md, log.md)
// are exempt per the OKF spec.
func TestDocsOKFFrontmatter(t *testing.T) {
	entries, err := os.ReadDir("docs")
	if err != nil {
		t.Fatalf("docs/ must exist and be readable: %v", err)
	}

	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".md") || okfReserved[name] {
			continue
		}
		checked++
		data, err := os.ReadFile(filepath.Join("docs", name))
		if err != nil {
			t.Errorf("read docs/%s: %v", name, err)
			continue
		}
		if typ := frontmatterType(string(data)); typ == "" {
			t.Errorf("docs/%s: missing or empty `type` in YAML frontmatter (OKF)", name)
		} else {
			t.Logf("docs/%s: type %q", name, typ)
		}
	}
	if checked == 0 {
		t.Fatal("docs/ contains no non-reserved .md files; OKF requires typed knowledge files")
	}
}

// frontmatterType extracts the value of the `type:` key from leading YAML
// frontmatter, or "" if absent. Only the flat `key: value` form is supported;
// nested YAML is out of scope for OKF compliance.
func frontmatterType(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return ""
	}
	rest := content[len("---\n"):]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return ""
	}
	for line := range strings.Lines(rest[:end]) {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) == "type" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
