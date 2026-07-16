package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
	"github.com/andresbott/netcheckout/internal/marker"
	"github.com/andresbott/netcheckout/pkg/threewayrsync"
)

// ConflictError reports that some paths changed on both sides and the sync
// stopped without writing either one.
type ConflictError struct{ Paths []string }

func (e *ConflictError) Error() string {
	return (&threewayrsync.ConflictError{Paths: e.Paths}).Error()
}

// engineOptions assembles the threewayrsync options for one action run: the
// scope, the conflict policy (--force resolves local-wins), and the progress
// bridge. base is the manifest the add/modify verb is judged against.
// The engine's two mass-deletion valves (empty-endpoint, delete-fraction) stay
// armed unless the user passed --allow-deletes for this run.
func engineOptions(pf profilePlan, opts Options, base threewayrsync.Manifest) threewayrsync.Options {
	policy := threewayrsync.Abort
	if opts.Force {
		policy = threewayrsync.PreferLocal
	}
	// The marker (the cooperative lock at the remote root) is metadata, not content:
	// it must never be pulled, pushed, or counted as a discrepancy. AcceptEmpty
	// defuses the engine's unmounted-share valve so that deleting the last file(s)
	// of a tree propagates on a plain sync (pinned by the e2e suite). That is safe
	// because the dangerous cases have their own guards: the remote root provably
	// exists (the preflight read our marker off it, which an unmounted mountpoint
	// cannot contain), a MISSING local root over a non-empty baseline is a hard
	// engine error, and a mass deletion (> the engine's absolute floor and fraction)
	// still stops unless --allow-deletes waives it. Residual risk: a local root
	// that is an existing-but-unmounted mountpoint DIRECTORY over a small (≤ floor)
	// baseline reads as "tree emptied" — accepted as the cost of the UX above.
	eo := threewayrsync.Options{
		Scope:       pf.scope,
		Conflict:    policy,
		Exclude:     marker.Exclude(),
		AcceptEmpty: true,
	}
	if opts.AllowDeletes {
		eo.MaxDeleteFraction = 1 // disable the fraction guard for this run
	}
	if opts.OnApply != nil {
		eo.OnEvent = func(e threewayrsync.Event) { opts.OnApply(translateEvent(e, base)) }
	}
	return eo
}

// userFacingEngineErr rewrites the engine's delete-guard error — which points
// at a Go API option — into a message that names the CLI escape hatch.
func userFacingEngineErr(err error) error {
	var td *threewayrsync.TooManyDeletesError
	if errors.As(err, &td) {
		return fmt.Errorf("this sync would delete %d of %d previously synced file(s) — that many deletions usually mean "+
			"a renamed/moved directory or a vanished tree; if intended, re-run with --allow-deletes", td.Deletes, td.Base)
	}
	return err
}

// translateEvent maps an engine event onto the lifecycle event vocabulary: the
// side is where the change landed (a pull lands locally, a push remotely), and a
// transfer is a modify when the path was in the base manifest, an add otherwise.
func translateEvent(e threewayrsync.Event, base threewayrsync.Manifest) Event {
	switch e.Op {
	case "pull":
		return Event{Kind: transferKind(e.Path, base), Side: SideLocal, Path: e.Path}
	case "push":
		return Event{Kind: transferKind(e.Path, base), Side: SideRemote, Path: e.Path}
	case "delete-local":
		return Event{Kind: EventDelete, Side: SideLocal, Path: e.Path}
	default: // "delete-remote"
		return Event{Kind: EventDelete, Side: SideRemote, Path: e.Path}
	}
}

func transferKind(path string, base threewayrsync.Manifest) EventKind {
	if _, ok := base[path]; ok {
		return EventModify
	}
	return EventAdd
}

// fillPlan copies a dry-run plan into the report buckets.
func fillPlan(rep *Report, plan threewayrsync.Plan) {
	rep.Pulled = plan.Pull
	rep.Pushed = plan.Push
	rep.RemovedRemote = plan.RemoteDeletes
	rep.RemovedLocal = plan.LocalDeletes
	rep.Conflicts = plan.Conflicts
}

// Sync reconciles a held checkout in place, leaving the lock untouched. The
// engine loads the base through the profile's store, applies per the conflict
// policy (Abort, or local-wins with --force), and commits the merged base itself;
// lifecycle only refreshes the marker's last-sync stamp afterwards.
func (r Runner) Sync(ctx context.Context, name string, p config.Profile, id ident.Ident, relpath string, opts Options) (Report, error) {
	rep := Report{Action: "sync", DryRun: opts.DryRun}
	pf, err := r.preflightProfile(ctx, name, p, id, relpath, "sync")
	if err != nil {
		return rep, err
	}
	syncer := r.syncer(baseline.Store(name))
	eo := engineOptions(pf, opts, pf.state.Files)

	// Dry-run always exits clean: report the plan (and any would-be conflicts)
	// without writing anything, even when the plan itself has conflicts.
	if opts.DryRun {
		plan, err := syncer.Diff(ctx, pf.local, pf.remote, eo)
		if err != nil {
			return rep, userFacingEngineErr(err)
		}
		fillPlan(&rep, plan)
		rep.Marker = pf.marker
		return rep, nil
	}

	res, err := syncer.Sync(ctx, pf.local, pf.remote, eo)
	rep.Pulled = res.Applied.Pull
	rep.Pushed = res.Applied.Push
	rep.RemovedRemote = res.Applied.RemoteDeletes
	rep.RemovedLocal = res.Applied.LocalDeletes
	rep.Conflicts = res.Conflicts
	if err != nil {
		var ce *threewayrsync.ConflictError
		if errors.As(err, &ce) {
			rep.Conflicts = ce.Paths
			return rep, &ConflictError{Paths: ce.Paths}
		}
		return rep, userFacingEngineErr(err)
	}

	m := pf.marker
	m.LastSyncAt = r.now()
	if err := pf.acc.Write(ctx, m); err != nil {
		return rep, err
	}
	rep.Marker = m
	return rep, nil
}
