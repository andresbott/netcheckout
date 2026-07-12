package reconcile

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/rsync"
)

// recordSyncer records the jobs it is given and simulates a pull/push by copying
// the listed files between roots.
type recordSyncer struct{ jobs []rsync.Job }

func (r *recordSyncer) Sync(_ context.Context, j rsync.Job) (rsync.Result, error) {
	r.jobs = append(r.jobs, j)
	src, dst := j.Remote.Path, j.Local.Path
	if j.Direction == rsync.Push {
		src, dst = j.Local.Path, j.Remote.Path
	}
	for _, f := range j.Files {
		data, err := os.ReadFile(filepath.Join(src, f))
		if err != nil {
			return rsync.Result{}, err
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dst, f)), 0o755); err != nil {
			return rsync.Result{}, err
		}
		// Classify like rsync's itemize would: a new destination file is a
		// creation, an existing one an update.
		ct := rsync.Created
		if _, err := os.Stat(filepath.Join(dst, f)); err == nil {
			ct = rsync.Modified
		}
		if err := os.WriteFile(filepath.Join(dst, f), data, 0o644); err != nil {
			return rsync.Result{}, err
		}
		if j.OnChange != nil {
			j.OnChange(rsync.Change{Path: f, Type: ct})
		}
	}
	return rsync.Result{}, nil
}
func (r *recordSyncer) Diff(context.Context, rsync.Job) (rsync.Diff, error) {
	return rsync.Diff{}, nil
}

func TestApplyConflictWithoutForceWritesNothing(t *testing.T) {
	s := &recordSyncer{}
	_, err := Apply(context.Background(), s, t.TempDir(), t.TempDir(), Plan{Conflicts: []string{"c.txt"}}, false, nil)
	var ce *ConflictError
	if err == nil {
		t.Fatal("want ConflictError")
	}
	if !as(err, &ce) {
		t.Fatalf("want *ConflictError, got %T", err)
	}
	if len(s.jobs) != 0 {
		t.Error("no transfer should run when a conflict stops the reconcile")
	}
}

func TestApplyForceResolvesConflictAsPush(t *testing.T) {
	localRoot := t.TempDir()
	remoteRoot := t.TempDir()
	_ = os.WriteFile(filepath.Join(localRoot, "c.txt"), []byte("local"), 0o644)
	s := &recordSyncer{}
	applied, err := Apply(context.Background(), s, localRoot, remoteRoot, Plan{Conflicts: []string{"c.txt"}}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(remoteRoot, "c.txt")); err != nil || string(got) != "local" {
		t.Errorf("force should push local over remote; got %q err %v", got, err)
	}
	if len(applied.Pushed) != 1 {
		t.Errorf("applied.Pushed = %v", applied.Pushed)
	}
}

func TestApplyPerformsDeletes(t *testing.T) {
	localRoot := t.TempDir()
	remoteRoot := t.TempDir()
	_ = os.WriteFile(filepath.Join(remoteRoot, "r.txt"), []byte("r"), 0o644)
	_ = os.WriteFile(filepath.Join(localRoot, "l.txt"), []byte("l"), 0o644)
	s := &recordSyncer{}
	applied, err := Apply(context.Background(), s, localRoot, remoteRoot, Plan{
		RemoteDeletes: []string{"r.txt"},
		LocalDeletes:  []string{"l.txt"},
	}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(remoteRoot, "r.txt")); !os.IsNotExist(err) {
		t.Error("r.txt should be removed from the remote")
	}
	if _, err := os.Stat(filepath.Join(localRoot, "l.txt")); !os.IsNotExist(err) {
		t.Error("l.txt should be removed locally")
	}
	if len(applied.RemovedRemote) != 1 || len(applied.RemovedLocal) != 1 {
		t.Errorf("applied = %+v", applied)
	}
}

func TestApplyEmitsSidedEvents(t *testing.T) {
	localRoot := t.TempDir()
	remoteRoot := t.TempDir()
	// A local add to push, a remote add to pull, and a delete on each side.
	_ = os.WriteFile(filepath.Join(localRoot, "up.txt"), []byte("u"), 0o644)
	_ = os.WriteFile(filepath.Join(remoteRoot, "down.txt"), []byte("d"), 0o644)
	_ = os.WriteFile(filepath.Join(remoteRoot, "gone-r.txt"), []byte("r"), 0o644)
	_ = os.WriteFile(filepath.Join(localRoot, "gone-l.txt"), []byte("l"), 0o644)

	var got []Event
	s := &recordSyncer{}
	if _, err := Apply(context.Background(), s, localRoot, remoteRoot, Plan{
		Push:          []string{"up.txt"},
		Pull:          []string{"down.txt"},
		RemoteDeletes: []string{"gone-r.txt"},
		LocalDeletes:  []string{"gone-l.txt"},
	}, false, func(e Event) { got = append(got, e) }); err != nil {
		t.Fatal(err)
	}

	want := []Event{
		{Kind: EventAdd, Side: SideLocal, Path: "down.txt"}, // pull runs first
		{Kind: EventAdd, Side: SideRemote, Path: "up.txt"},  // push next
		{Kind: EventDelete, Side: SideRemote, Path: "gone-r.txt"},
		{Kind: EventDelete, Side: SideLocal, Path: "gone-l.txt"},
	}
	if len(got) != len(want) {
		t.Fatalf("event count = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// as is a tiny errors.As wrapper kept local to avoid an extra import in the table.
func as(err error, target **ConflictError) bool {
	for err != nil {
		if ce, ok := err.(*ConflictError); ok {
			*target = ce
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func TestPullEmitterMapsToLocalSide(t *testing.T) {
	var got []Event
	emit := PullEmitter(func(e Event) { got = append(got, e) })
	emit(rsync.Change{Path: "new.txt", Type: rsync.Created})
	emit(rsync.Change{Path: "upd.txt", Type: rsync.Modified})

	want := []Event{
		{Kind: EventAdd, Side: SideLocal, Path: "new.txt"},
		{Kind: EventModify, Side: SideLocal, Path: "upd.txt"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if PullEmitter(nil) != nil {
		t.Error("PullEmitter(nil) should return nil so no callback is installed")
	}
}
