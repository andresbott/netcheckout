package threewayrsync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ConflictError reports that some paths changed on both sides while the policy was Abort.
type ConflictError struct{ Paths []string }

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%d conflicting path(s) changed on both sides", len(e.Paths))
}

// TooManyDeletesError reports a plan whose deletions exceed Options.MaxDeleteFraction of
// the base. A sudden mass deletion usually means an endpoint went bad (unmounted share,
// wrong path), not that the user removed most of a tree.
type TooManyDeletesError struct {
	Deletes int
	Base    int
}

func (e *TooManyDeletesError) Error() string {
	return fmt.Sprintf("refusing to delete %d of %d previously synced file(s); raise Options.MaxDeleteFraction to allow this", e.Deletes, e.Base)
}

// Result is the outcome of a Sync.
type Result struct {
	Applied   Plan     // buckets actually applied
	Conflicts []string // conflicts left unresolved (Skip policy, or deletes skipped because the file changed after planning)
	BaseSaved bool
}

// minDeleteGuard is the absolute number of deletions below which the fraction guard never
// fires: deleting 3 of 4 files is a plausible manual cleanup, deleting 300 of 400 is not.
const minDeleteGuard = 10

// maxDeleteFraction resolves the option's default: 0 means 0.5, >= 1 disables the guard.
func maxDeleteFraction(opts Options) float64 {
	if opts.MaxDeleteFraction == 0 {
		return 0.5
	}
	return opts.MaxDeleteFraction
}

// Sync enumerates, classifies, applies per the conflict policy, and commits the new base
// once at the end. It is safe to cancel via ctx and to re-run to resume: nothing is
// committed until every operation succeeds. When the Store implements Locker, the whole
// run holds the lock; a concurrent Sync fails fast with ErrLocked.
func (s *Syncer) Sync(ctx context.Context, local, remote Endpoint, opts Options) (Result, error) {
	scope, err := normalizeScope(opts.Scope)
	if err != nil {
		return Result{}, err
	}
	opts.Scope = scope

	if l, ok := s.Store.(Locker); ok {
		release, err := l.TryLock()
		if err != nil {
			return Result{}, err
		}
		defer release()
	}

	plan, base, localM, remoteM, err := s.computePlan(ctx, local, remote, opts)
	if err != nil {
		return Result{}, err
	}

	pull, push, unresolved, err := resolveConflicts(&plan, localM, remoteM, opts.Conflict)
	if err != nil {
		return Result{}, err
	}

	// The guard aims at mass deletion (a wedged endpoint), not small trees where
	// removing a couple of files legitimately exceeds any fraction — hence the absolute
	// floor. A scoped sync can only delete in-scope files, so the fraction is judged
	// against the in-scope base entries — out-of-scope entries must not dilute it. A
	// fully empty endpoint is caught separately by the EmptyEndpointError check.
	if frac := maxDeleteFraction(opts); frac < 1 {
		scopedBase := countInScope(base, opts.Scope)
		deletes := len(plan.LocalDeletes) + len(plan.RemoteDeletes)
		if scopedBase > 0 && deletes > minDeleteGuard && float64(deletes) > frac*float64(scopedBase) {
			return Result{}, &TooManyDeletesError{Deletes: deletes, Base: scopedBase}
		}
	}

	var applied Plan
	if len(pull) > 0 {
		if err := s.pullInto(ctx, remote, local, pull, opts); err != nil {
			return Result{}, err
		}
		applied.Pull = pull
	}
	if len(push) > 0 {
		if err := s.transfer(ctx, local, remote, push, opts, "push"); err != nil {
			return Result{}, err
		}
		applied.Push = push
	}
	rDeleted, rSkipped, err := s.deleteFrom(ctx, remote, plan.RemoteDeletes, remoteM)
	if err != nil {
		return Result{}, err
	}
	applied.RemoteDeletes = rDeleted
	for _, rel := range rDeleted {
		emit(opts.OnEvent, Event{Op: "delete-remote", Path: rel})
	}
	lDeleted, lSkipped, err := s.deleteFrom(ctx, local, plan.LocalDeletes, localM)
	if err != nil {
		return Result{}, err
	}
	applied.LocalDeletes = lDeleted
	for _, rel := range lDeleted {
		emit(opts.OnEvent, Event{Op: "delete-local", Path: rel})
	}
	unresolved = append(unresolved, rSkipped...)
	unresolved = append(unresolved, lSkipped...)
	sort.Strings(unresolved)

	postLocal, postRemote, err := s.postApplyManifests(ctx, local, remote, applied, localM, remoteM, opts)
	if err != nil {
		return Result{Applied: applied, Conflicts: unresolved}, err
	}
	merged := mergedBase(base, postLocal, postRemote, opts.Scope)
	if err := s.Store.SaveBase(merged); err != nil {
		return Result{Applied: applied, Conflicts: unresolved}, err
	}
	return Result{Applied: applied, Conflicts: unresolved, BaseSaved: true}, nil
}

// postApplyManifests returns the manifests to derive the new base from: a fresh listing
// of both sides when anything was applied — mtime granularity differences (e.g. a
// FAT-formatted share) would otherwise bake a state into base that matches neither side —
// or the pre-transfer manifests unchanged when nothing was, since they still are the
// live state.
func (s *Syncer) postApplyManifests(ctx context.Context, local, remote Endpoint, applied Plan, localM, remoteM Manifest, opts Options) (Manifest, Manifest, error) {
	if len(applied.Pull)+len(applied.Push)+len(applied.LocalDeletes)+len(applied.RemoteDeletes) == 0 {
		return localM, remoteM, nil
	}
	postLocal, err := s.list(ctx, local, opts.Exclude, opts.Scope)
	if err != nil {
		return nil, nil, err
	}
	postRemote, err := s.list(ctx, remote, opts.Exclude, opts.Scope)
	if err != nil {
		return nil, nil, err
	}
	return postLocal, postRemote, nil
}

// resolveConflicts folds the plan's conflicts into the work buckets per the policy and
// returns the final sorted pull/push lists plus the conflicts left unresolved. Under
// Abort any conflict is an error. PreferLocal/PreferRemote route a conflict either to a
// transfer or — when the preferred side deleted the path — to a delete on the other side
// (a transfer would no-op and resurrect it next run).
func resolveConflicts(plan *Plan, localM, remoteM Manifest, policy ConflictPolicy) (pull, push, unresolved []string, err error) {
	pull = append([]string(nil), plan.Pull...)
	push = append([]string(nil), plan.Push...)
	switch policy {
	case Abort:
		if len(plan.Conflicts) > 0 {
			return nil, nil, nil, &ConflictError{Paths: plan.Conflicts}
		}
	case Skip:
		unresolved = append(unresolved, plan.Conflicts...)
	case PreferLocal:
		for _, p := range plan.Conflicts {
			if _, ok := localM[p]; ok {
				push = append(push, p)
			} else {
				plan.RemoteDeletes = append(plan.RemoteDeletes, p)
			}
		}
	case PreferRemote:
		for _, p := range plan.Conflicts {
			if _, ok := remoteM[p]; ok {
				pull = append(pull, p)
			} else {
				plan.LocalDeletes = append(plan.LocalDeletes, p)
			}
		}
	}
	sort.Strings(pull)
	sort.Strings(push)
	sort.Strings(plan.RemoteDeletes)
	sort.Strings(plan.LocalDeletes)
	return pull, push, unresolved, nil
}

// pullInto transfers remote → local, materializing the local working copy first
// when it does not exist yet (fresh checkout): computePlan treated the missing
// dir as an empty tree, and rsync needs a real destination to write into.
func (s *Syncer) pullInto(ctx context.Context, remote, local Endpoint, pull []string, opts Options) error {
	if !local.remote() {
		if err := os.MkdirAll(local.Path, 0o755); err != nil { //nolint:gosec // G301: the local working copy is user content, not private state.
			return err
		}
	}
	return s.transfer(ctx, remote, local, pull, opts, "pull")
}

// transfer runs one rsync transfer from src to dst restricted to files, emitting a
// progress Event per itemized path when opts.OnEvent is set.
func (s *Syncer) transfer(ctx context.Context, src, dst Endpoint, files []string, opts Options, op string) error {
	listPath, err := writeFileList(files)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(listPath) }()
	args := withFilesFrom(buildTransferArgs(src, dst, opts.Checksum, opts.Exclude, opts.Scope), listPath)
	var tee io.Writer
	if opts.OnEvent != nil {
		tee = &itemizeWriter{onPath: func(p string) { opts.OnEvent(Event{Op: op, Path: p}) }}
	}
	res, err := s.runner()(ctx, s.bin(), args, tee)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &Error{Op: op, Args: args, Stderr: res.stderr, ExitCode: res.exitCode, Err: err}
	}
	return nil
}

// deleteFrom removes paths from an endpoint. On a filesystem endpoint each delete is
// guarded: the file is removed only while it still matches the state the plan was
// computed from — a file edited between enumeration and apply is skipped and reported
// (returned in skipped, resurfacing as a delete-vs-edit conflict next run) instead of
// destroyed. A remote endpoint (ssh or daemon) has no cheap stat+remove roundtrip, so its
// deletes run unguarded through one rsync call (see deleteRemote); that enumerate→delete
// window is a documented limitation. A file already gone counts as deleted, so re-running
// after a partial delete is idempotent.
func (s *Syncer) deleteFrom(ctx context.Context, e Endpoint, paths []string, planned Manifest) (deleted, skipped []string, err error) {
	if len(paths) == 0 {
		return nil, nil, nil
	}
	if !e.remote() {
		for _, rel := range paths {
			abs := filepath.Join(e.Path, filepath.FromSlash(rel))
			st, statErr := os.Stat(abs)
			switch {
			case statErr != nil && os.IsNotExist(statErr):
				deleted = append(deleted, rel)
				continue
			case statErr != nil:
				return deleted, skipped, statErr
			}
			want, ok := planned[rel]
			// The manifest's mtime has one-second resolution (rsync %M); compare at that grain.
			if !ok || st.Size() != want.Size || !st.ModTime().Truncate(time.Second).Equal(want.ModTime.Truncate(time.Second)) {
				skipped = append(skipped, rel)
				continue
			}
			if rmErr := os.Remove(abs); rmErr != nil && !os.IsNotExist(rmErr) {
				return deleted, skipped, rmErr
			}
			deleted = append(deleted, rel)
		}
		return deleted, skipped, nil
	}

	if err := s.deleteRemote(ctx, e, paths); err != nil {
		return nil, nil, err
	}
	return append([]string(nil), paths...), nil, nil
}

// deleteRemote deletes the given relative paths on a remote endpoint with a single rsync
// run: a file-less source directory plus --delete-missing-args turns each --files-from
// entry into a deletion request on the destination. One rsync-native mechanism covers ssh
// and daemon endpoints alike (a daemon has no shell to run rm through), with no ARG_MAX
// concern — the paths travel in a NUL-separated file list, not on the command line. The
// source must contain each entry's parent directory: --files-from walks the parents on
// the sending side, and a missing one is a "vanished file" that silently skips the
// deletion (observed with rsync 3.2.7, exit 24).
func (s *Syncer) deleteRemote(ctx context.Context, e Endpoint, paths []string) error {
	empty, err := os.MkdirTemp("", "threewayrsync-empty-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(empty) }()
	for _, rel := range paths {
		if dir := filepath.Dir(filepath.FromSlash(rel)); dir != "." {
			if err := os.MkdirAll(filepath.Join(empty, dir), 0o700); err != nil {
				return err
			}
		}
	}
	listPath, err := writeFileList(paths)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(listPath) }()
	args := withFilesFrom(buildDeleteArgs(empty, e), listPath)
	res, err := s.runner()(ctx, s.bin(), args, nil)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return &Error{Op: "delete", Args: args, Stderr: res.stderr, ExitCode: res.exitCode, Err: err}
	}
	return nil
}

// mergedBase derives the new base from the two post-apply manifests: a path enters the
// base only where both sides agree (size+mtime equal) — that agreement is what "last
// synced state" means. Where the sides still disagree (unresolved Skip conflicts, deletes
// skipped by the guard, mtime drift), the previous base entry is kept so the next run
// re-classifies the path and either re-applies or reports it; a disagreeing path with no
// previous base entry stays out and surfaces as a both-added conflict next run.
//
// A scoped sync's listings only see the in-scope part of the tree, so every out-of-scope
// previous base entry is carried forward unchanged — dropping them would make the next
// unscoped sync misread all of them as both-side adds.
func mergedBase(prev, postLocal, postRemote Manifest, scope []string) Manifest {
	merged := make(Manifest, len(postLocal))
	for p, lst := range postLocal {
		if rst, ok := postRemote[p]; ok && lst.Equal(rst) {
			merged[p] = lst
			continue
		}
		if bst, ok := prev[p]; ok {
			merged[p] = bst
		}
	}
	for p := range postRemote {
		if _, ok := postLocal[p]; ok {
			continue
		}
		if bst, ok := prev[p]; ok {
			merged[p] = bst
		}
	}
	if len(scope) > 0 {
		for p, bst := range prev {
			if !inScope(p, scope) {
				merged[p] = bst
			}
		}
	}
	return merged
}

func emit(onEvent func(Event), e Event) {
	if onEvent != nil {
		onEvent(e)
	}
}
