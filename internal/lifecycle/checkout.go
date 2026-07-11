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
	"github.com/andresbott/netcheckout/internal/rsync"
)

// Checkout pulls remote -> local for the scoped relpath, writes the per-profile
// marker, and records the baseline. relpath scopes which files copy; the lock is
// always the whole profile. An existing foreign marker refuses unless Force.
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
	held := exists && existing.OwnedBy(id.By, id.Host)
	if exists && !held && !opts.Force {
		return rep, fmt.Errorf("profile %q is checked out by %s on %s since %s (use --force to override)",
			name, existing.CheckedOutBy, existing.Host, existing.CheckedOutAt.Format("2006-01-02 15:04"))
	}

	rel := normalizeRelpath(relpath)
	src := filepath.Join(remoteRoot, rel)
	dst := filepath.Join(localRoot, rel)
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

	res, err := r.Syncer.Sync(ctx, job)
	if err != nil {
		return rep, err // transfer failed: no marker, no baseline
	}
	for _, c := range res.Changes {
		rep.Pulled = append(rep.Pulled, c.Path)
	}

	var relpaths []string
	if held {
		relpaths = mergeRelpath(existing.Relpaths, rel)
	} else {
		relpaths = []string{rel}
	}

	files, err := baseline.Snapshot(localRoot, relpaths)
	if err != nil {
		return rep, err
	}
	now := r.now()
	b := &baseline.Baseline{Profile: name, Relpaths: relpaths, Files: files, LastSyncAt: now}
	if err := baseline.Save(b); err != nil {
		return rep, err
	}

	checkedOutAt := now
	if held {
		checkedOutAt = existing.CheckedOutAt
	}
	m := &marker.Marker{
		CheckedOutBy: id.By,
		Profile:      name,
		Host:         id.Host,
		Relpaths:     relpaths,
		CheckedOutAt: checkedOutAt,
		LastSyncAt:   now,
		ToolVersion:  r.ToolVersion,
	}
	if err := marker.Write(remoteRoot, m); err != nil {
		if !held {
			_ = baseline.Remove(name) // roll back a fresh checkout's baseline
		}
		return rep, err
	}
	rep.Marker = m
	return rep, nil
}
