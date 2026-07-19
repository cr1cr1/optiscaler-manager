package installer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/rs/zerolog/log"

	"github.com/cr1cr1/optiscaler-manager/internal/domain"
	"github.com/cr1cr1/optiscaler-manager/internal/store"
)

// Rollback restores original state after an interrupted or failed install.
// Overwritten files are restored from verified backups; created files are
// deleted when their bytes match the manifest (or were never finished).
// Safe to re-run: completed steps are skipped.
func Rollback(ctx context.Context, st *store.Store, id string) error {
	m, err := st.Load(id)
	if err != nil {
		return err
	}
	switch m.Status {
	case domain.StatusRolledBack:
		return nil
	case domain.StatusCommitted:
		return fmt.Errorf("install %s is committed; use Uninstall", id)
	}

	for i, ow := range m.Overwritten {
		if err := ctx.Err(); err != nil {
			return err // idempotent: a later Rollback redoes completed steps safely
		}
		if ow.PreSHA256 == "" {
			continue // original was never touched before the crash
		}
		backup := filepath.Join(st.BackupDir(id), "files", ow.BackupRelPath)
		backupSHA, err := hashFile(backup)
		if err != nil {
			return fmt.Errorf("rollback %s: backup unreadable: %w", id, err)
		}
		if backupSHA != ow.PreSHA256 {
			return fmt.Errorf("rollback %s: backup of %s failed integrity check", id, ow.Path)
		}
		if _, err := copyFileFn(backup, ow.Path); err != nil {
			return fmt.Errorf("rollback restore %s: %w", ow.Path, err)
		}
		m.Overwritten[i].InstalledSHA256 = ""
		m.Ops = append(m.Ops, domain.OpEntry{Op: "restore", Path: ow.Path})
	}

	kept := m.Created[:0]
	for _, c := range m.Created {
		if err := ctx.Err(); err != nil {
			return err
		}
		if c.SHA256 == "" {
			_ = os.Remove(c.Path) // partial write, ours by definition
			continue
		}
		current, err := hashFile(c.Path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("rollback inspect %s: %w", c.Path, err)
		}
		if current == c.SHA256 {
			if err := os.Remove(c.Path); err != nil {
				return fmt.Errorf("rollback delete %s: %w", c.Path, err)
			}
			m.Ops = append(m.Ops, domain.OpEntry{Op: "delete", Path: c.Path})
			continue
		}
		log.Warn().Str("path", c.Path).Msg("rollback: file changed since install, leaving in place")
		kept = append(kept, c)
	}
	m.Created = kept
	m.CreatedDirs = removeEmptyDirs(m.CreatedDirs)

	m.Status = domain.StatusRolledBack
	return st.Save(m)
}

// RefusedError reports files Uninstall declined to touch because their bytes
// no longer match the manifest (foreign modifications are never deleted).
type RefusedError struct {
	Paths []string
}

func (e *RefusedError) Error() string {
	return fmt.Sprintf("uninstall refused %d changed file(s): %v", len(e.Paths), e.Paths)
}

// Uninstall reverses a committed install: created files are deleted and
// overwritten files restored, but only where current bytes match what the
// manifest recorded. Refusals abort with RefusedError and leave the manifest
// committed (already-processed entries are dropped, so retry resumes).
// On full success the manifest and backup dir are removed.
func Uninstall(ctx context.Context, st *store.Store, id string) error {
	m, err := st.Load(id)
	if err != nil {
		return err
	}
	if m.Status == domain.StatusRolledBack {
		return nil
	}
	if m.Status != domain.StatusCommitted {
		return fmt.Errorf("install %s is %s; only committed installs can be uninstalled", id, m.Status)
	}

	var refused []string

	kept := m.Created[:0]
	for i, c := range m.Created {
		if err := ctx.Err(); err != nil {
			return abortUninstall(ctx, st, m, append(kept, m.Created[i:]...), m.Overwritten)
		}
		current, err := hashFile(c.Path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("uninstall inspect %s: %w", c.Path, err)
		}
		if current == c.SHA256 {
			if err := os.Remove(c.Path); err != nil {
				return fmt.Errorf("uninstall delete %s: %w", c.Path, err)
			}
			m.Ops = append(m.Ops, domain.OpEntry{Op: "delete", Path: c.Path})
			continue
		}
		refused = append(refused, c.Path)
		kept = append(kept, c)
	}
	m.Created = kept

	keptOw := m.Overwritten[:0]
	for i, ow := range m.Overwritten {
		if err := ctx.Err(); err != nil {
			return abortUninstall(ctx, st, m, m.Created, append(keptOw, m.Overwritten[i:]...))
		}
		current, err := hashFile(ow.Path)
		switch {
		case os.IsNotExist(err):
			// Our installed file vanished; restoring the original is still right.
		case err != nil:
			return fmt.Errorf("uninstall inspect %s: %w", ow.Path, err)
		case current != ow.InstalledSHA256:
			refused = append(refused, ow.Path)
			keptOw = append(keptOw, ow)
			continue
		}
		backup := filepath.Join(st.BackupDir(id), "files", ow.BackupRelPath)
		backupSHA, err := hashFile(backup)
		if err != nil {
			return fmt.Errorf("uninstall %s: backup unreadable: %w", id, err)
		}
		if backupSHA != ow.PreSHA256 {
			return fmt.Errorf("uninstall %s: backup of %s failed integrity check", id, ow.Path)
		}
		if _, err := copyFileFn(backup, ow.Path); err != nil {
			return fmt.Errorf("uninstall restore %s: %w", ow.Path, err)
		}
		m.Ops = append(m.Ops, domain.OpEntry{Op: "restore", Path: ow.Path})
	}
	m.Overwritten = keptOw
	m.CreatedDirs = removeEmptyDirs(m.CreatedDirs)

	if len(refused) > 0 {
		m.LastError = "uninstall refused changed files"
		if err := st.Save(m); err != nil {
			return err
		}
		sort.Strings(refused)
		return &RefusedError{Paths: refused}
	}

	// Cancel boundary (pre-cleanup): everything processable is already
	// reversed; persist progress so a retry resumes instead of redoing.
	if err := ctx.Err(); err != nil {
		return abortUninstall(ctx, st, m, m.Created, m.Overwritten)
	}

	if err := os.RemoveAll(st.BackupDir(id)); err != nil {
		return fmt.Errorf("remove backups %s: %w", id, err)
	}
	return st.Delete(id)
}

// abortUninstall persists uninstall progress on cancellation: processed
// entries are already dropped from the retained slices, so a retry resumes
// where this run stopped. The manifest stays committed and the returned
// error is the context cause.
func abortUninstall(ctx context.Context, st *store.Store, m *domain.Manifest, created []domain.CreatedEntry, overwritten []domain.OverwrittenEntry) error {
	cause := ctx.Err()
	log.Warn().Str("id", m.ID).Err(cause).Msg("uninstall cancelled; progress persisted for resume")
	m.Created = created
	m.Overwritten = overwritten
	m.LastError = cause.Error()
	if err := st.Save(m); err != nil {
		return fmt.Errorf("%w (persist uninstall progress: %v)", cause, err)
	}
	return cause
}

// removeEmptyDirs deletes recorded created dirs that are now empty, deepest
// first, and returns the ones that remain.
func removeEmptyDirs(dirs []string) []string {
	sorted := make([]string, len(dirs))
	copy(sorted, dirs)
	sort.Sort(sort.Reverse(sort.StringSlice(sorted)))
	var kept []string
	for _, d := range sorted {
		if err := os.Remove(d); err != nil {
			kept = append(kept, d) // non-empty or otherwise unremovable
		}
	}
	sort.Strings(kept)
	return kept
}
