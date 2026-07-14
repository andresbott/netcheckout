package threewayrsync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// ConflictError reports that some paths changed on both sides while the policy was Abort.
type ConflictError struct{ Paths []string }

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%d conflicting path(s) changed on both sides", len(e.Paths))
}

// Result is the outcome of a Sync.
type Result struct {
	Applied   Plan     // buckets actually applied
	Conflicts []string // conflicts left unresolved (Skip policy)
	BaseSaved bool
}

// Sync enumerates, classifies, applies per the conflict policy, and commits the new base
// once at the end. It is safe to cancel via ctx and to re-run to resume: nothing is
// committed until every operation succeeds.
func (s *Syncer) Sync(ctx context.Context, local, remote Endpoint, opts Options) (Result, error) {
	plan, base, localM, remoteM, err := s.computePlan(ctx, local, remote, opts)
	if err != nil {
		return Result{}, err
	}

	pull := append([]string(nil), plan.Pull...)
	push := append([]string(nil), plan.Push...)
	var unresolved []string
	switch opts.Conflict {
	case Abort:
		if len(plan.Conflicts) > 0 {
			return Result{}, &ConflictError{Paths: plan.Conflicts}
		}
	case Skip:
		unresolved = plan.Conflicts
	case PreferLocal:
		push = append(push, plan.Conflicts...)
	case PreferRemote:
		pull = append(pull, plan.Conflicts...)
	}
	sort.Strings(pull)
	sort.Strings(push)

	var applied Plan
	if len(pull) > 0 {
		if err := s.transfer(ctx, remote, local, pull, opts, "pull"); err != nil {
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
	if err := s.deleteAll(ctx, remote, plan.RemoteDeletes); err != nil {
		return Result{}, err
	}
	for _, rel := range plan.RemoteDeletes {
		applied.RemoteDeletes = append(applied.RemoteDeletes, rel)
		emit(opts.OnEvent, Event{Op: "delete-remote", Path: rel})
	}
	if err := s.deleteAll(ctx, local, plan.LocalDeletes); err != nil {
		return Result{}, err
	}
	for _, rel := range plan.LocalDeletes {
		applied.LocalDeletes = append(applied.LocalDeletes, rel)
		emit(opts.OnEvent, Event{Op: "delete-local", Path: rel})
	}

	merged := mergedBase(base, localM, remoteM, pull, plan.LocalDeletes, unresolved)
	if err := s.Store.SaveBase(merged); err != nil {
		return Result{Applied: applied, Conflicts: unresolved}, err
	}
	return Result{Applied: applied, Conflicts: unresolved, BaseSaved: true}, nil
}

// transfer runs one rsync transfer from src to dst restricted to files, emitting a
// progress Event per itemized path when opts.OnEvent is set.
func (s *Syncer) transfer(ctx context.Context, src, dst Endpoint, files []string, opts Options, op string) error {
	listPath, err := writeFileList(files)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(listPath) }()
	args := withFilesFrom(buildTransferArgs(src, dst, opts.Checksum, opts.Exclude), listPath)
	var tee io.Writer
	if opts.OnEvent != nil {
		tee = &itemizeWriter{onPath: func(p string) { opts.OnEvent(Event{Op: op, Path: p}) }}
	}
	res, err := s.runner()(ctx, s.bin(), args, tee)
	if err != nil {
		return &Error{Op: op, Args: args, Stderr: res.stderr, ExitCode: res.exitCode, Err: err}
	}
	return nil
}

// deleteAll removes paths from an endpoint: os.Remove locally, one batched "ssh host rm"
// for an ssh endpoint. A missing file is not an error, so re-running after a partial delete
// is idempotent.
func (s *Syncer) deleteAll(ctx context.Context, e Endpoint, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	if e.SSH == nil {
		for _, rel := range paths {
			if err := os.Remove(filepath.Join(e.Path, filepath.FromSlash(rel))); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	args := sshCmdArgs(e.SSH)
	args = append(args, "rm", "-f", "--")
	for _, rel := range paths {
		args = append(args, e.Path+"/"+rel)
	}
	res, err := s.runner()(ctx, "ssh", args, nil)
	if err != nil {
		return &Error{Op: "delete", Args: args, Stderr: res.stderr, ExitCode: res.exitCode, Err: err}
	}
	return nil
}

// mergedBase derives the new base after a successful apply. It starts from the local
// manifest, overlays pulled paths with the remote state (both sides now match), drops
// locally deleted paths, and restores each unresolved conflict to its previous base entry
// (so it resurfaces next run). Pushed paths already equal the local state; remotely deleted
// paths are absent from localM.
func mergedBase(base, localM, remoteM Manifest, pulls, localDeletes, unresolved []string) Manifest {
	merged := make(Manifest, len(localM))
	for p, st := range localM {
		merged[p] = st
	}
	for _, p := range pulls {
		if st, ok := remoteM[p]; ok {
			merged[p] = st
		}
	}
	for _, p := range localDeletes {
		delete(merged, p)
	}
	for _, p := range unresolved {
		if st, ok := base[p]; ok {
			merged[p] = st
		} else {
			delete(merged, p)
		}
	}
	return merged
}

func emit(onEvent func(Event), e Event) {
	if onEvent != nil {
		onEvent(e)
	}
}
