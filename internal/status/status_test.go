package status

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/marker"
)

// fixture creates local/ and remote/ dirs under a fresh temp root and points
// NETCHECKOUT_STATE at a temp state dir so baseline.Load/Save resolve there. It
// returns the profile name, profile, and the two roots.
func fixture(t *testing.T) (name string, p config.Profile, local, remote string) {
	t.Helper()
	root := t.TempDir()
	local = filepath.Join(root, "local")
	remote = filepath.Join(root, "remote")
	for _, d := range []string{local, remote} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("NETCHECKOUT_STATE", t.TempDir())
	return "prof", config.Profile{LocalRoot: local, RemoteRoot: remote}, local, remote
}

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func markCheckedOut(t *testing.T, remote string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(remote, marker.FileName), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// baselineFromLocal snapshots the local tree and saves it as the profile's
// baseline, mimicking the state a checkout+sync leaves behind.
func baselineFromLocal(t *testing.T, name, local string) {
	t.Helper()
	files, err := baseline.Snapshot(local, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if err := baseline.Save(&baseline.Baseline{Profile: name, Relpaths: []string{"."}, Files: files}); err != nil {
		t.Fatal(err)
	}
}

func TestComputeNotCheckedOut(t *testing.T) {
	name, p, _, _ := fixture(t)
	st, err := Compute(name, p)
	if err != nil {
		t.Fatal(err)
	}
	if st.CheckedOut {
		t.Error("want CheckedOut false when no marker is present")
	}
}

func TestComputeCheckedOutNoBaseline(t *testing.T) {
	name, p, _, remote := fixture(t)
	markCheckedOut(t, remote)
	// No baseline saved: checked out, but not on this machine.
	st, err := Compute(name, p)
	if err != nil {
		t.Fatal(err)
	}
	if !st.CheckedOut || st.HasBaseline {
		t.Errorf("want CheckedOut true, HasBaseline false; got %#v", st)
	}
}

func TestComputeInSync(t *testing.T) {
	name, p, local, remote := fixture(t)
	writeFile(t, local, "a.txt", "hello")
	writeFile(t, remote, "a.txt", "hello")
	markCheckedOut(t, remote)
	baselineFromLocal(t, name, local)

	st, err := Compute(name, p)
	if err != nil {
		t.Fatal(err)
	}
	if !st.InSync() {
		t.Errorf("want in sync, got %#v", st)
	}
	// A whole-root profile resolves to a single "(root)" target.
	if len(st.Targets) != 1 || st.Targets[0].Label() != "(root)" {
		t.Errorf("want one (root) target, got %#v", st.Targets)
	}
}

// only returns the single target of a whole-root profile's status, failing if
// the shape is unexpected.
func only(t *testing.T, st ProfileStatus) TargetStatus {
	t.Helper()
	if len(st.Targets) != 1 {
		t.Fatalf("want exactly one target, got %#v", st.Targets)
	}
	return st.Targets[0]
}

// TestComputeRemoteDeleteIsLocalDelete is the regression guard: a file that was
// in the baseline and is still local but has been deleted on the remote must be
// previewed as a local deletion (what sync does), never as a push that would
// resurrect it on the remote.
func TestComputeRemoteDeleteIsLocalDelete(t *testing.T) {
	name, p, local, remote := fixture(t)
	writeFile(t, local, "song.flac", "audio")
	writeFile(t, remote, "song.flac", "audio")
	markCheckedOut(t, remote)
	baselineFromLocal(t, name, local)

	// Delete on the remote only.
	if err := os.Remove(filepath.Join(remote, "song.flac")); err != nil {
		t.Fatal(err)
	}

	st, err := Compute(name, p)
	if err != nil {
		t.Fatal(err)
	}
	tt := only(t, st)
	if len(tt.Push) != 0 {
		t.Errorf("remote delete must not become a push; got Push=%#v", tt.Push)
	}
	if len(tt.LocalDeletes) != 1 || tt.LocalDeletes[0] != "song.flac" {
		t.Errorf("want LocalDeletes=[song.flac], got %#v", tt.LocalDeletes)
	}
}

func TestComputeLocalDeletePropagatesToRemote(t *testing.T) {
	name, p, local, remote := fixture(t)
	writeFile(t, local, "notes.md", "x")
	writeFile(t, remote, "notes.md", "x")
	markCheckedOut(t, remote)
	baselineFromLocal(t, name, local)

	// Delete on the local side only.
	if err := os.Remove(filepath.Join(local, "notes.md")); err != nil {
		t.Fatal(err)
	}

	st, err := Compute(name, p)
	if err != nil {
		t.Fatal(err)
	}
	tt := only(t, st)
	if len(tt.RemoteDeletes) != 1 || tt.RemoteDeletes[0] != "notes.md" {
		t.Errorf("want RemoteDeletes=[notes.md], got %#v", tt.RemoteDeletes)
	}
}

func TestComputeLocalAddIsPush(t *testing.T) {
	name, p, local, remote := fixture(t)
	markCheckedOut(t, remote)
	baselineFromLocal(t, name, local) // empty baseline
	writeFile(t, local, "new.txt", "fresh")

	st, err := Compute(name, p)
	if err != nil {
		t.Fatal(err)
	}
	tt := only(t, st)
	if len(tt.Push) != 1 || tt.Push[0].Path != "new.txt" || tt.Push[0].Modify {
		t.Errorf("want a single add push of new.txt, got %#v", tt.Push)
	}
}

func TestComputeRemoteAddIsPull(t *testing.T) {
	name, p, local, remote := fixture(t)
	markCheckedOut(t, remote)
	baselineFromLocal(t, name, local) // empty baseline
	writeFile(t, remote, "incoming.txt", "fresh")

	st, err := Compute(name, p)
	if err != nil {
		t.Fatal(err)
	}
	tt := only(t, st)
	if len(tt.Pull) != 1 || tt.Pull[0].Path != "incoming.txt" || tt.Pull[0].Modify {
		t.Errorf("want a single add pull of incoming.txt, got %#v", tt.Pull)
	}
}

func TestComputeModifyFlaggedOnEditedFile(t *testing.T) {
	name, p, local, remote := fixture(t)
	writeFile(t, local, "doc.txt", "v1")
	writeFile(t, remote, "doc.txt", "v1")
	markCheckedOut(t, remote)
	baselineFromLocal(t, name, local)

	// Edit locally: an in-baseline file that changed is a modify push.
	writeFile(t, local, "doc.txt", "v2-longer")

	st, err := Compute(name, p)
	if err != nil {
		t.Fatal(err)
	}
	tt := only(t, st)
	if len(tt.Push) != 1 || !tt.Push[0].Modify {
		t.Errorf("want a modify push, got %#v", tt.Push)
	}
}

func TestComputeConflict(t *testing.T) {
	name, p, local, remote := fixture(t)
	writeFile(t, local, "f.txt", "base")
	writeFile(t, remote, "f.txt", "base")
	markCheckedOut(t, remote)
	baselineFromLocal(t, name, local)

	writeFile(t, local, "f.txt", "local-edit")
	writeFile(t, remote, "f.txt", "remote-edit")

	st, err := Compute(name, p)
	if err != nil {
		t.Fatal(err)
	}
	tt := only(t, st)
	if len(tt.Conflicts) != 1 || tt.Conflicts[0] != "f.txt" {
		t.Errorf("want Conflicts=[f.txt], got %#v", tt.Conflicts)
	}
}

func TestComputeRemoteNotMounted(t *testing.T) {
	name, p, _, _ := fixture(t)
	p.RemoteRoot = filepath.Join(t.TempDir(), "not-mounted")
	_, err := Compute(name, p)
	if err == nil || err.Error() != "remote root "+p.RemoteRoot+" is not mounted" {
		t.Fatalf("err = %v, want not-mounted error", err)
	}
}

func TestProfileStatusInSync(t *testing.T) {
	if !(ProfileStatus{CheckedOut: true, HasBaseline: true}).InSync() {
		t.Error("no targets should be in sync")
	}
	if !(ProfileStatus{Targets: []TargetStatus{{}}}).InSync() {
		t.Error("an empty target should be in sync")
	}
	if (ProfileStatus{Targets: []TargetStatus{{Push: []Change{{Path: "x"}}}}}).InSync() {
		t.Error("a pending push means not in sync")
	}
	if (ProfileStatus{Targets: []TargetStatus{{LocalDeletes: []string{"y"}}}}).InSync() {
		t.Error("a pending local delete means not in sync")
	}
}

func TestTargetStatusLabel(t *testing.T) {
	if got := (TargetStatus{}).Label(); got != "(root)" {
		t.Errorf("Label() = %q, want (root)", got)
	}
	if got := (TargetStatus{Subpath: "albums/live"}).Label(); got != "albums/live" {
		t.Errorf("Label() = %q, want albums/live", got)
	}
}

// TestComputeGroupsBySubpath: a change is reported under the subpath (target) it
// falls within, and only there.
func TestComputeGroupsBySubpath(t *testing.T) {
	name, p, local, remote := fixture(t)
	p.Subpaths = []string{"albums", "stems"}
	markCheckedOut(t, remote)
	baselineFromLocal(t, name, local) // empty baseline
	// A new local file under albums/ only.
	writeFile(t, local, "albums/track.flac", "audio")

	st, err := Compute(name, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Targets) != 2 {
		t.Fatalf("want two targets (albums, stems), got %#v", st.Targets)
	}
	albums, stems := st.Targets[0], st.Targets[1]
	if albums.Subpath != "albums" || len(albums.Push) != 1 || albums.Push[0].Path != "albums/track.flac" {
		t.Errorf("albums target = %#v, want a single push of albums/track.flac", albums)
	}
	if stems.Subpath != "stems" || !stems.InSync() {
		t.Errorf("stems target = %#v, want in sync", stems)
	}
}

func TestComputeInvalidSubpath(t *testing.T) {
	name, p, _, remote := fixture(t)
	p.Subpaths = []string{"../escape"}
	markCheckedOut(t, remote)
	baselineFromLocal(t, name, p.LocalRoot)
	if _, err := Compute(name, p); err == nil {
		t.Fatal("want an error for a subpath escaping the root")
	}
}
