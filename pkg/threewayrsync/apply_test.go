package threewayrsync

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// testEndpoints returns two local endpoints backed by real (empty) temp dirs so the
// preflight stat passes; enumeration still comes from the fake runner's canned lists.
func testEndpoints(t *testing.T) (Endpoint, Endpoint) {
	t.Helper()
	return Endpoint{Path: t.TempDir()}, Endpoint{Path: t.TempDir()}
}

// applyRunner routes list calls (by source path) to canned manifests and records transfer
// and remote-delete calls. computePlan performs exactly two list calls, so from the third
// list on — the post-apply re-list — answers come from postLists (when set), simulating
// the state after apply. It returns empty output (success) for non-list calls, unless
// failTransfer is set, in which case a transfer call (not a list, not a delete) fails
// with a non-nil error.
type applyRunner struct {
	lists        map[string]string // src prefix -> list output before apply
	postLists    map[string]string // src prefix -> list output for the post-apply re-list
	transfers    [][]string        // captured rsync transfer arg lists
	deletes      [][]string        // captured rsync remote-delete arg lists (--delete-missing-args)
	deletedLists []string          // contents of each delete call's --files-from list
	failTransfer bool              // when true, a transfer call returns a non-nil error
	listCalls    int
}

func (a *applyRunner) run(_ context.Context, _ string, args []string, tee io.Writer) (runResult, error) {
	// rsync: a list call has --dry-run + --out-format; a remote delete has
	// --delete-missing-args; anything else is a transfer.
	isList, isDelete := false, false
	var filesFrom string
	for _, x := range args {
		switch {
		case x == "--dry-run":
			isList = true
		case x == "--delete-missing-args":
			isDelete = true
		case strings.HasPrefix(x, "--files-from="):
			filesFrom = strings.TrimPrefix(x, "--files-from=")
		}
	}
	if isDelete {
		a.deletes = append(a.deletes, args)
		if filesFrom != "" {
			data, err := os.ReadFile(filesFrom)
			if err != nil {
				return runResult{}, err
			}
			a.deletedLists = append(a.deletedLists, string(data))
		}
		return runResult{}, nil
	}
	if isList {
		a.listCalls++
		lists := a.lists
		if a.listCalls > 2 && a.postLists != nil {
			lists = a.postLists
		}
		src := args[len(args)-2]
		for prefix, out := range lists {
			if strings.HasPrefix(src, prefix) {
				return runResult{stdout: out}, nil
			}
		}
		return runResult{}, nil
	}
	a.transfers = append(a.transfers, args)
	if a.failTransfer {
		return runResult{stderr: "rsync: connection unexpectedly closed", exitCode: 12}, errors.New("exit status 12")
	}
	return runResult{}, nil
}

func TestSyncAbortsOnConflict(t *testing.T) {
	local, remote := testEndpoints(t)
	ar := &applyRunner{lists: map[string]string{
		local.Path + "/":  ">f.st...... 2 2026/07/14-09:15:01 x.txt\n", // local edited
		remote.Path + "/": ">f.st...... 3 2026/07/14-09:15:02 x.txt\n", // remote edited differently
	}}
	base := Manifest{"x.txt": {Size: 1, ModTime: time.Unix(100, 0)}}
	store := &memStore{base: base, ok: true}
	s := &Syncer{Store: store, run: ar.run}
	_, err := s.Sync(context.Background(), local, remote, Options{Conflict: Abort})
	var ce *ConflictError
	if !errors.As(err, &ce) || len(ce.Paths) != 1 || ce.Paths[0] != "x.txt" {
		t.Fatalf("want ConflictError for x.txt, got %v", err)
	}
	if len(ar.transfers) != 0 {
		t.Errorf("Abort must not transfer anything, got %v", ar.transfers)
	}
	if store.saved != 0 {
		t.Errorf("Abort must not persist a base, got saved=%d", store.saved)
	}
}

func TestSyncPreferLocalPushesConflict(t *testing.T) {
	local, remote := testEndpoints(t)
	agreed := ">f.st...... 2 2026/07/14-09:15:01 x.txt\n"
	ar := &applyRunner{
		lists: map[string]string{
			local.Path + "/":  agreed,
			remote.Path + "/": ">f.st...... 3 2026/07/14-09:15:02 x.txt\n",
		},
		// After the push both sides agree on the local state.
		postLists: map[string]string{local.Path + "/": agreed, remote.Path + "/": agreed},
	}
	store := &memStore{base: Manifest{"x.txt": {Size: 1, ModTime: time.Unix(100, 0)}}, ok: true}
	s := &Syncer{Store: store, run: ar.run}
	res, err := s.Sync(context.Background(), local, remote, Options{Conflict: PreferLocal})
	if err != nil {
		t.Fatal(err)
	}
	if len(ar.transfers) != 1 {
		t.Fatalf("want one push transfer, got %d", len(ar.transfers))
	}
	// A push runs local -> remote: source (2nd-to-last arg) is under the local root.
	pushArgs := ar.transfers[0]
	if !strings.HasPrefix(pushArgs[len(pushArgs)-2], local.Path+"/") {
		t.Errorf("conflict should be pushed local->remote: %v", pushArgs)
	}
	if !res.BaseSaved || store.saved != 1 {
		t.Errorf("base must be saved exactly once; BaseSaved=%v saved=%d", res.BaseSaved, store.saved)
	}
	// After PreferLocal, merged base records the local state (size 2).
	if store.base["x.txt"].Size != 2 {
		t.Errorf("merged base x.txt size = %d, want 2", store.base["x.txt"].Size)
	}
}

func TestSyncPreferLocalDeletesRemoteOnDeleteVsEdit(t *testing.T) {
	local, remote := testEndpoints(t)
	ar := &applyRunner{
		lists: map[string]string{
			local.Path + "/":  "",                                          // x.txt deleted locally
			remote.Path + "/": ">f.st...... 5 2026/07/14-09:15:02 x.txt\n", // remote edited (different size)
		},
		postLists: map[string]string{}, // both sides empty after the remote delete
	}
	base := Manifest{"x.txt": {Size: 1, ModTime: time.Unix(100, 0)}}
	store := &memStore{base: base, ok: true}
	s := &Syncer{Store: store, run: ar.run}
	res, err := s.Sync(context.Background(), local, remote, Options{Conflict: PreferLocal, AcceptEmpty: true, MaxDeleteFraction: 1})
	if err != nil {
		var ce *ConflictError
		if errors.As(err, &ce) {
			t.Fatalf("PreferLocal must resolve a delete-vs-edit conflict, not report ConflictError: %v", err)
		}
		t.Fatal(err)
	}
	if !slices.Contains(res.Applied.RemoteDeletes, "x.txt") {
		t.Errorf("RemoteDeletes = %v, want x.txt", res.Applied.RemoteDeletes)
	}
	if slices.Contains(res.Applied.Push, "x.txt") {
		t.Errorf("Push must not contain x.txt when local already deleted it: %v", res.Applied.Push)
	}
	if _, ok := store.base["x.txt"]; ok {
		t.Errorf("saved base must not resurrect x.txt: %+v", store.base)
	}
	if store.saved != 1 {
		t.Errorf("base must be saved exactly once; saved=%d", store.saved)
	}
}

func TestSyncPreferRemoteDeletesLocalOnEditVsDelete(t *testing.T) {
	local, remote := testEndpoints(t)
	ar := &applyRunner{
		lists: map[string]string{
			local.Path + "/":  ">f.st...... 5 2026/07/14-09:15:02 x.txt\n", // local edited (different size)
			remote.Path + "/": "",                                          // x.txt deleted remotely
		},
		postLists: map[string]string{},
	}
	base := Manifest{"x.txt": {Size: 1, ModTime: time.Unix(100, 0)}}
	store := &memStore{base: base, ok: true}
	s := &Syncer{Store: store, run: ar.run}
	res, err := s.Sync(context.Background(), local, remote, Options{Conflict: PreferRemote, AcceptEmpty: true, MaxDeleteFraction: 1})
	if err != nil {
		var ce *ConflictError
		if errors.As(err, &ce) {
			t.Fatalf("PreferRemote must resolve an edit-vs-delete conflict, not report ConflictError: %v", err)
		}
		t.Fatal(err)
	}
	if !slices.Contains(res.Applied.LocalDeletes, "x.txt") {
		t.Errorf("LocalDeletes = %v, want x.txt", res.Applied.LocalDeletes)
	}
	if slices.Contains(res.Applied.Pull, "x.txt") {
		t.Errorf("Pull must not contain x.txt when remote already deleted it: %v", res.Applied.Pull)
	}
	if _, ok := store.base["x.txt"]; ok {
		t.Errorf("saved base must not resurrect x.txt: %+v", store.base)
	}
	if store.saved != 1 {
		t.Errorf("base must be saved exactly once; saved=%d", store.saved)
	}
}

func TestSyncCanceledContextDoesNotSaveBase(t *testing.T) {
	local, remote := testEndpoints(t)
	failing := func(ctx context.Context, _ string, _ []string, _ io.Writer) (runResult, error) {
		return runResult{}, ctx.Err()
	}
	store := &memStore{}
	s := &Syncer{Store: store, run: failing}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Sync(ctx, local, remote, Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation must surface ctx.Err(), got %v", err)
	}
	if store.saved != 0 {
		t.Errorf("base must not be saved on cancel; saved=%d", store.saved)
	}
}

func TestSyncErrorAfterTransferDoesNotSaveBase(t *testing.T) {
	local, remote := testEndpoints(t)
	ar := &applyRunner{
		lists: map[string]string{
			local.Path + "/":  ">f+++++++++ 1 2026/07/14-09:15:00 new.txt\n", // local-only add => push
			remote.Path + "/": "",
		},
		failTransfer: true,
	}
	store := &memStore{}
	s := &Syncer{Store: store, run: ar.run}
	_, err := s.Sync(context.Background(), local, remote, Options{})
	if err == nil {
		t.Fatal("want an error when the transfer fails")
	}
	if store.saved != 0 {
		t.Errorf("base must not be saved when a transfer fails after a successful list; saved=%d", store.saved)
	}
}

func TestSyncRefusesMassDeletion(t *testing.T) {
	local, remote := testEndpoints(t)
	// Remote lists 1 of the 20 base files; local still has all 20 => 19 remote-deletes.
	base := Manifest{}
	var localList strings.Builder
	for i := 0; i < 20; i++ {
		name := "f" + strconv.Itoa(i) + ".txt"
		base[name] = FileState{Size: 1, ModTime: time.Date(2026, 7, 14, 9, 15, 0, 0, time.Local)}
		localList.WriteString(">f+++++++++ 1 2026/07/14-09:15:00 " + name + "\n")
	}
	ar := &applyRunner{lists: map[string]string{
		local.Path + "/":  localList.String(),
		remote.Path + "/": ">f+++++++++ 1 2026/07/14-09:15:00 f0.txt\n",
	}}
	store := &memStore{base: base, ok: true}
	s := &Syncer{Store: store, run: ar.run}
	_, err := s.Sync(context.Background(), local, remote, Options{})
	var tde *TooManyDeletesError
	if !errors.As(err, &tde) {
		t.Fatalf("want TooManyDeletesError, got %v", err)
	}
	if store.saved != 0 || len(ar.transfers) != 0 {
		t.Errorf("guard must fire before any change; saved=%d transfers=%d", store.saved, len(ar.transfers))
	}
	// Raising the fraction lets the deletion proceed.
	if _, err := s.Sync(context.Background(), local, remote, Options{MaxDeleteFraction: 1}); err != nil {
		t.Fatalf("MaxDeleteFraction=1 must disable the guard: %v", err)
	}
}

func TestSyncRefusesEmptyEndpointWithHistory(t *testing.T) {
	local, remote := testEndpoints(t)
	ar := &applyRunner{lists: map[string]string{
		local.Path + "/":  ">f+++++++++ 1 2026/07/14-09:15:00 a.txt\n",
		remote.Path + "/": "", // remote suddenly lists nothing
	}}
	base := Manifest{"a.txt": {Size: 1, ModTime: time.Date(2026, 7, 14, 9, 15, 0, 0, time.Local)}}
	store := &memStore{base: base, ok: true}
	s := &Syncer{Store: store, run: ar.run}
	_, err := s.Sync(context.Background(), local, remote, Options{})
	var ee *EmptyEndpointError
	if !errors.As(err, &ee) || ee.Side != "remote" {
		t.Fatalf("want EmptyEndpointError for the remote side, got %v", err)
	}
	// AcceptEmpty overrides (the delete guard is a separate valve, disabled here).
	if _, err := s.Sync(context.Background(), local, remote, Options{AcceptEmpty: true, MaxDeleteFraction: 1}); err != nil {
		t.Fatalf("AcceptEmpty must allow the sync: %v", err)
	}
}

func TestSyncMissingLocalEndpointFails(t *testing.T) {
	local, _ := testEndpoints(t)
	s := &Syncer{Store: &memStore{}, run: (&applyRunner{}).run}
	_, err := s.Sync(context.Background(), local, Endpoint{Path: "/does/not/exist-xyz"}, Options{})
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("want missing-endpoint error, got %v", err)
	}
}

func TestSyncNilStoreErrors(t *testing.T) {
	local, remote := testEndpoints(t)
	s := &Syncer{run: (&applyRunner{}).run}
	if _, err := s.Sync(context.Background(), local, remote, Options{}); err == nil {
		t.Fatal("nil Store must be an error, not a panic")
	}
}

// lockStore wraps memStore with a Locker that is already held.
type lockStore struct{ memStore }

func (l *lockStore) TryLock() (func(), error) { return nil, ErrLocked }

func TestSyncFailsFastWhenLocked(t *testing.T) {
	local, remote := testEndpoints(t)
	s := &Syncer{Store: &lockStore{}, run: (&applyRunner{}).run}
	if _, err := s.Sync(context.Background(), local, remote, Options{}); !errors.Is(err, ErrLocked) {
		t.Fatalf("want ErrLocked, got %v", err)
	}
}

func TestSyncSkipsDeleteOfLocallyChangedFile(t *testing.T) {
	local, remote := testEndpoints(t)
	// Base + local list say x.txt exists (size 1); remote deleted it => LocalDelete. But
	// the file on disk differs from the planned state (edited after enumeration): the
	// guard must skip it, not delete it.
	if err := os.WriteFile(filepath.Join(local.Path, "x.txt"), []byte("edited-after-listing"), 0o644); err != nil {
		t.Fatal(err)
	}
	ar := &applyRunner{lists: map[string]string{
		local.Path + "/":  ">f+++++++++ 1 2026/07/14-09:15:00 x.txt\n",
		remote.Path + "/": "",
	}}
	base := Manifest{"x.txt": {Size: 1, ModTime: time.Date(2026, 7, 14, 9, 15, 0, 0, time.Local)}}
	store := &memStore{base: base, ok: true}
	s := &Syncer{Store: store, run: ar.run}
	res, err := s.Sync(context.Background(), local, remote, Options{AcceptEmpty: true, MaxDeleteFraction: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(filepath.Join(local.Path, "x.txt")); statErr != nil {
		t.Fatalf("changed file must survive the delete: %v", statErr)
	}
	if len(res.Applied.LocalDeletes) != 0 {
		t.Errorf("LocalDeletes = %v, want none", res.Applied.LocalDeletes)
	}
	if !slices.Contains(res.Conflicts, "x.txt") {
		t.Errorf("skipped delete must be reported as a conflict: %v", res.Conflicts)
	}
}

func TestMergedBaseRecordsAgreementOnly(t *testing.T) {
	prev := Manifest{"old.txt": {Size: 1, ModTime: time.Unix(1, 0)}}
	postLocal := Manifest{
		"agree.txt": {Size: 9, ModTime: time.Unix(2, 0)},
		"old.txt":   {Size: 5, ModTime: time.Unix(5, 0)}, // still disagrees with remote
	}
	postRemote := Manifest{
		"agree.txt": {Size: 9, ModTime: time.Unix(2, 0)},
		"old.txt":   {Size: 7, ModTime: time.Unix(7, 0)},
	}
	merged := mergedBase(prev, postLocal, postRemote, nil)
	if merged["agree.txt"].Size != 9 {
		t.Errorf("agreeing path should enter the base: %+v", merged["agree.txt"])
	}
	// A disagreeing path keeps its previous base entry so it resurfaces next run.
	if merged["old.txt"].Size != 1 {
		t.Errorf("disagreeing path must retain the previous base entry, got %+v", merged["old.txt"])
	}
}

func TestMergedBaseDropsDeletedPaths(t *testing.T) {
	prev := Manifest{"del.txt": {Size: 1, ModTime: time.Unix(1, 0)}}
	// Deleted on both sides post-apply: absent from both manifests => out of the base.
	merged := mergedBase(prev, Manifest{}, Manifest{}, nil)
	if _, ok := merged["del.txt"]; ok {
		t.Error("path absent from both sides must be absent from merged base")
	}
}

func TestMergedBaseSkipsBothAddedDisagreement(t *testing.T) {
	// Present on both sides with different state and no previous base entry (Skip policy
	// on a both-added conflict): must stay out so it re-classifies as a conflict.
	postLocal := Manifest{"x.txt": {Size: 2, ModTime: time.Unix(2, 0)}}
	postRemote := Manifest{"x.txt": {Size: 3, ModTime: time.Unix(3, 0)}}
	merged := mergedBase(Manifest{}, postLocal, postRemote, nil)
	if _, ok := merged["x.txt"]; ok {
		t.Errorf("both-added disagreement must not enter the base: %+v", merged)
	}
}

func TestDeleteFromRemoteUsesRsyncDeleteMissingArgs(t *testing.T) {
	for name, ep := range map[string]Endpoint{
		"ssh":    {Path: "/remote", SSH: &SSH{Host: "h"}},
		"daemon": {Path: "sub", Daemon: &Daemon{Host: "h", Module: "m"}},
	} {
		t.Run(name, func(t *testing.T) {
			ar := &applyRunner{}
			s := &Syncer{Store: &memStore{}, run: ar.run}
			paths := []string{"a b.txt", "x$(id).txt", "odd\nname.txt"}
			deleted, skipped, err := s.deleteFrom(context.Background(), ep, paths, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(skipped) != 0 || len(deleted) != 3 {
				t.Fatalf("deleted=%v skipped=%v", deleted, skipped)
			}
			if len(ar.deletes) != 1 {
				t.Fatalf("want one rsync delete call, got %d: %v", len(ar.deletes), ar.deletes)
			}
			args := ar.deletes[0]
			for _, want := range []string{"--delete-missing-args", "--from0"} {
				if !slices.Contains(args, want) {
					t.Errorf("delete args missing %q: %v", want, args)
				}
			}
			// The paths travel via the NUL-separated file list, not the command line.
			if want := "a b.txt\x00x$(id).txt\x00odd\nname.txt\x00"; ar.deletedLists[0] != want {
				t.Errorf("files-from content = %q, want %q", ar.deletedLists[0], want)
			}
		})
	}
}

func TestDeleteFromRemoteLargeSetIsOneCall(t *testing.T) {
	ar := &applyRunner{}
	s := &Syncer{Store: &memStore{}, run: ar.run}
	// Paths travel in a file, so even a huge set (way past ARG_MAX as command-line
	// arguments) is a single rsync invocation.
	paths := make([]string, 1000)
	for i := range paths {
		paths[i] = strings.Repeat("d", 100) + "/" + strconv.Itoa(i) + ".txt"
	}
	deleted, _, err := s.deleteFrom(context.Background(), Endpoint{Path: "/remote", SSH: &SSH{Host: "h"}}, paths, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(deleted) != 1000 {
		t.Fatalf("deleted %d of 1000", len(deleted))
	}
	if len(ar.deletes) != 1 {
		t.Fatalf("want one rsync delete call, got %d", len(ar.deletes))
	}
	if got := strings.Count(ar.deletedLists[0], "\x00"); got != 1000 {
		t.Errorf("file list entries = %d, want 1000", got)
	}
}
