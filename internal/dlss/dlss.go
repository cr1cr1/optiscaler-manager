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
	"github.com/cr1cr1/optiscaler-manager/internal/version"
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

// Latest is the published NVIDIA DLSS runtime version as seen at check
// time. Version is the newest tag ("310.9.1" form); Commit is the commit
// that tag points at — the cache-key hint Update installs from.
type Latest struct {
	Version string
	Commit  string
}

// Update installs the NVIDIA runtime set for gameDir from the download
// cache: a commit hint whose cache dir is complete (the startup check's
// published commit already fetched) is served with zero network; anything
// else resolves main's head, fetches missing members into the cache, and
// installs from that cache dir — never straight from the network. The
// current set is backed up first when it is complete; a partial or absent
// current set only warns and the update proceeds WITHOUT a rollback
// backup (snapshots are all-or-nothing, so an incomplete set has no
// rollback value). A failed replacement restores every original when a
// backup exists. Downloads are cached per commit under cacheRoot (same
// layout as the OptiScaler bundle cache — fetch once per version — plus a
// SHA-256 manifest).
func Update(ctx context.Context, c *Client, cacheRoot, dataRoot, gameDir, commitHint string) (Snapshot, error) {
	if c == nil {
		return Snapshot{}, fmt.Errorf("dlss: no download client")
	}
	commit := commitHint
	if commit == "" || !cacheComplete(cacheRoot, commit) {
		resolved, err := c.commit(ctx)
		if err != nil {
			return Snapshot{}, err
		}
		commit = resolved
	}
	cached, err := c.ensureCache(ctx, cacheRoot, commit)
	if err != nil {
		return Snapshot{}, err
	}
	targetVersion := ""
	for _, name := range Files {
		v, err := pever.FileVersion(filepath.Join(cached, name))
		if err != nil {
			return Snapshot{}, fmt.Errorf("dlss: invalid downloaded %s: %w", name, err)
		}
		if name == Files[0] { // the main DLL carries the set's version
			targetVersion = v
		}
	}
	// Already latest? A press must not reinstall the same set over itself
	// (and pile up a duplicate backup for it): byte-identical members — the
	// cache's own digests — or a readable applied version at or above the
	// target settle as a graceful no-op. An unreadable applied version is
	// NOT provably current, so the update proceeds (the bare-label case).
	if sameSet(gameDir, cached) {
		return Snapshot{}, &AlreadyLatestError{Version: targetVersion}
	}
	if applied, err := pever.FileVersion(filepath.Join(gameDir, Files[0])); err == nil && version.Compare(applied, targetVersion) >= 0 {
		return Snapshot{}, &AlreadyLatestError{Version: applied}
	}
	snap, err := backupIfComplete(dataRoot, gameDir, commit)
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

// Restore backs up the current set first when it is complete (a partial or
// absent current set only warns and the restore proceeds without a rollback
// backup), then restores the chosen prior set.
func Restore(ctx context.Context, dataRoot, gameDir, id string) (Snapshot, error) {
	target, err := load(dataRoot, gameDir, id)
	if err != nil {
		return Snapshot{}, err
	}
	current, err := backupIfComplete(dataRoot, gameDir, "")
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

// isHexSHA reports whether s is a 40-char hex commit SHA — the only shape
// the update flow accepts out of the GitHub API.
func isHexSHA(s string) bool {
	return len(s) == 40 && strings.Trim(s, "0123456789abcdef") == ""
}

// dlssCacheDir is the download-cache layout for one commit, sibling of the
// OptiScaler bundle cache under the shared cacheDir root. An empty commit
// yields the dlss cache root itself.
func dlssCacheDir(cacheRoot, commit string) string {
	return filepath.Join(cacheRoot, "dlss", commit)
}

// Latest resolves the newest published DLSS runtime version WITHOUT
// downloading runtime bytes: one small tags-API call returns the newest
// tag and the commit it points at. This is the startup check half of the
// update flow; the runtime files themselves are only ever fetched on user
// action (Update).
func (c *Client) Latest(ctx context.Context) (Latest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase+"/repos/NVIDIA/DLSS/tags?per_page=10", nil)
	if err != nil {
		return Latest{}, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Latest{}, fmt.Errorf("dlss: resolve published version: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Latest{}, fmt.Errorf("dlss: resolve published version: unexpected HTTP %d", resp.StatusCode)
	}
	var body []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Latest{}, fmt.Errorf("dlss: decode published version: %w", err)
	}
	// Newest-first is not a documented GitHub contract: pick the greatest
	// version among the fetched tags. A tag whose version or commit SHA
	// does not parse is not a servable candidate; no candidate at all is
	// the error.
	best := -1
	for i, tag := range body {
		name := strings.TrimPrefix(tag.Name, "v")
		if name == "" || !isHexSHA(tag.Commit.SHA) {
			continue
		}
		if best == -1 || version.Compare(name, strings.TrimPrefix(body[best].Name, "v")) > 0 {
			best = i
		}
	}
	if best == -1 {
		return Latest{}, fmt.Errorf("dlss: no servable published versions")
	}
	return Latest{Version: strings.TrimPrefix(body[best].Name, "v"), Commit: body[best].Commit.SHA}, nil
}

// cacheComplete reports whether a commit's cache dir holds a manifest entry
// and a regular file for every runtime member. Existence-only, the same
// reuse check as the OptiScaler bundle cache — ensureCache re-hashes and
// PE-validates every member before any install, so the display/hint path
// needs no digest pass.
// ponytail: existence-only completeness; a corrupted member costs one
// refetch at press time, caught by the same gate that already guards it.
func cacheComplete(cacheRoot, commit string) bool {
	if cacheRoot == "" {
		return false
	}
	rec, err := loadCacheRecord(dlssCacheDir(cacheRoot, commit))
	if err != nil {
		return false
	}
	for _, name := range Files {
		if rec.Files[name] == "" {
			return false
		}
		st, err := os.Stat(filepath.Join(dlssCacheDir(cacheRoot, commit), name))
		if err != nil || !st.Mode().IsRegular() {
			return false
		}
	}
	return true
}

// CachedVersion returns the newest version available in the local download
// cache ("" when none) and the commit dir that holds it: every complete
// commit dir's nvngx_dlss.dll is PE-read and the greatest version wins.
// Local-only — the startup check's cached half never touches the network;
// the commit lets an offline press serve the cache without resolving
// online first.
// ponytail: PE-reads every complete cache dir; the cache holds a handful
// of commits, so the bounded reads stay trivial. Version-pin them in the
// manifest if this ever grows.
func CachedVersion(cacheRoot string) (string, string) {
	entries, err := os.ReadDir(dlssCacheDir(cacheRoot, ""))
	if err != nil {
		return "", ""
	}
	best, bestCommit := "", ""
	for _, e := range entries {
		if !e.IsDir() || !cacheComplete(cacheRoot, e.Name()) {
			continue
		}
		v, err := pever.FileVersion(filepath.Join(dlssCacheDir(cacheRoot, e.Name()), Files[0]))
		if err != nil {
			// Unreadable cache bytes can't be displayed or served; the
			// next update refetches them anyway.
			log.Warn().Err(err).Str("dir", e.Name()).Msg("dlss: cached DLL unreadable")
			continue
		}
		if best == "" || version.Compare(v, best) > 0 {
			best, bestCommit = v, e.Name()
		}
	}
	return best, bestCommit
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
	dir := dlssCacheDir(cacheRoot, commit)
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

// Preload is the exported ensureCache for the startup pre-warm: it makes
// the download cache hold the commit's complete three-file set, fetching
// only the missing or hash-failed members (zero network when the set is
// already complete). The session's startup check uses it to keep the
// latest published set offline-ready for the DLSS pill.
func (c *Client) Preload(ctx context.Context, cacheRoot, commit string) error {
	if commit == "" {
		return fmt.Errorf("dlss: no commit to preload")
	}
	_, err := c.ensureCache(ctx, cacheRoot, commit)
	return err
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

// Complete reports whether gameDir holds the complete regular-file NVIDIA
// runtime set — the scan-layer gate that decides whether the DLSS pill is
// the interactive update control or a static badge.
func Complete(gameDir string) bool { return requireFiles(gameDir) == nil }

// AlreadyLatestError reports that the game's NVIDIA runtime already holds
// the update target: a press settles as a graceful no-op instead of
// reinstalling the same set over itself.
type AlreadyLatestError struct {
	Version string // the version the installed set already holds
}

func (e *AlreadyLatestError) Error() string {
	return fmt.Sprintf("dlss: already at %s", e.Version)
}

// sameSet reports whether the game's runtime set is byte-identical to the
// cached one: every member's SHA-256 equals the cached member's manifest
// digest. Any read or record failure yields false — an unreadable set is
// not provably identical, so the update proceeds.
func sameSet(gameDir, cacheDir string) bool {
	rec, err := loadCacheRecord(cacheDir)
	if err != nil {
		return false
	}
	for _, name := range Files {
		want := rec.Files[name]
		if want == "" {
			return false
		}
		if h, err := fileSHA256(filepath.Join(gameDir, name)); err != nil || h != want {
			return false
		}
	}
	return true
}

// backupIfComplete backs up the current set only when every member exists.
// A partial or absent current set downgrades to a warning and a zero
// Snapshot: snapshots are all-or-nothing (load refuses partial records), so
// an incomplete set has no rollback value and the caller proceeds WITHOUT a
// backup — the operation still installs/restores the complete target set.
// The zero Snapshot also neutralizes the caller's rollback-on-failure leg:
// restoreFiles over zero files is a no-op.
func backupIfComplete(dataRoot, gameDir, source string) (Snapshot, error) {
	if err := requireFiles(gameDir); err != nil {
		log.Warn().Err(err).Str("gameDir", gameDir).
			Msg("dlss: current set incomplete; proceeding without a rollback backup")
		return Snapshot{}, nil
	}
	return backup(dataRoot, gameDir, source)
}

func backup(dataRoot, gameDir, source string) (Snapshot, error) {
	// The current set is hashed first so a digest-identical prior snapshot
	// can be reused untouched (update/restore ping-pong must not pile up
	// duplicate ~115 MB dirs).
	// ponytail: on a dedup miss the files are read twice (this hash probe,
	// then copyHashed); stream-copy with a digest-on-the-fly if that ever
	// matters.
	current := make([]File, 0, len(Files))
	for _, name := range Files {
		path := filepath.Join(gameDir, name)
		v, _ := pever.FileVersion(path)
		h, err := fileSHA256(path)
		if err != nil {
			return Snapshot{}, fmt.Errorf("dlss: backup %s: %w", name, err)
		}
		current = append(current, File{Name: name, Version: v, SHA256: h})
	}
	if prior := identicalSnapshot(dataRoot, gameDir, current); prior.ID != "" {
		return prior, nil
	}
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

// identicalSnapshot returns an existing snapshot whose members carry
// exactly the current set's digests (zero value when none): update and
// restore ping-pong reuses it instead of writing a duplicate backup. A
// candidate's stored bytes are re-verified against its record before
// reuse — a tampered snapshot dir must never alias into a rollback.
// ponytail: linear scan of the game's snapshot index per backup — the list
// stays a handful of entries since dedup keeps it that way; index by
// digest set if a user ever accumulates dozens.
func identicalSnapshot(dataRoot, gameDir string, current []File) Snapshot {
	prior, err := Snapshots(dataRoot, gameDir)
	if err != nil {
		log.Warn().Err(err).Msg("dlss: snapshot index unreadable, writing a fresh backup")
		return Snapshot{}
	}
	recorded := make(map[string]string, len(current))
	for _, f := range current {
		recorded[f.Name] = f.SHA256
	}
	for _, s := range prior {
		if len(s.Files) != len(current) {
			continue
		}
		match := true
		for _, f := range s.Files {
			if recorded[f.Name] != f.SHA256 {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		usable := true
		for _, f := range s.Files {
			h, err := fileSHA256(filepath.Join(snapshotsDir(dataRoot, gameDir), s.ID, f.Name))
			if err != nil || h != f.SHA256 {
				usable = false
				break
			}
		}
		if usable {
			return s
		}
		log.Warn().Str("snapshot", s.ID).Msg("dlss: dedup candidate failed verification, writing a fresh backup")
	}
	return Snapshot{}
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
