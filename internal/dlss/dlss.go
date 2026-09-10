// Package dlss updates the three NVIDIA DLSS runtime DLLs on explicit user action.
package dlss

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/cr1cr1/optiscaler-manager/internal/pever"
)

// Files is the NVIDIA DLSS runtime set. Updates and restores always handle it together.
var Files = []string{"nvngx_dlss.dll", "nvngx_dlssd.dll", "nvngx_dlssg.dll"}

const relPath = "lib/Windows_x86_64/rel"

// Client obtains the NVIDIA DLSS runtime files from GitHub.
type Client struct {
	http    *http.Client
	apiBase string
	rawBase string
}

// New constructs the production client.
func New(httpClient *http.Client) *Client {
	return NewWithBaseURLs(httpClient, "https://api.github.com", "https://raw.githubusercontent.com")
}

// NewWithBaseURLs constructs a client with test endpoints.
func NewWithBaseURLs(httpClient *http.Client, apiBase, rawBase string) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{http: httpClient, apiBase: strings.TrimRight(apiBase, "/"), rawBase: strings.TrimRight(rawBase, "/")}
}

// Snapshot is one complete, externally stored set of old NVIDIA DLL bytes.
type Snapshot struct {
	ID           string    `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	SourceCommit string    `json:"source_commit,omitempty"`
	Files        []File    `json:"files"`
}

// File records one member's version and checksum in a snapshot.
type File struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// Update downloads all three files at one immutable NVIDIA commit, backs up
// the current complete set, then replaces the files. A failed replacement
// restores every original before returning.
func Update(ctx context.Context, c *Client, dataRoot, gameDir string) (Snapshot, error) {
	if err := requireFiles(gameDir); err != nil {
		return Snapshot{}, err
	}
	if c == nil {
		return Snapshot{}, fmt.Errorf("dlss: no download client")
	}
	commit, err := c.commit(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	stagingRoot := filepath.Join(dataRoot, "dlss-staging")
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		return Snapshot{}, err
	}
	stage, err := os.MkdirTemp(stagingRoot, "update-*")
	if err != nil {
		return Snapshot{}, err
	}
	defer os.RemoveAll(stage)
	for _, name := range Files {
		if _, err := c.download(ctx, commit, name, filepath.Join(stage, name)); err != nil {
			return Snapshot{}, err
		}
		if _, err := fileVersion(filepath.Join(stage, name)); err != nil {
			return Snapshot{}, fmt.Errorf("dlss: invalid downloaded %s: %w", name, err)
		}
	}
	snap, err := backup(dataRoot, gameDir, commit)
	if err != nil {
		return Snapshot{}, err
	}
	for _, name := range Files {
		if err := ctx.Err(); err != nil {
			_ = restoreFiles(dataRoot, gameDir, snap)
			return Snapshot{}, err
		}
		if _, err := copyHashed(filepath.Join(stage, name), filepath.Join(gameDir, name)); err != nil {
			_ = restoreFiles(dataRoot, gameDir, snap)
			return Snapshot{}, err
		}
	}
	return snap, nil
}

// Restore backs up the current set first, then restores the chosen prior set.
func Restore(ctx context.Context, dataRoot, gameDir, id string) (Snapshot, error) {
	if err := requireFiles(gameDir); err != nil {
		return Snapshot{}, err
	}
	target, err := load(dataRoot, gameDir, id)
	if err != nil {
		return Snapshot{}, err
	}
	current, err := backup(dataRoot, gameDir, "")
	if err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	if err := restoreFiles(dataRoot, gameDir, target); err != nil {
		_ = restoreFiles(dataRoot, gameDir, current)
		return Snapshot{}, err
	}
	return current, nil
}

// Snapshots returns prior sets newest first.
func Snapshots(dataRoot, gameDir string) ([]Snapshot, error) {
	dir := snapshotsDir(dataRoot, gameDir)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]Snapshot, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		s, err := load(dataRoot, gameDir, e.Name())
		if err == nil {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (c *Client) commit(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+"/repos/NVIDIA/DLSS/commits/main", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("dlss: resolve source: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("dlss: resolve source: unexpected HTTP %d", resp.StatusCode)
	}
	var body struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("dlss: decode source: %w", err)
	}
	if len(body.SHA) != 40 || strings.Trim(body.SHA, "0123456789abcdef") != "" {
		return "", fmt.Errorf("dlss: invalid source commit %q", body.SHA)
	}
	return body.SHA, nil
}

func (c *Client) download(ctx context.Context, commit, name, dest string) (string, error) {
	url := c.rawBase + "/NVIDIA/DLSS/" + commit + "/" + relPath + "/" + name
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("dlss: download %s: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("dlss: download %s: unexpected HTTP %d", name, resp.StatusCode)
	}
	return copyReaderHashed(resp.Body, dest)
}

func requireFiles(gameDir string) error {
	for _, name := range Files {
		st, err := os.Stat(filepath.Join(gameDir, name))
		if err != nil || !st.Mode().IsRegular() {
			return fmt.Errorf("dlss: %s is missing", name)
		}
	}
	return nil
}

func backup(dataRoot, gameDir, source string) (Snapshot, error) {
	id := fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	dir := filepath.Join(snapshotsDir(dataRoot, gameDir), id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Snapshot{}, err
	}
	s := Snapshot{ID: id, CreatedAt: time.Now().UTC(), SourceCommit: source}
	for _, name := range Files {
		path := filepath.Join(gameDir, name)
		// ponytail: an unreadable version resource only degrades the menu
		// label to "unknown" — the backup's fidelity is the bytes, so a
		// stripped-resource DLL must not block an update.
		v, _ := fileVersion(path)
		h, err := copyHashed(path, filepath.Join(dir, name))
		if err != nil {
			return Snapshot{}, err
		}
		s.Files = append(s.Files, File{Name: name, Version: v, SHA256: h})
	}
	data, err := json.Marshal(s)
	if err != nil {
		return Snapshot{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "snapshot.json"), data, 0o600); err != nil {
		return Snapshot{}, err
	}
	return s, nil
}

// restoreFiles copies a snapshot back into the game dir. Every member is
// SHA-verified against the snapshot record BEFORE the first copy, so a
// corrupted or tampered backup can never reach the game directory half-way.
func restoreFiles(dataRoot, gameDir string, s Snapshot) error {
	for _, f := range s.Files {
		path := filepath.Join(snapshotsDir(dataRoot, gameDir), s.ID, f.Name)
		h, err := fileSHA256(path)
		if err != nil {
			return err
		}
		if f.SHA256 != "" && h != f.SHA256 {
			return fmt.Errorf("dlss: backup %s failed verification", f.Name)
		}
	}
	for _, f := range s.Files {
		if _, err := copyHashed(filepath.Join(snapshotsDir(dataRoot, gameDir), s.ID, f.Name), filepath.Join(gameDir, f.Name)); err != nil {
			return err
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func load(dataRoot, gameDir, id string) (Snapshot, error) {
	data, err := os.ReadFile(filepath.Join(snapshotsDir(dataRoot, gameDir), id, "snapshot.json"))
	if err != nil {
		return Snapshot{}, fmt.Errorf("dlss: load snapshot: %w", err)
	}
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return Snapshot{}, fmt.Errorf("dlss: parse snapshot: %w", err)
	}
	if s.ID != id || len(s.Files) != len(Files) {
		return Snapshot{}, fmt.Errorf("dlss: invalid snapshot %q", id)
	}
	return s, nil
}

func snapshotsDir(dataRoot, gameDir string) string {
	return filepath.Join(dataRoot, "dlss-backups", snapshotGameID(gameDir))
}
func snapshotGameID(gameDir string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(gameDir)))
	return hex.EncodeToString(sum[:])[:16]
}
func fileVersion(path string) (string, error) { return pever.FileVersion(path) }

func copyHashed(src, dest string) (string, error) {
	in, err := os.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	return copyReaderHashed(in, dest)
}

func copyReaderHashed(r io.Reader, dest string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".dlss-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), r); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", err
	}
	if err := os.Rename(name, dest); err != nil {
		os.Remove(name)
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
