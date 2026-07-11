// Package status computes whether a profile's local and remote roots are in
// sync, using rsync dry runs in both directions.
package status

import (
	"context"
	"fmt"
	"os"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/rsync"
	"github.com/andresbott/netcheckout/internal/sanity"
)

// Differ is satisfied by *rsync.Syncer; lets tests inject a fake.
type Differ interface {
	Diff(ctx context.Context, j rsync.Job) (rsync.Diff, error)
}

// TargetStatus is the bidirectional diff for one target of a profile: the
// whole root when no subpaths are declared, or one declared subpath.
type TargetStatus struct {
	Subpath      string
	Pull         rsync.Diff // remote -> local: what a checkout/sync-down would change
	Push         rsync.Diff // local -> remote: what a check-in would change
	LocalMissing bool       // Local doesn't exist yet; Push was not attempted
}

// InSync reports whether this target has no pending changes in either
// direction. When LocalMissing, only Pull is meaningful (Push was skipped).
func (t TargetStatus) InSync() bool {
	if t.LocalMissing {
		return t.Pull.InSync
	}
	return t.Pull.InSync && t.Push.InSync
}

// Label is a human-readable name for this target: "(root)" for the whole
// root, or the declared subpath.
func (t TargetStatus) Label() string {
	return label(t.Subpath)
}

func label(subpath string) string {
	if subpath == "" {
		return "(root)"
	}
	return subpath
}

func computeTarget(ctx context.Context, d Differ, t config.Target) (TargetStatus, error) {
	_, statErr := os.Stat(t.Local)
	localMissing := os.IsNotExist(statErr)

	pull, err := d.Diff(ctx, rsync.Job{
		Local:     rsync.Endpoint{Path: t.Local},
		Remote:    rsync.Endpoint{Path: t.Remote},
		Direction: rsync.Pull,
	})
	if err != nil {
		return TargetStatus{}, fmt.Errorf("%s: pull diff: %w", label(t.Subpath), err)
	}

	if localMissing {
		return TargetStatus{Subpath: t.Subpath, Pull: pull, LocalMissing: true}, nil
	}

	push, err := d.Diff(ctx, rsync.Job{
		Local:     rsync.Endpoint{Path: t.Local},
		Remote:    rsync.Endpoint{Path: t.Remote},
		Direction: rsync.Push,
	})
	if err != nil {
		return TargetStatus{}, fmt.Errorf("%s: push diff: %w", label(t.Subpath), err)
	}
	return TargetStatus{Subpath: t.Subpath, Pull: pull, Push: push}, nil
}

// ProfileStatus is the status across every target of a profile.
type ProfileStatus struct {
	CheckedOut bool // a checkout marker is present; false means Compute stopped early and Targets is empty
	Targets    []TargetStatus
}

// InSync reports whether every target of the profile is in sync.
func (p ProfileStatus) InSync() bool {
	for _, t := range p.Targets {
		if !t.InSync() {
			return false
		}
	}
	return true
}

// Compute runs Pull and Push dry-run diffs for every target the profile
// resolves to. It returns an error only for a real failure (the remote root
// missing, invalid declared subpaths, or the differ itself erroring) --
// finding differences is a normal result captured in the returned
// ProfileStatus, not an error. On a mid-loop failure the targets computed so
// far are still returned alongside the error.
func Compute(ctx context.Context, d Differ, p config.Profile) (ProfileStatus, error) {
	remoteRoot := config.ExpandRoot(p.RemoteRoot)
	if info, err := os.Stat(remoteRoot); err != nil || !info.IsDir() {
		return ProfileStatus{}, fmt.Errorf("remote root %s is not mounted", remoteRoot)
	}

	targets, err := p.Targets()
	if err != nil {
		return ProfileStatus{}, err
	}

	// A profile that is not checked out has nothing to diff: stop early rather
	// than running rsync. "checked out" is the marker check from internal/sanity
	// (aggregated across targets). Ordering matters: the remote-mounted and
	// invalid-subpath errors above still take precedence.
	if !sanity.Check(p).CheckedOut {
		return ProfileStatus{CheckedOut: false}, nil
	}

	out := ProfileStatus{Targets: make([]TargetStatus, 0, len(targets)), CheckedOut: true}
	for _, t := range targets {
		ts, err := computeTarget(ctx, d, t)
		if err != nil {
			return out, err
		}
		out.Targets = append(out.Targets, ts)
	}
	return out, nil
}
