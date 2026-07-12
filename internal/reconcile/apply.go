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

// EventKind is the verb of a single applied change, mirroring the status view.
type EventKind int

const (
	EventAdd    EventKind = iota // a new file appeared on the destination side
	EventModify                  // an existing file's contents were updated
	EventDelete                  // a file was removed from the destination side
)

// Side is the endpoint an applied change landed on.
type Side int

const (
	SideLocal  Side = iota // change applied under the local root
	SideRemote             // change applied under the remote root
)

// Event is one applied change, emitted live as Apply carries it out so callers
// can render progress in the same "verb → side  path" shape as the status view.
type Event struct {
	Kind EventKind
	Side Side
	Path string
}

// kindFromType maps an rsync itemized change type to an apply EventKind. Apply's
// jobs never pass --delete, so rsync only ever reports Created/Modified here;
// deletes are emitted separately from the explicit os.Remove loops.
func kindFromType(t rsync.ChangeType) EventKind {
	if t == rsync.Created {
		return EventAdd
	}
	return EventModify
}

// Apply executes a plan. With a conflict and !force it returns *ConflictError
// before writing anything. force reclassifies conflicts as pushes (local wins).
// onEvent, when non-nil, is called once per applied change (pulls and pushes as
// rsync streams them, then each delete) so callers can show live progress.
func Apply(ctx context.Context, s Syncer, localRoot, remoteRoot string, p Plan, force bool, onEvent func(Event)) (Applied, error) {
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
			OnChange:  changeEmitter(onEvent, SideLocal),
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
			OnChange:  changeEmitter(onEvent, SideRemote),
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
		emit(onEvent, Event{Kind: EventDelete, Side: SideRemote, Path: rel})
	}
	for _, rel := range p.LocalDeletes {
		if err := os.Remove(filepath.Join(localRoot, filepath.FromSlash(rel))); err != nil && !os.IsNotExist(err) {
			return applied, err
		}
		applied.RemovedLocal = append(applied.RemovedLocal, rel)
		emit(onEvent, Event{Kind: EventDelete, Side: SideLocal, Path: rel})
	}

	return applied, nil
}

// changeEmitter adapts an apply-event callback into the rsync per-change callback
// for a job landing on side. It returns nil when onEvent is nil so no callback is
// installed on the job.
func changeEmitter(onEvent func(Event), side Side) func(rsync.Change) {
	if onEvent == nil {
		return nil
	}
	return func(c rsync.Change) {
		onEvent(Event{Kind: kindFromType(c.Type), Side: side, Path: c.Path})
	}
}

// PullEmitter adapts an apply-event callback into the rsync per-change callback
// for a remote→local pull (checkout), landing every change on SideLocal. It
// returns nil when onEvent is nil so no callback is installed on the job.
func PullEmitter(onEvent func(Event)) func(rsync.Change) {
	return changeEmitter(onEvent, SideLocal)
}

func emit(onEvent func(Event), e Event) {
	if onEvent != nil {
		onEvent(e)
	}
}
