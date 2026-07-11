package reconcile

import (
	"context"
	"os"
	"path/filepath"

	"github.com/andresbott/netcheckout/internal/rsync"
)

// Syncer is the rsync surface Apply needs; *rsync.Syncer satisfies it (as does
// lifecycle.Syncer — the two interfaces are structurally identical).
type Syncer interface {
	Sync(ctx context.Context, j rsync.Job) (rsync.Result, error)
	Diff(ctx context.Context, j rsync.Job) (rsync.Diff, error)
}

// Applied records what Apply actually did.
type Applied struct {
	Pulled        []string
	Pushed        []string
	RemovedRemote []string
	RemovedLocal  []string
}

// Apply executes a plan. With a conflict and !force it returns *ConflictError
// before writing anything. force reclassifies conflicts as pushes (local wins).
func Apply(ctx context.Context, s Syncer, localRoot, remoteRoot string, p Plan, force bool) (Applied, error) {
	if len(p.Conflicts) > 0 && !force {
		return Applied{}, &ConflictError{Paths: p.Conflicts}
	}
	push := p.Push
	if force {
		push = append(append([]string{}, p.Push...), p.Conflicts...)
	}

	var applied Applied

	if len(p.Pull) > 0 {
		if _, err := s.Sync(ctx, rsync.Job{
			Local:     rsync.Endpoint{Path: localRoot},
			Remote:    rsync.Endpoint{Path: remoteRoot},
			Direction: rsync.Pull,
			Files:     p.Pull,
		}); err != nil {
			return applied, err
		}
		applied.Pulled = p.Pull
	}

	if len(push) > 0 {
		if _, err := s.Sync(ctx, rsync.Job{
			Local:     rsync.Endpoint{Path: localRoot},
			Remote:    rsync.Endpoint{Path: remoteRoot},
			Direction: rsync.Push,
			Files:     push,
		}); err != nil {
			return applied, err
		}
		applied.Pushed = push
	}

	for _, rel := range p.RemoteDeletes {
		if err := os.Remove(filepath.Join(remoteRoot, filepath.FromSlash(rel))); err != nil && !os.IsNotExist(err) {
			return applied, err
		}
		applied.RemovedRemote = append(applied.RemovedRemote, rel)
	}
	for _, rel := range p.LocalDeletes {
		if err := os.Remove(filepath.Join(localRoot, filepath.FromSlash(rel))); err != nil && !os.IsNotExist(err) {
			return applied, err
		}
		applied.RemovedLocal = append(applied.RemovedLocal, rel)
	}

	return applied, nil
}
