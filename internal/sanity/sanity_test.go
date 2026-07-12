package sanity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/marker"
)

func TestCheckRootsExistence(t *testing.T) {
	local, remote := t.TempDir(), t.TempDir()
	r := Check(config.Profile{LocalRoot: local, RemoteRoot: remote})
	if !r.LocalRoot || !r.RemoteRoot {
		t.Fatalf("both roots exist, got %+v", r)
	}
	r = Check(config.Profile{
		LocalRoot:  filepath.Join(local, "nope"),
		RemoteRoot: filepath.Join(remote, "nope"),
	})
	if r.LocalRoot || r.RemoteRoot {
		t.Fatalf("missing roots should be false, got %+v", r)
	}
}

func TestCheckRemoteRootMustBeDir(t *testing.T) {
	remote := t.TempDir()
	file := filepath.Join(remote, "afile")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Check(config.Profile{LocalRoot: t.TempDir(), RemoteRoot: file}).RemoteRoot {
		t.Error("RemoteRoot should be false for a regular file")
	}
}

func TestCheckMarkerAtRemoteRoot(t *testing.T) {
	local, remote := t.TempDir(), t.TempDir()
	p := config.Profile{LocalRoot: local, RemoteRoot: remote}
	if Check(p).CheckedOut {
		t.Error("CheckedOut should be false with no marker")
	}
	if err := os.WriteFile(filepath.Join(remote, marker.FileName), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Check(p).CheckedOut {
		t.Error("CheckedOut should be true with a root marker")
	}
}

func TestCheckSubpathsExistOnRemote(t *testing.T) {
	local, remote := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(remote, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	// "b" is intentionally absent.
	r := Check(config.Profile{LocalRoot: local, RemoteRoot: remote, Subpaths: []string{"a", "b"}})
	want := []Subpath{{Path: "a", Exists: true}, {Path: "b", Exists: false}}
	if len(r.Subpaths) != len(want) {
		t.Fatalf("Subpaths = %+v, want %+v", r.Subpaths, want)
	}
	for i := range want {
		if r.Subpaths[i] != want[i] {
			t.Errorf("Subpaths[%d] = %+v, want %+v", i, r.Subpaths[i], want[i])
		}
	}
}

func TestCheckWholeRootHasNoSubpaths(t *testing.T) {
	if r := Check(config.Profile{LocalRoot: t.TempDir(), RemoteRoot: t.TempDir()}); len(r.Subpaths) != 0 {
		t.Errorf("whole-root profile should have no Subpaths, got %+v", r.Subpaths)
	}
}

func TestCheckInvalidSubpathReturnsRootsOnly(t *testing.T) {
	r := Check(config.Profile{LocalRoot: t.TempDir(), RemoteRoot: t.TempDir(), Subpaths: []string{"../escape"}})
	if !r.LocalRoot || !r.RemoteRoot {
		t.Errorf("roots should still be reported on an invalid subpath: %+v", r)
	}
	if len(r.Subpaths) != 0 {
		t.Errorf("Subpaths should be empty when Targets() errors, got %+v", r.Subpaths)
	}
}
