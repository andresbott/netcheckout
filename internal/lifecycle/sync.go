package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
	"github.com/andresbott/netcheckout/internal/marker"
	"github.com/andresbott/netcheckout/internal/reconcile"
)

// reconcileProfile is the shared body of Sync and Checkin: lock+baseline checks,
// classify, dry-run, apply, and baseline/marker refresh. It does NOT remove the
// marker (Checkin does that after a clean return). relpaths is the scope.
func (r Runner) reconcileProfile(ctx context.Context, name string, p config.Profile, id ident.Ident, relpath string, opts Options, rep *Report) (*marker.Marker, *baseline.Baseline, error) {
	remoteRoot := config.ExpandRoot(p.RemoteRoot)
	localRoot := config.ExpandRoot(p.LocalRoot)

	if info, err := os.Stat(remoteRoot); err != nil || !info.IsDir() {
		return nil, nil, fmt.Errorf("remote root %s is not mounted", remoteRoot)
	}

	m, exists, err := marker.Read(remoteRoot)
	if err != nil {
		return nil, nil, err
	}
	if !exists {
		return nil, nil, fmt.Errorf("profile %q is not checked out (no marker)", name)
	}
	if !m.OwnedBy(id.By, id.Host) && !opts.Force {
		return nil, nil, fmt.Errorf("profile %q is checked out by %s on %s (not this machine)", name, m.CheckedOutBy, m.Host)
	}

	b, hasBase, err := baseline.Load(name)
	if err != nil {
		return nil, nil, err
	}
	if !hasBase {
		return nil, nil, fmt.Errorf("no local baseline for %q — re-checkout on this machine to establish one", name)
	}

	relpaths := b.Relpaths
	if rel := normalizeRelpath(relpath); rel != "." {
		relpaths = []string{rel}
	}

	localScan, err := baseline.Scan(localRoot, relpaths)
	if err != nil {
		return nil, nil, err
	}
	remoteScan, err := baseline.Scan(remoteRoot, relpaths)
	if err != nil {
		return nil, nil, err
	}

	plan, err := reconcile.Classify(b.Files, localScan, remoteScan, localRoot, remoteRoot)
	if err != nil {
		return nil, nil, err
	}
	rep.Conflicts = plan.Conflicts

	if len(plan.Conflicts) > 0 && !opts.Force {
		return nil, nil, &reconcile.ConflictError{Paths: plan.Conflicts}
	}

	if opts.DryRun {
		rep.Pulled = plan.Pull
		rep.Pushed = plan.Push
		rep.RemovedRemote = plan.RemoteDeletes
		rep.RemovedLocal = plan.LocalDeletes
		return m, b, nil
	}

	applied, err := reconcile.Apply(ctx, r.Syncer, localRoot, remoteRoot, plan, opts.Force)
	if err != nil {
		var ce *reconcile.ConflictError
		if errors.As(err, &ce) {
			rep.Conflicts = ce.Paths
		}
		return nil, nil, err
	}
	rep.Pulled = applied.Pulled
	rep.Pushed = applied.Pushed
	rep.RemovedRemote = applied.RemovedRemote
	rep.RemovedLocal = applied.RemovedLocal

	// Re-snapshot the baseline to the reconciled state and bump last_sync_at.
	now := r.now()
	files, err := baseline.Snapshot(localRoot, b.Relpaths)
	if err != nil {
		return nil, nil, err
	}
	nb := &baseline.Baseline{Profile: name, Relpaths: b.Relpaths, Files: files, LastSyncAt: now}
	if err := baseline.Save(nb); err != nil {
		return nil, nil, err
	}
	m.LastSyncAt = now
	return m, nb, nil
}

// Sync reconciles a held checkout in place, leaving the lock untouched.
func (r Runner) Sync(ctx context.Context, name string, p config.Profile, id ident.Ident, relpath string, opts Options) (Report, error) {
	rep := Report{Action: "sync", DryRun: opts.DryRun}
	m, _, err := r.reconcileProfile(ctx, name, p, id, relpath, opts, &rep)
	if err != nil {
		return rep, err
	}
	if !opts.DryRun {
		if err := marker.Write(config.ExpandRoot(p.RemoteRoot), m); err != nil {
			return rep, err
		}
	}
	rep.Marker = m
	return rep, nil
}
