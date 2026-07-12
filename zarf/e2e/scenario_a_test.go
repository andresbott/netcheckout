//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScenarioA(t *testing.T) {
	requireRsync(t)

	local, remote := newFixture(t)
	randomTree(t, remote)
	remoteSnapshot := snapshot(t, remote)
	configPath := writeConfig(t, "e2e-test@localhost", "e2e", local, remote)
	state := t.TempDir()
	env := []string{"NETCHECKOUT_STATE=" + state}

	if !t.Run("status reports no checkout before checkout", func(t *testing.T) {
		if got := snapshot(t, local); len(got) != 0 {
			t.Fatalf("local should start empty, got %d files: %#v", len(got), got)
		}
		stdout, _, exitCode := runCLIEnv(t, configPath, env, "status", "e2e")
		if exitCode != 0 {
			t.Fatalf("status exit = %d, want 0 (stdout: %s)", exitCode, stdout)
		}
		if !strings.Contains(stdout, "not checked out") {
			t.Fatalf("status stdout = %q, want it to report no checkout", stdout)
		}
	}) {
		t.FailNow()
	}

	if !t.Run("checkin refuses a profile that is not checked out", func(t *testing.T) {
		stdout, _, exitCode := runCLIEnv(t, configPath, env, "checkin", "e2e")
		if exitCode == 0 {
			t.Fatal("checkin of a not-checked-out profile should fail, got exit 0")
		}
		if !strings.Contains(stdout, "not checked out") {
			t.Fatalf("checkin output = %q, want it to report the profile is not checked out", stdout)
		}
	}) {
		t.FailNow()
	}

	if !t.Run("checkout copies remote to local and writes a marker", func(t *testing.T) {
		_, _, exitCode := runCLIEnv(t, configPath, env, "checkout", "e2e")
		if exitCode != 0 {
			t.Fatalf("checkout exit = %d, want 0", exitCode)
		}
		assertSnapshotsEqual(t, remoteSnapshot, snapshot(t, local))
		if _, err := os.Stat(markerPath(remote)); err != nil {
			t.Fatalf("expected checkout marker at %s: %v", markerPath(remote), err)
		}
	}) {
		t.FailNow()
	}

	if !t.Run("checkout refuses a second checkout while held", func(t *testing.T) {
		stdout, _, exitCode := runCLIEnv(t, configPath, env, "checkout", "e2e")
		if exitCode == 0 {
			t.Fatal("a second checkout of a held profile should fail, got exit 0")
		}
		if !strings.Contains(stdout, "already checked out") {
			t.Fatalf("checkout output = %q, want it to report the profile is already checked out", stdout)
		}
	}) {
		t.FailNow()
	}

	var editedSnapshot map[string][]byte
	if !t.Run("editing the local copy", func(t *testing.T) {
		before := snapshot(t, local)
		var existing string
		for rel := range before {
			existing = rel
			break
		}
		writeRandomFile(t, filepath.Join(local, existing))
		writeRandomFile(t, filepath.Join(local, "e2e-added.dat"))
		editedSnapshot = snapshot(t, local)
	}) {
		t.FailNow()
	}

	if !t.Run("status reports local changes after editing", func(t *testing.T) {
		stdout, _, exitCode := runCLIEnv(t, configPath, env, "status", "e2e")
		if exitCode != 0 {
			t.Fatalf("status exit = %d, want 0 (stdout: %s)", exitCode, stdout)
		}
		if !strings.Contains(stdout, "e2e-added.dat") {
			t.Fatalf("status stdout = %q, want it to report the locally added file", stdout)
		}
	}) {
		t.FailNow()
	}

	if !t.Run("sync propagates local changes to remote and keeps the marker", func(t *testing.T) {
		_, _, exitCode := runCLIEnv(t, configPath, env, "sync", "e2e")
		if exitCode != 0 {
			t.Fatalf("sync exit = %d, want 0", exitCode)
		}
		assertSnapshotsEqual(t, editedSnapshot, snapshot(t, remote))
		if _, err := os.Stat(markerPath(remote)); err != nil {
			t.Fatalf("expected marker to remain after sync at %s: %v", markerPath(remote), err)
		}
	}) {
		t.FailNow()
	}

	t.Run("checkin propagates changes and clears the marker", func(t *testing.T) {
		_, _, exitCode := runCLIEnv(t, configPath, env, "checkin", "e2e")
		if exitCode != 0 {
			t.Fatalf("checkin exit = %d, want 0", exitCode)
		}
		assertSnapshotsEqual(t, editedSnapshot, snapshot(t, remote))
		if _, err := os.Stat(markerPath(remote)); !os.IsNotExist(err) {
			t.Fatalf("expected marker to be removed after checkin, stat err = %v", err)
		}
	})
}
