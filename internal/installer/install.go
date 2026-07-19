// Package installer performs transactional OptiScaler installs: plan → stage
// → validate → backup → copy → manifest, with rollback and SHA-verified
// uninstall. Invariants live in docs/safety.md; the tests here enforce them.
package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/archive"
	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/profile"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// injectionDLL is the name OptiScaler.dll takes in the game directory.
const injectionDLL = "dxgi.dll"

// requiredBaseNames is the post-extract validation set (0.9.4 bundle ground
// truth; see docs/log.md M2d).
var requiredBaseNames = []string{"optiscaler.dll", "fakenvapi.dll", "fakenvapi.ini"}

// copyFileFn is the file-copy seam for fault-injection tests (white-box only).
var copyFileFn = copyFile

// Request describes one install operation.
type Request struct {
	GameRoot         string // canonical game root
	InstallDir       string // injection target, must be inside GameRoot
	ArchivePath      string // downloaded bundle .7z
	RequestedVersion string // as asked ("latest" or tag)
	Resolved         domain.ResolvedAsset
}

// filePlan maps one archive member to its destination relative to InstallDir.
type filePlan struct {
	srcRel string
	dstRel string
}

// Install runs the full transaction and returns the committed manifest.
// A leftover in_progress/failed manifest for the same target is rolled back
// first; a committed one is an error (update-in-place is v0.2 territory).
func Install(ctx context.Context, st *store.Store, req Request) (*domain.Manifest, error) {
	gameRoot, err := canonicalPath(req.GameRoot)
	if err != nil {
		return nil, fmt.Errorf("game root: %w", err)
	}
	installDir, err := canonicalPath(req.InstallDir)
	if err != nil {
		return nil, fmt.Errorf("install dir: %w", err)
	}
	if installDir != gameRoot && !strings.HasPrefix(installDir, gameRoot+string(os.PathSeparator)) {
		return nil, fmt.Errorf("install dir %s is not inside game root %s", installDir, gameRoot)
	}
	if st_, err := os.Stat(installDir); err != nil || !st_.IsDir() {
		return nil, fmt.Errorf("install dir %s: %w", installDir, err)
	}

	id := domain.ManifestID(installDir)
	if existing, err := st.Load(id); err == nil {
		switch existing.Status {
		case domain.StatusCommitted:
			return nil, fmt.Errorf("already installed at %s; uninstall first", installDir)
		case domain.StatusInProgress, domain.StatusFailed:
			// Leftover cleanup is not op work: it runs to completion even
			// when the op context is already dead (docs/safety.md —
			// cancellation invariant). WithoutCancel keeps the caller's
			// values while detaching from cancellation.
			if err := Rollback(context.WithoutCancel(ctx), st, id); err != nil {
				return nil, fmt.Errorf("rollback leftover install: %w", err)
			}
		}
	}

	// Cancel boundary: nothing has been staged or written yet, so returning
	// the bare cause leaves the filesystem exactly as found.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	names, err := archive.List(req.ArchivePath)
	if err != nil {
		return nil, err
	}
	plan, err := buildPlan(names)
	if err != nil {
		return nil, err
	}

	staging := st.StagingDir(id)
	if err := os.RemoveAll(staging); err != nil {
		return nil, fmt.Errorf("clean staging: %w", err)
	}
	if err := archive.ExtractTo(req.ArchivePath, staging); err != nil {
		return nil, err
	}

	resolved := req.Resolved
	for _, fp := range plan {
		if strings.EqualFold(filepath.Base(fp.srcRel), "OptiScaler.dll") && resolved.SHA256 == "" {
			digest, err := hashFile(filepath.Join(staging, fp.srcRel))
			if err != nil {
				return nil, fmt.Errorf("hash staged OptiScaler.dll: %w", err)
			}
			resolved.SHA256 = digest
		}
	}

	// Cancel boundary (post-extract): staging is ours and the game dir is
	// still untouched (invariant 1), so dropping staging and returning the
	// bare cause leaves no partial state.
	if err := ctx.Err(); err != nil {
		_ = os.RemoveAll(staging)
		return nil, err
	}

	m := &domain.Manifest{
		ID:               id,
		SchemaVersion:    domain.SchemaVersion,
		Status:           domain.StatusInProgress,
		GameRoot:         gameRoot,
		InstallDir:       installDir,
		RequestedVersion: req.RequestedVersion,
		Resolved:         resolved,
		CreatedAt:        time.Now().UTC(),
	}
	// Manifest lands before the first destructive write (safety invariant 2).
	if err := st.Save(m); err != nil {
		return nil, err
	}

	backupFiles := filepath.Join(st.BackupDir(id), "files")
	for _, fp := range plan {
		if err := ctx.Err(); err != nil {
			return cancelInstall(ctx, st, m)
		}
		src := filepath.Join(staging, fp.srcRel)
		dst := filepath.Join(installDir, fp.dstRel)
		if _, err := os.Stat(dst); err == nil {
			if err := installOverwrite(st, m, src, dst, backupFiles, fp.dstRel); err != nil {
				return fail(st, m, err)
			}
		} else {
			if err := installCreate(st, m, src, dst, installDir); err != nil {
				return fail(st, m, err)
			}
		}
		if err := st.Save(m); err != nil {
			return fail(st, m, err)
		}
	}

	if err := applyCuratedINI(st, m); err != nil {
		return fail(st, m, err)
	}

	// Cancel boundary (post-INI write, pre-commit): the swap is done but not
	// committed, so the cancellation rolls everything back.
	if err := ctx.Err(); err != nil {
		return cancelInstall(ctx, st, m)
	}

	m.Status = domain.StatusCommitted
	if err := st.Save(m); err != nil {
		return fail(st, m, err)
	}
	// The manifest is committed — that is the truth. A staging-cleanup
	// failure afterwards is housekeeping, not an op failure.
	if err := os.RemoveAll(staging); err != nil {
		log.Warn().Str("id", m.ID).Err(err).Msg("clean staging after commit")
	}
	return m, nil
}

// installOverwrite backs up the original bytes before replacing them. Entry
// fields are filled progressively so a crash at any point leaves a manifest
// Rollback can interpret (empty PreSHA256 = original never touched).
func installOverwrite(st *store.Store, m *domain.Manifest, src, dst, backupFiles, rel string) error {
	entry := domain.OverwrittenEntry{Path: dst, BackupRelPath: rel}
	m.Overwritten = append(m.Overwritten, entry)
	idx := len(m.Overwritten) - 1
	if err := st.Save(m); err != nil {
		return err
	}

	preSHA, err := hashFile(dst)
	if err != nil {
		return fmt.Errorf("hash original %s: %w", dst, err)
	}
	backupPath := filepath.Join(backupFiles, rel)
	if _, err := copyFileFn(dst, backupPath); err != nil {
		return fmt.Errorf("backup %s: %w", dst, err)
	}
	backupSHA, err := hashFile(backupPath)
	if err != nil {
		return fmt.Errorf("verify backup %s: %w", backupPath, err)
	}
	if backupSHA != preSHA {
		return fmt.Errorf("backup of %s failed verification", dst)
	}
	m.Overwritten[idx].PreSHA256 = preSHA
	if err := st.Save(m); err != nil {
		return err
	}

	written, err := copyFileFn(src, dst)
	if err != nil {
		return fmt.Errorf("install %s: %w", dst, err)
	}
	m.Overwritten[idx].InstalledSHA256 = written
	m.Ops = append(m.Ops, domain.OpEntry{Op: "overwrite", Path: dst})
	return nil
}

// installCreate copies a file that did not exist before. The entry is
// registered before the copy (empty SHA256) so Rollback can delete partial
// writes unconditionally.
func installCreate(st *store.Store, m *domain.Manifest, src, dst, installDir string) error {
	entry := domain.CreatedEntry{Path: dst}
	m.Created = append(m.Created, entry)
	idx := len(m.Created) - 1
	trackCreatedDirs(m, filepath.Dir(dst), installDir)
	if err := st.Save(m); err != nil {
		return err
	}

	written, err := copyFileFn(src, dst)
	if err != nil {
		return fmt.Errorf("install %s: %w", dst, err)
	}
	m.Created[idx].SHA256 = written
	m.Ops = append(m.Ops, domain.OpEntry{Op: "create", Path: dst})
	return nil
}

// trackCreatedDirs records directories between installDir and dir that do not
// exist yet, so uninstall/rollback can remove the empty ones later.
func trackCreatedDirs(m *domain.Manifest, dir, installDir string) {
	for dir != installDir && strings.HasPrefix(dir, installDir+string(os.PathSeparator)) {
		if _, err := os.Stat(dir); err == nil {
			return // first existing ancestor; deeper ones are already recorded
		}
		if !contains(m.CreatedDirs, dir) {
			m.CreatedDirs = append(m.CreatedDirs, dir)
		}
		dir = filepath.Dir(dir)
	}
}

// applyCuratedINI replaces the bundle's OptiScaler.ini with the curated
// safe-defaults profile and refreshes the manifest entry to match.
func applyCuratedINI(st *store.Store, m *domain.Manifest) error {
	iniPath := filepath.Join(m.InstallDir, "OptiScaler.ini")
	tmp, err := os.CreateTemp(m.InstallDir, ".optiscaler-*.ini")
	if err != nil {
		return fmt.Errorf("stage curated ini: %w", err)
	}
	if err := profile.WriteDefaultINI(tmp); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write curated ini: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("write curated ini: %w", err)
	}
	if err := os.Rename(tmp.Name(), iniPath); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("install curated ini: %w", err)
	}

	digest, err := hashFile(iniPath)
	if err != nil {
		return err
	}
	for i := range m.Created {
		if m.Created[i].Path == iniPath {
			m.Created[i].SHA256 = digest
			m.Ops = append(m.Ops, domain.OpEntry{Op: "profile", Path: iniPath})
			return st.Save(m)
		}
	}
	for i := range m.Overwritten {
		if m.Overwritten[i].Path == iniPath {
			m.Overwritten[i].InstalledSHA256 = digest
			m.Ops = append(m.Ops, domain.OpEntry{Op: "profile", Path: iniPath})
			return st.Save(m)
		}
	}
	return fmt.Errorf("curated ini %s not tracked in manifest", iniPath)
}

// fail marks the manifest failed and persists it before returning the error.
func fail(st *store.Store, m *domain.Manifest, cause error) (*domain.Manifest, error) {
	m.Status = domain.StatusFailed
	m.LastError = cause.Error()
	if err := st.Save(m); err != nil {
		return m, fmt.Errorf("mark install failed: %w (original error: %v)", err, cause)
	}
	return m, cause
}

// cancelInstall settles a cancelled install atomically: the manifest is
// marked failed with the cancellation recorded (crash-recovery evidence),
// then rolled back to the pre-op state so zero partial files remain in the
// game dir. Rollback runs under context.WithoutCancel: the op context is
// dead by definition, but cleanup belongs to the same atomic operation and
// must run to completion (docs/safety.md — cancellation invariant). The
// returned error always unwraps to the context cause.
func cancelInstall(ctx context.Context, st *store.Store, m *domain.Manifest) (*domain.Manifest, error) {
	cause := ctx.Err()
	log.Warn().Str("id", m.ID).Str("installDir", m.InstallDir).Err(cause).
		Msg("install cancelled; marking failed and rolling back")
	m.Status = domain.StatusFailed
	m.LastError = cause.Error()
	if err := st.Save(m); err != nil {
		return m, fmt.Errorf("%w (mark cancelled install failed: %v)", cause, err)
	}
	if err := Rollback(context.WithoutCancel(ctx), st, m.ID); err != nil {
		return m, fmt.Errorf("%w (rollback after cancel: %v)", cause, err)
	}
	if err := os.RemoveAll(st.StagingDir(m.ID)); err != nil {
		log.Warn().Str("id", m.ID).Err(err).Msg("clean staging after cancel")
	}
	return m, cause
}

// ManifestIDFor returns the manifest ID for an install directory,
// canonicalizing the path the same way Install does.
func ManifestIDFor(installDir string) (string, error) {
	c, err := canonicalPath(installDir)
	if err != nil {
		return "", err
	}
	return domain.ManifestID(c), nil
}

// buildPlan validates raw archive member names and maps them to destinations.
// Directory members are skipped; OptiScaler.dll is renamed to the injection
// DLL. This is the plan-time hostile-input gate (safety invariant 1).
func buildPlan(names []string) ([]filePlan, error) {
	required := map[string]bool{}
	for _, base := range requiredBaseNames {
		required[base] = false
	}
	seen := map[string]bool{}
	var plan []filePlan

	for _, n := range names {
		if strings.HasSuffix(n, "/") {
			continue
		}
		rel, err := archive.SanitizeName(n)
		if err != nil {
			return nil, fmt.Errorf("unsafe archive: %w", err)
		}
		dstRel := rel
		base := strings.ToLower(filepath.Base(rel))
		if _, tracked := required[base]; tracked {
			required[base] = true
		}
		if base == "optiscaler.dll" {
			dstRel = injectionDLL
		}
		key := strings.ToLower(dstRel)
		if seen[key] {
			return nil, fmt.Errorf("unsafe archive: duplicate destination %q", dstRel)
		}
		seen[key] = true
		plan = append(plan, filePlan{srcRel: rel, dstRel: dstRel})
	}
	for base, found := range required {
		if !found {
			return nil, fmt.Errorf("bundle is missing required file %q", base)
		}
	}
	return plan, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
