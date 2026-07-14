package threewayrsync

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// applyRunner routes list calls (by source path) to canned manifests and records transfer
// and ssh calls. It returns empty output (success) for non-list calls.
type applyRunner struct {
	lists     map[string]string // src prefix -> list output
	transfers [][]string        // captured rsync transfer arg lists
	sshCalls  [][]string        // captured ssh arg lists
}

func (a *applyRunner) run(_ context.Context, bin string, args []string, tee io.Writer) (runResult, error) {
	if bin == "ssh" {
		a.sshCalls = append(a.sshCalls, args)
		return runResult{}, nil
	}
	// rsync: a list call has --dry-run + --out-format; anything else is a transfer.
	isList := false
	for _, x := range args {
		if x == "--dry-run" {
			isList = true
		}
	}
	if isList {
		src := args[len(args)-2]
		for prefix, out := range a.lists {
			if strings.HasPrefix(src, prefix) {
				return runResult{stdout: out}, nil
			}
		}
		return runResult{}, nil
	}
	a.transfers = append(a.transfers, args)
	return runResult{}, nil
}

func TestSyncAbortsOnConflict(t *testing.T) {
	ar := &applyRunner{lists: map[string]string{
		"/l/": ">f.st...... 2 2026/07/14-09:15:01 x.txt\n", // local edited
		"/r/": ">f.st...... 3 2026/07/14-09:15:02 x.txt\n", // remote edited differently
	}}
	base := Manifest{"x.txt": {Size: 1, ModTime: time.Unix(100, 0)}}
	s := &Syncer{Store: &memStore{base: base, ok: true}, run: ar.run}
	_, err := s.Sync(context.Background(), Endpoint{Path: "/l"}, Endpoint{Path: "/r"}, Options{Conflict: Abort})
	var ce *ConflictError
	if !errors.As(err, &ce) || len(ce.Paths) != 1 || ce.Paths[0] != "x.txt" {
		t.Fatalf("want ConflictError for x.txt, got %v", err)
	}
	if len(ar.transfers) != 0 {
		t.Errorf("Abort must not transfer anything, got %v", ar.transfers)
	}
}

func TestSyncPreferLocalPushesConflict(t *testing.T) {
	ar := &applyRunner{lists: map[string]string{
		"/l/": ">f.st...... 2 2026/07/14-09:15:01 x.txt\n",
		"/r/": ">f.st...... 3 2026/07/14-09:15:02 x.txt\n",
	}}
	store := &memStore{base: Manifest{"x.txt": {Size: 1, ModTime: time.Unix(100, 0)}}, ok: true}
	s := &Syncer{Store: store, run: ar.run}
	res, err := s.Sync(context.Background(), Endpoint{Path: "/l"}, Endpoint{Path: "/r"}, Options{Conflict: PreferLocal})
	if err != nil {
		t.Fatal(err)
	}
	if len(ar.transfers) != 1 {
		t.Fatalf("want one push transfer, got %d", len(ar.transfers))
	}
	// A push runs local -> remote: source (2nd-to-last arg) is under /l.
	pushArgs := ar.transfers[0]
	if !strings.HasPrefix(pushArgs[len(pushArgs)-2], "/l/") {
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

func TestSyncCanceledContextDoesNotSaveBase(t *testing.T) {
	failing := func(_ context.Context, _ string, _ []string, _ io.Writer) (runResult, error) {
		return runResult{}, context.Canceled
	}
	store := &memStore{}
	s := &Syncer{Store: store, run: failing}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Sync(ctx, Endpoint{Path: "/l"}, Endpoint{Path: "/r"}, Options{})
	if err == nil {
		t.Fatal("want error from canceled context")
	}
	if store.saved != 0 {
		t.Errorf("base must not be saved on cancel; saved=%d", store.saved)
	}
}

func TestMergedBaseAppliesPullsAndDeletes(t *testing.T) {
	base := Manifest{"keep.txt": {Size: 1, ModTime: time.Unix(1, 0)}, "del.txt": {Size: 1, ModTime: time.Unix(1, 0)}}
	localM := Manifest{"keep.txt": {Size: 1, ModTime: time.Unix(1, 0)}, "del.txt": {Size: 1, ModTime: time.Unix(1, 0)}}
	remoteM := Manifest{"keep.txt": {Size: 9, ModTime: time.Unix(2, 0)}, "del.txt": {Size: 1, ModTime: time.Unix(1, 0)}}
	// keep.txt pulled (local takes remote state); del.txt deleted locally.
	merged := mergedBase(base, localM, remoteM, []string{"keep.txt"}, []string{"del.txt"}, nil)
	if merged["keep.txt"].Size != 9 {
		t.Errorf("pulled path should take remote state: %+v", merged["keep.txt"])
	}
	if _, ok := merged["del.txt"]; ok {
		t.Error("locally deleted path must be absent from merged base")
	}
}

func TestMergedBaseRetainsUnresolvedConflict(t *testing.T) {
	base := Manifest{"x.txt": {Size: 1, ModTime: time.Unix(1, 0)}}
	localM := Manifest{"x.txt": {Size: 2, ModTime: time.Unix(2, 0)}}
	remoteM := Manifest{"x.txt": {Size: 3, ModTime: time.Unix(3, 0)}}
	merged := mergedBase(base, localM, remoteM, nil, nil, []string{"x.txt"})
	if merged["x.txt"].Size != 1 {
		t.Errorf("unresolved conflict must retain the previous base entry, got %+v", merged["x.txt"])
	}
}
