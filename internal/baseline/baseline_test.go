package baseline

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/andresbott/netcheckout/internal/marker"
)

func TestDirHonorsStateOverride(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", "/tmp/state-x")
	got, err := Dir()
	if err != nil || got != "/tmp/state-x" {
		t.Fatalf("Dir = %q, %v; want /tmp/state-x", got, err)
	}
}

func TestSaveLoadRemoveRoundTrip(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	b := &Baseline{
		Profile:    "photos",
		Relpaths:   []string{"."},
		Files:      map[string]FileState{"a.txt": {Size: 3, ModTime: time.Unix(100, 0), Hash: "deadbeef"}},
		LastSyncAt: time.Unix(200, 0),
	}
	if err := Save(b); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load("photos")
	if err != nil || !ok {
		t.Fatalf("Load = ok %v err %v", ok, err)
	}
	if got.Files["a.txt"].Hash != "deadbeef" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if err := Remove("photos"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := Load("photos"); ok {
		t.Error("baseline still present after Remove")
	}
}

func TestLoadMissingReturnsNotExists(t *testing.T) {
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	if _, ok, err := Load("nope"); ok || err != nil {
		t.Fatalf("Load(missing) = ok %v err %v; want false,nil", ok, err)
	}
}

func TestSnapshotHashesFilesAndSkipsMarker(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A marker at the root must be excluded from the snapshot.
	if err := os.WriteFile(filepath.Join(root, marker.FileName), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := Snapshot(root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files[marker.FileName]; ok {
		t.Error("snapshot must not include the marker file")
	}
	if len(files) != 2 {
		t.Fatalf("want 2 files, got %d: %+v", len(files), files)
	}
	if files["a.txt"].Hash == "" || files["sub/b.txt"].Hash == "" {
		t.Errorf("snapshot must record hashes: %+v", files)
	}
	if files["a.txt"].Size != 5 {
		t.Errorf("a.txt size = %d, want 5", files["a.txt"].Size)
	}
}

func TestSnapshotSkipsNonRegularFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A broken symlink (points nowhere) must be skipped, not hashed.
	if err := os.Symlink(filepath.Join(root, "does-not-exist"), filepath.Join(root, "dangling")); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}
	files, err := Snapshot(root, []string{"."})
	if err != nil {
		t.Fatalf("Snapshot must not error on a symlink: %v", err)
	}
	if _, ok := files["dangling"]; ok {
		t.Error("symlink must be excluded from the snapshot")
	}
	if _, ok := files["real.txt"]; !ok {
		t.Error("regular file should still be captured")
	}
}

func TestScanRecordsSizeMtimeNoHash(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Scan(root, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	fs, ok := got["a.txt"]
	if !ok || fs.Size != 5 {
		t.Fatalf("scan a.txt = %+v ok=%v", fs, ok)
	}
	if fs.Hash != "" {
		t.Errorf("Scan must not hash (fast path); got %q", fs.Hash)
	}
}

func TestScanSkipsNonRegularFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "real.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A broken symlink (points nowhere) must be skipped, not stat'd into the manifest.
	if err := os.Symlink(filepath.Join(root, "does-not-exist"), filepath.Join(root, "dangling")); err != nil {
		t.Skipf("symlinks unsupported here: %v", err)
	}
	files, err := Scan(root, []string{"."})
	if err != nil {
		t.Fatalf("Scan must not error on a symlink: %v", err)
	}
	if _, ok := files["dangling"]; ok {
		t.Error("symlink must be excluded from the scan manifest")
	}
	if _, ok := files["real.txt"]; !ok {
		t.Error("regular file should still be captured")
	}
}

func TestChangedFastPathUnchanged(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(p)
	base := FileState{Size: 5, ModTime: info.ModTime(), Hash: "irrelevant"}
	cur := FileState{Size: 5, ModTime: info.ModTime()}
	changed, err := Changed(base, cur, p)
	if err != nil || changed {
		t.Fatalf("same size+mtime must be unchanged; changed=%v err=%v", changed, err)
	}
}

func TestChangedHashConfirmsSameContent(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	h, _ := HashFile(p)
	// mtime differs from base, but content (hash) matches => NOT changed.
	base := FileState{Size: 5, ModTime: time.Unix(1, 0), Hash: h}
	cur := FileState{Size: 5, ModTime: time.Unix(999, 0)}
	changed, err := Changed(base, cur, p)
	if err != nil || changed {
		t.Fatalf("matching hash must be unchanged despite mtime; changed=%v err=%v", changed, err)
	}
}

func TestChangedDetectsRealEdit(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a.txt")
	if err := os.WriteFile(p, []byte("EDITED"), 0o644); err != nil {
		t.Fatal(err)
	}
	base := FileState{Size: 5, ModTime: time.Unix(1, 0), Hash: "0000"}
	cur := FileState{Size: 6, ModTime: time.Unix(2, 0)}
	changed, err := Changed(base, cur, p)
	if err != nil || !changed {
		t.Fatalf("different content must be changed; changed=%v err=%v", changed, err)
	}
}
