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
	"github.com/andresbott/netcheckout/internal/reconcile"
	"github.com/andresbott/netcheckout/internal/rsync"
)

// Checkout pulls remote -> local for the scoped relpath, writes the per-profile
// marker, and records the baseline. relpath scopes which files copy; the lock is
// always the whole profile. An existing foreign marker refuses unless Force.
// When opts.OnApply is set (and not a dry-run), each pulled file is streamed live
// as a SideLocal reconcile.Event, matching Sync/Checkin.
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
	src := filepath.Join(remoteRoot, rel)
	dst := filepath.Join(localRoot, rel)

	// Refuse to check out over an existing local copy. This guard is absolute:
	// unlike the lock check, --force does not bypass it.
	if err := ensureLocalTargetVacant(dst, name); err != nil {
		return rep, err
	}

	job := rsync.Job{
		Local:     rsync.Endpoint{Path: dst},
		Remote:    rsync.Endpoint{Path: src},
		Direction: rsync.Pull,
	}

	if opts.DryRun {
		d, err := r.Syncer.Diff(ctx, job)
		if err != nil {
			return rep, err
		}
		for _, c := range d.Changes {
			rep.Pulled = append(rep.Pulled, c.Path)
		}
		return rep, nil
	}

	if opts.OnApply != nil {
		job.OnChange = reconcile.PullEmitter(opts.OnApply)
	}
	res, err := r.Syncer.Sync(ctx, job)
	if err != nil {
		return rep, err // transfer failed: no marker, no baseline
	}
	for _, c := range res.Changes {
		rep.Pulled = append(rep.Pulled, c.Path)
	}

	relpaths := []string{rel}

	files, err := baseline.Snapshot(localRoot, relpaths)
	if err != nil {
		return rep, err
	}
	now := r.now()
	b := &baseline.Baseline{Profile: name, Relpaths: relpaths, Files: files, LastSyncAt: now}
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
