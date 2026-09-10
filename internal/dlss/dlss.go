// Package dlss updates the three NVIDIA DLSS runtime DLLs on explicit user action.
package dlss

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/jsoncache"
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

// Label is the display name of a snapshot: the backed-up DLSS Super
// Resolution version plus the local creation time.
func (s Snapshot) Label() string {
	version := "unknown"
	for _, f := range s.Files {
		if f.Name == "nvngx_dlss.dll" {
			version = f.Version
			break
		}
	}
	return "DLSS " + version + " · " + s.CreatedAt.Local().Format("2006-01-02 15:04")
}

// Update downloads all three files at one immutable NVIDIA commit, backs up
// the current complete set, then replaces the files. A failed replacement
// restores every original before returning. Downloads are cached per commit
// under cacheRoot (same layout as the OptiScaler bundle cache — fetch once
// per version — plus a SHA-256 manifest).
func Update(ctx context.Context, c *Client, cacheRoot, dataRoot, gameDir string) (Snapshot, error) {
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
	cached, err := c.ensureCache(ctx, cacheRoot, commit)
	if err != nil {
		return Snapshot{}, err
	}
	for _, name := range Files {
		if _, err := pever.FileVersion(filepath.Join(cached, name)); err != nil {
			return Snapshot{}, fmt.Errorf("dlss: invalid downloaded %s: %w", name, err)
		}
	}
	snap, err := backup(dataRoot, gameDir, commit)
	if err != nil {
		return Snapshot{}, err
	}
	for _, name := range Files {
		if err := ctx.Err(); err != nil {
			if rerr := restoreFiles(dataRoot, gameDir, snap); rerr != nil {
				return Snapshot{}, errors.Join(err, rerr)
			}
			return Snapshot{}, err
		}
		if _, err := copyHashed(filepath.Join(cached, name), filepath.Join(gameDir, name)); err != nil {
			if rerr := restoreFiles(dataRoot, gameDir, snap); rerr != nil {
				return Snapshot{}, errors.Join(err, rerr)
			}
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
		if rerr := restoreFiles(dataRoot, gameDir, current); rerr != nil {
			return Snapshot{}, errors.Join(err, rerr)
		}
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
		if err != nil {
			// A partial backup (crash mid-copy) is never restorable; say so
			// instead of silently dropping it.
			log.Warn().Err(err).Str("snapshot", e.Name()).Msg("dlss: dropping unloadable snapshot")
			continue
		}
		out = append(out, s)
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
	defer func() { _ = resp.Body.Close() }()
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

// ensureCache ensures the commit-keyed download cache holds a complete
// file set, fetching only the missing or hash-failed members through the
// same raw-file routine as before (same source, same destination shape as
// the OptiScaler bundle cache). A manifest.json pins each member's
// SHA-256, so a tampered cache file is refetched, never installed. A
// freshly downloaded member is PE-validated BEFORE its hash is recorded:
// a lying 200 must never earn a manifest entry, or the hash gate would
// legitimize the garbage on every later update.
func (c *Client) ensureCache(ctx context.Context, cacheRoot, commit string) (string, error) {
	if cacheRoot == "" {
		return "", fmt.Errorf("dlss: no download cache root")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dir := filepath.Join(cacheRoot, "dlss", commit)
	rec, err := loadCacheRecord(dir)
	if err != nil {
		// An unreadable or corrupt manifest degrades to a full refetch: the
		// cache is re-derivable, never precious.
		log.Warn().Err(err).Msg("dlss: unreadable cache manifest, refetching")
		rec = cacheRecord{}
	}
	files := make(map[string]string, len(Files))
	fetched := false
	for _, name := range Files {
		if want := rec.Files[name]; want != "" {
			if h, err := fileSHA256(filepath.Join(dir, name)); err == nil && h == want {
				files[name] = h
				continue // verified cache hit
			}
		}
		h, err := c.download(ctx, commit, name, filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		if _, err := pever.FileVersion(filepath.Join(dir, name)); err != nil {
			return "", fmt.Errorf("dlss: invalid downloaded %s: %w", name, err)
		}
		files[name] = h
		fetched = true
	}
	if fetched {
		if err := jsoncache.Write(filepath.Join(dir, "manifest.json"), cacheRecord{Files: files}); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// cacheRecord pins the expected SHA-256 of every member of one cached
// commit.
type cacheRecord struct {
	Files map[string]string `json:"files"`
}

// loadCacheRecord reads the cache manifest; a missing one is an empty
// record (full refetch), a corrupt one an error the caller degrades.
func loadCacheRecord(dir string) (cacheRecord, error) {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if os.IsNotExist(err) {
		return cacheRecord{}, nil
	}
	if err != nil {
		return cacheRecord{}, err
	}
	var r cacheRecord
	if err := json.Unmarshal(data, &r); err != nil {
		return cacheRecord{}, fmt.Errorf("dlss: parse cache manifest: %w", err)
	}
	return r, nil
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
	defer func() { _ = resp.Body.Close() }()
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
		v, _ := pever.FileVersion(path)
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
	// Re-verify the persisted snapshot before any caller swaps a file: the
	// recorded hashes describe the copy stream, but only the bytes on disk
	// protect the originals (installer invariant 3 precedent).
	for _, f := range s.Files {
		h, err := fileSHA256(filepath.Join(dir, f.Name))
		if err != nil {
			_ = os.RemoveAll(dir)
			return Snapshot{}, fmt.Errorf("dlss: snapshot verify %s: %w", f.Name, err)
		}
		if h != f.SHA256 {
			_ = os.RemoveAll(dir)
			return Snapshot{}, fmt.Errorf("dlss: snapshot verify %s: hash mismatch", f.Name)
		}
	}
	return s, nil
}

// restoreFiles copies a snapshot back into the game dir. Every member is
// SHA-verified against the snapshot record BEFORE the first copy, and a
// record with a missing hash is refused: a tampered snapshot.json that
// blanked its hashes must not bypass the gate.
func restoreFiles(dataRoot, gameDir string, s Snapshot) error {
	for _, f := range s.Files {
		path := filepath.Join(snapshotsDir(dataRoot, gameDir), s.ID, f.Name)
		h, err := fileSHA256(path)
		if err != nil {
			return err
		}
		if f.SHA256 == "" || h != f.SHA256 {
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
		_ = tmp.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	if err := os.Rename(name, dest); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
