package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
	"github.com/andresbott/netcheckout/internal/marker"
	"github.com/andresbott/netcheckout/internal/reconcile"
	"github.com/andresbott/netcheckout/internal/sanity"
)

// reconcileProfile is the shared body of Sync and Checkin: lock+baseline checks,
// classify, dry-run, apply, and baseline/marker refresh. It does NOT remove the
// marker (Checkin does that after a clean return). relpaths is the scope.
func (r Runner) reconcileProfile(ctx context.Context, name string, p config.Profile, id ident.Ident, relpath string, opts Options, rep *Report) (*marker.Marker, *baseline.Baseline, error) {
	// Pre-flight: refuse if local content lies outside the declared subpaths, so a
	// scoped push never silently skips local work. Runs before any mount/transfer;
	// --force does not bypass it (this is data safety, not a lock override).
	// A walk error (permissions, I/O, etc.) fails the operation closed — unlike
	// status/Check, which swallow walk errors best-effort to report what it can.
	if unlisted, err := sanity.UnlistedLocal(p); err != nil {
		return nil, nil, err
	} else if len(unlisted) > 0 {
		return nil, nil, fmt.Errorf(
			"refusing to %s %q: local content is outside the profile's subpaths and would not be synced (add it to subpaths or remove it):\n  %s",
			rep.Action, name, strings.Join(unlisted, "\n  "))
	}

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

	plan, err := reconcile.PlanFor(b.Files, localRoot, remoteRoot, relpaths)
	if err != nil {
		return nil, nil, err
	}
	rep.Conflicts = plan.Conflicts

	// Dry-run always exits clean: report the plan (and any would-be conflicts)
	// without writing anything, even when the plan itself has conflicts.
	if opts.DryRun {
		rep.Pulled = plan.Pull
		rep.Pushed = plan.Push
		rep.RemovedRemote = plan.RemoteDeletes
		rep.RemovedLocal = plan.LocalDeletes
		return m, b, nil
	}

	if len(plan.Conflicts) > 0 && !opts.Force {
		return nil, nil, &reconcile.ConflictError{Paths: plan.Conflicts}
	}

	// Nothing stopped us: either there were no conflicts, or --force resolved
	// them (local-wins, folded into the pushes by Apply). Clear rep.Conflicts
	// so the report reflects what actually happened, not what was classified.
	rep.Conflicts = nil

	applied, err := reconcile.Apply(ctx, r.Syncer, localRoot, remoteRoot, plan, opts.Force, opts.OnApply)
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
	// Only the reconciled (possibly relpath-narrowed) scope is re-scanned; the
	// fresh scoped snapshot is merged into the existing baseline rather than
	// replacing it wholesale, so out-of-scope entries (files outside relpaths
	// on a scoped sync) are left untouched instead of being overwritten with
	// their current — possibly un-synced — local content.
	now := r.now()
	files, err := baseline.Snapshot(localRoot, relpaths)
	if err != nil {
		return nil, nil, err
	}
	mergedFiles := mergeScopedSnapshot(b.Files, files, relpaths)
	nb := &baseline.Baseline{Profile: name, Relpaths: b.Relpaths, Files: mergedFiles, LastSyncAt: now}
	if err := baseline.Save(nb); err != nil {
		return nil, nil, err
	}
	m.LastSyncAt = now
	return m, nb, nil
}

// mergeScopedSnapshot merges a freshly-scanned scoped snapshot into an existing
// baseline manifest: every existing key that falls under any of relpaths is
// dropped (so in-scope deletions are reflected), then the scoped snapshot's
// keys are overlaid. Keys outside relpaths are left untouched. When relpaths
// covers the whole tree (e.g. ["."]), this is equivalent to replacing existing
// wholesale with scoped.
func mergeScopedSnapshot(existing, scoped map[string]baseline.FileState, relpaths []string) map[string]baseline.FileState {
	merged := make(map[string]baseline.FileState, len(existing)+len(scoped))
	for k, v := range existing {
		if !underAnyRelpath(k, relpaths) {
			merged[k] = v
		}
	}
	for k, v := range scoped {
		merged[k] = v
	}
	return merged
}

// underAnyRelpath reports whether key (a slash path relative to root) falls
// under any of relpaths. "." matches everything; otherwise a match is an exact
// path or path-prefix match on "/".
func underAnyRelpath(key string, relpaths []string) bool {
	for _, rp := range relpaths {
		if rp == "." || key == rp || strings.HasPrefix(key, rp+"/") {
			return true
		}
	}
	return false
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
