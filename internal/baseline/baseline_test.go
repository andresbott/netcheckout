package baseline

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andresbott/netcheckout/pkg/threewayrsync"
)

func stateDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("NETCHECKOUT_STATE", dir)
	return dir
}

func TestDirHonorsStateOverride(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", "/tmp/state-x")
	got, err := Dir()
	if err != nil || got != "/tmp/state-x" {
		t.Fatalf("Dir = %q, %v; want /tmp/state-x", got, err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	stateDir(t)
	when := time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)
	s := &State{
		Profile:    "photos",
		Relpaths:   []string{"2025/jan"},
		Files:      threewayrsync.Manifest{"a.txt": {Size: 3, ModTime: when}},
		LastSyncAt: when,
	}
	if err := Save(s); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load("photos")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got.Profile != "photos" || len(got.Relpaths) != 1 || got.Relpaths[0] != "2025/jan" {
		t.Errorf("state = %+v", got)
	}
	if fs := got.Files["a.txt"]; fs.Size != 3 || !fs.ModTime.Equal(when) {
		t.Errorf("files = %+v", got.Files)
	}
}

func TestLoadMissingIsNotError(t *testing.T) {
	stateDir(t)
	_, ok, err := Load("nope")
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestLoadTruncatesMtimesToSeconds(t *testing.T) {
	stateDir(t)
	when := time.Date(2026, 7, 15, 10, 0, 0, 123456789, time.UTC)
	if err := Save(&State{Profile: "p", Files: threewayrsync.Manifest{"a": {Size: 1, ModTime: when}}}); err != nil {
		t.Fatal(err)
	}
	got, _, err := Load("p")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Files["a"].ModTime.Equal(when.Truncate(time.Second)) {
		t.Errorf("mtime = %v, want second resolution", got.Files["a"].ModTime)
	}
}

func TestLoadOldFormatWithHashes(t *testing.T) {
	dir := stateDir(t)
	// A state file written by the previous engine: hash fields present, ns mtimes.
	old := `{
  "profile": "work",
  "relpaths": ["."],
  "files": {"doc.txt": {"size": 10, "mtime": "2026-07-01T09:30:15.123456789Z", "hash": "abc123"}},
  "last_sync_at": "2026-07-01T09:31:00Z"
}`
	if err := os.WriteFile(filepath.Join(dir, "work.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load("work")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	fs := got.Files["doc.txt"]
	if fs.Size != 10 || fs.ModTime.Nanosecond() != 0 {
		t.Errorf("files = %+v", fs)
	}
}

func TestRemoveIsIdempotent(t *testing.T) {
	stateDir(t)
	if err := Save(&State{Profile: "p"}); err != nil {
		t.Fatal(err)
	}
	if err := Remove("p"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Load("p"); ok {
		t.Fatal("state should be gone")
	}
	if err := Remove("p"); err != nil {
		t.Errorf("second remove: %v", err)
	}
}

func TestProfileStoreRoundTrip(t *testing.T) {
	stateDir(t)
	when := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	if err := Save(&State{Profile: "p", Relpaths: []string{"docs"}, LastSyncAt: when}); err != nil {
		t.Fatal(err)
	}
	ps := &ProfileStore{Profile: "p", Now: func() time.Time { return when.Add(time.Hour) }}
	base, ok, err := ps.LoadBase()
	if err != nil || !ok || len(base) != 0 {
		t.Fatalf("base=%v ok=%v err=%v", base, ok, err)
	}
	merged := threewayrsync.Manifest{"x": {Size: 1, ModTime: when}}
	if err := ps.SaveBase(merged); err != nil {
		t.Fatal(err)
	}
	// The envelope survives, Files is replaced, LastSyncAt is stamped.
	s, _, err := Load("p")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Relpaths) != 1 || s.Relpaths[0] != "docs" {
		t.Errorf("relpaths lost: %+v", s)
	}
	if len(s.Files) != 1 || !s.LastSyncAt.Equal(when.Add(time.Hour)) {
		t.Errorf("state = %+v", s)
	}
}

func TestProfileStoreLoadBaseMissing(t *testing.T) {
	stateDir(t)
	base, ok, err := Store("ghost").LoadBase()
	if err != nil || ok || base != nil {
		t.Fatalf("base=%v ok=%v err=%v", base, ok, err)
	}
}

func TestProfileStoreTryLock(t *testing.T) {
	stateDir(t)
	ps := Store("p")
	release, err := ps.TryLock()
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, err := ps.TryLock(); !errors.Is(err, threewayrsync.ErrLocked) {
		t.Fatalf("second lock err = %v, want ErrLocked", err)
	}
}
