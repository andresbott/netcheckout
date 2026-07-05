//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/andresbott/netcheckout/internal/config"
)

// runCLI runs the built netcheckout binary with "--config configPath" plus args, under a
// 30-second timeout. A failure to start the process at all (for example a missing
// binary) is a harness bug, not a scenario outcome, so it calls t.Fatalf directly; a
// normal non-zero exit is returned as exitCode for the caller to assert on.
func runCLI(t *testing.T, configPath string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	fullArgs := append([]string{"--config", configPath}, args...)
	cmd := exec.CommandContext(ctx, binPath, fullArgs...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	stdout = outBuf.String()
	stderr = errBuf.String()

	var exitErr *exec.ExitError
	switch {
	case err == nil:
		exitCode = 0
	case errors.As(err, &exitErr):
		exitCode = exitErr.ExitCode()
	default:
		t.Fatalf("run %s %v: %v (stderr: %s)", binPath, fullArgs, err, stderr)
	}
	return stdout, stderr, exitCode
}

// requireRsync skips the calling test if rsync is not on PATH (mirrors the check in
// internal/rsync/integration_test.go).
func requireRsync(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("rsync"); err != nil {
		t.Skip("rsync not on PATH")
	}
}

// newFixture creates empty local/ and remote/ directories under a fresh t.TempDir().
func newFixture(t *testing.T) (local, remote string) {
	t.Helper()
	root := t.TempDir()
	local = filepath.Join(root, "local")
	remote = filepath.Join(root, "remote")
	for _, dir := range []string{local, remote} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	return local, remote
}

// newRand returns a seeded random source, logging the seed via t.Logf so a failure can
// be reproduced.
func newRand(t *testing.T) *rand.Rand {
	t.Helper()
	seed := uint64(time.Now().UnixNano())
	t.Logf("e2e random seed: %d", seed)
	return rand.New(rand.NewPCG(seed, seed))
}

// randomTree populates dir with 2-4 top-level folders (one level deep) and 4-10 files
// total, scattered between dir and its folders, each containing 16-512 random bytes.
func randomTree(t *testing.T, dir string) {
	t.Helper()
	r := newRand(t)

	numFolders := 2 + r.IntN(3)
	folders := []string{dir}
	for i := 0; i < numFolders; i++ {
		path := filepath.Join(dir, fmt.Sprintf("folder-%d", i))
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		folders = append(folders, path)
	}

	numFiles := 4 + r.IntN(7)
	for i := 0; i < numFiles; i++ {
		target := folders[r.IntN(len(folders))]
		writeRandomFileWithRand(t, r, filepath.Join(target, fmt.Sprintf("file-%d.dat", i)))
	}
}

// writeRandomFile writes 16-512 random bytes to path, creating parent directories as
// needed. Used both by randomTree and by tests that need to add or modify one file.
func writeRandomFile(t *testing.T, path string) {
	t.Helper()
	writeRandomFileWithRand(t, newRand(t), path)
}

func writeRandomFileWithRand(t *testing.T, r *rand.Rand, path string) {
	t.Helper()
	data := make([]byte, 16+r.IntN(497))
	for i := range data {
		data[i] = byte(r.IntN(256))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// snapshot walks root and returns relative path -> file contents for every regular file.
func snapshot(t *testing.T, root string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot %s: %v", root, err)
	}
	return out
}

// assertSnapshotsEqual reports every missing, extra, or differing path between want and
// got, rather than failing on the first difference.
func assertSnapshotsEqual(t *testing.T, want, got map[string][]byte) {
	t.Helper()
	for path, wantData := range want {
		gotData, ok := got[path]
		if !ok {
			t.Errorf("missing path %s", path)
			continue
		}
		if !bytes.Equal(wantData, gotData) {
			t.Errorf("path %s: content differs (want %d bytes, got %d bytes)", path, len(wantData), len(gotData))
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("unexpected extra path %s", path)
		}
	}
}

// writeConfig writes a single-profile config via internal/config.Save (the real
// production writer) to a temp path and returns it.
func writeConfig(t *testing.T, identity, profile, local, remote string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := &config.Config{
		Identity: identity,
		Profiles: map[string]config.Profile{
			profile: {LocalRoot: local, RemoteRoot: remote},
		},
	}
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// markerPath returns the path GOALS.md §5 specifies for a checkout marker on a
// whole-root profile.
func markerPath(remoteRoot string) string {
	return filepath.Join(remoteRoot, ".netcheckout.json")
}

func TestRandomTreeWithinBounds(t *testing.T) {
	dir := t.TempDir()
	randomTree(t, dir)

	var folderCount, fileCount int
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		if d.IsDir() {
			folderCount++
			return nil
		}
		fileCount++
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Size() < 16 || info.Size() > 512 {
			t.Errorf("file %s size = %d, want [16,512]", path, info.Size())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	if folderCount < 2 || folderCount > 4 {
		t.Errorf("folder count = %d, want [2,4]", folderCount)
	}
	if fileCount < 4 || fileCount > 10 {
		t.Errorf("file count = %d, want [4,10]", fileCount)
	}
}

func TestSnapshotCapturesContents(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("world"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := snapshot(t, dir)
	want := map[string][]byte{
		"a.txt":     []byte("hello"),
		"sub/b.txt": []byte("world"),
	}
	assertSnapshotsEqual(t, want, got)
}

func TestAssertSnapshotsEqualPassesOnMatch(t *testing.T) {
	snap := map[string][]byte{"a.txt": []byte("A")}
	ok := t.Run("subcheck", func(t *testing.T) {
		assertSnapshotsEqual(t, snap, snap)
	})
	if !ok {
		t.Fatal("expected assertSnapshotsEqual to pass on identical snapshots")
	}
}

func TestAssertSnapshotsEqualDetectsMismatch(t *testing.T) {
	want := map[string][]byte{"a.txt": []byte("A")}
	got := map[string][]byte{"a.txt": []byte("B")}
	ok := t.Run("subcheck", func(t *testing.T) {
		assertSnapshotsEqual(t, want, got)
	})
	if ok {
		t.Fatal("expected assertSnapshotsEqual to fail the subtest on mismatch")
	}
}

func TestNewFixtureCreatesEmptyLocalAndRemote(t *testing.T) {
	local, remote := newFixture(t)
	for _, dir := range []string{local, remote} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		if len(entries) != 0 {
			t.Fatalf("%s should start empty, has %d entries", dir, len(entries))
		}
	}
}

func TestWriteConfigProducesLoadableProfile(t *testing.T) {
	local, remote := newFixture(t)
	path := writeConfig(t, "e2e-test@localhost", "e2e", local, remote)

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Identity != "e2e-test@localhost" {
		t.Errorf("identity = %q, want %q", cfg.Identity, "e2e-test@localhost")
	}
	profile, ok := cfg.Profiles["e2e"]
	if !ok {
		t.Fatalf("profile %q missing from loaded config", "e2e")
	}
	if profile.LocalRoot != local || profile.RemoteRoot != remote {
		t.Errorf("profile = %+v, want local=%q remote=%q", profile, local, remote)
	}
}

func TestMarkerPathJoinsRemoteRoot(t *testing.T) {
	got := markerPath("/tmp/example/remote")
	want := filepath.Join("/tmp/example/remote", ".netcheckout.json")
	if got != want {
		t.Errorf("markerPath = %q, want %q", got, want)
	}
}
