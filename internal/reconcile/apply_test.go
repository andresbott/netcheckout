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
		if err := os.WriteFile(filepath.Join(dst, f), data, 0o644); err != nil {
			return rsync.Result{}, err
		}
	}
	return rsync.Result{}, nil
}
func (r *recordSyncer) Diff(context.Context, rsync.Job) (rsync.Diff, error) {
	return rsync.Diff{}, nil
}

func TestApplyConflictWithoutForceWritesNothing(t *testing.T) {
	s := &recordSyncer{}
	_, err := Apply(context.Background(), s, t.TempDir(), t.TempDir(), Plan{Conflicts: []string{"c.txt"}}, false)
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
	applied, err := Apply(context.Background(), s, localRoot, remoteRoot, Plan{Conflicts: []string{"c.txt"}}, true)
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
	}, false)
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
