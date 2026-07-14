package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
	"github.com/andresbott/netcheckout/internal/marker"
)

// Checkout locks a profile: it verifies the remote is mounted and not already
// held elsewhere, refuses to lock over a non-empty local target, then records an
// empty baseline and writes the per-profile marker. It copies NO files — pulling
// the remote down is sync's job. relpath scopes the recorded relpaths (and thus
// what the first sync reconciles); the lock is always the whole profile. An
// existing foreign marker refuses unless Force.
func (r Runner) Checkout(ctx context.Context, name string, p config.Profile, id ident.Ident, relpath string, opts Options) (Report, error) {
	rep := Report{Action: "checkout", DryRun: opts.DryRun}
	remoteRoot := config.ExpandRoot(p.RemoteRoot)
	localRoot := config.ExpandRoot(p.LocalRoot)

	if info, err := os.Stat(remoteRoot); err != nil || !info.IsDir() {
		return rep, fmt.Errorf("remote root %s is not mounted", remoteRoot)
	}

	existing, exists, err := marker.Read(remoteRoot)
	if err != nil {
		return rep, err
	}
	if exists && existing.OwnedBy(id.By, id.Host) {
		return rep, fmt.Errorf("profile %q is already checked out on this machine — use sync to reconcile", name)
	}
	if exists && !opts.Force {
		return rep, fmt.Errorf("profile %q is checked out by %s on %s since %s (use --force to override)",
			name, existing.CheckedOutBy, existing.Host, existing.CheckedOutAt.Format("2006-01-02 15:04"))
	}

	rel := normalizeRelpath(relpath)
	dst := filepath.Join(localRoot, rel)

	// Refuse to lock over an existing local copy. This guard is absolute: unlike
	// the lock check, --force does not bypass it. Keeping the target empty means
	// the first sync after checkout is an unambiguous pull, not a wall of
	// "added on both sides" conflicts against the empty baseline recorded below.
	if err := ensureLocalTargetVacant(dst, name); err != nil {
		return rep, err
	}

	// Dry-run: the checks passed and nothing is written. Checkout copies no files,
	// so there is nothing to preview beyond "would write the marker".
	if opts.DryRun {
		return rep, nil
	}

	// Record an EMPTY baseline. The local target is vacant (guard above), so there
	// is nothing to snapshot; the empty manifest makes the first sync classify
	// every remote file as a fresh pull. Snapshotting the remote here instead
	// would make sync read the empty local as a baseline-scoped delete and wipe
	// the remote — so the baseline must be empty, never the remote's contents.
	relpaths := []string{rel}
	now := r.now()
	b := &baseline.Baseline{Profile: name, Relpaths: relpaths, Files: map[string]baseline.FileState{}, LastSyncAt: now}
	if err := baseline.Save(b); err != nil {
		return rep, err
	}

	m := &marker.Marker{
		CheckedOutBy: id.By,
		Profile:      name,
		Host:         id.Host,
		Relpaths:     relpaths,
		CheckedOutAt: now,
		LastSyncAt:   now,
		ToolVersion:  r.ToolVersion,
	}
	if err := marker.Write(remoteRoot, m); err != nil {
		_ = baseline.Remove(name) // roll back the fresh checkout's baseline
		return rep, err
	}
	rep.Marker = m
	return rep, nil
}

// ensureLocalTargetVacant returns an error if the checkout destination already
// holds content: a plain file, or a non-empty directory blocks. An empty
// directory or a missing path is fine. This is the absolute local-copy guard —
// callers must not let --force bypass it.
func ensureLocalTargetVacant(dst, name string) error {
	info, err := os.Stat(dst)
	if err != nil {
		return nil // missing path (or unreadable) — nothing to clobber
	}
	if !info.IsDir() {
		return fmt.Errorf("local target %s already exists — remove it before checking out %q", dst, name)
	}
	entries, err := os.ReadDir(dst)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("local target %s is not empty — remove its contents before checking out %q", dst, name)
	}
	return nil
}
