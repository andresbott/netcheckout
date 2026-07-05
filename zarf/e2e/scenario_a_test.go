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

	if !t.Run("status reports no checkout before checkout", func(t *testing.T) {
		if got := snapshot(t, local); len(got) != 0 {
			t.Fatalf("local should start empty, got %d files: %#v", len(got), got)
		}
		stdout, _, exitCode := runCLI(t, configPath, "status", "e2e")
		if exitCode != 0 {
			t.Fatalf("status exit = %d, want 0 (stdout: %s)", exitCode, stdout)
		}
		if !strings.Contains(stdout, "not checked out") {
			t.Fatalf("status stdout = %q, want it to report no checkout", stdout)
		}
	}) {
		t.FailNow()
	}

	if !t.Run("checkout copies remote to local and writes a marker", func(t *testing.T) {
		_, _, exitCode := runCLI(t, configPath, "checkout", "e2e")
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
		stdout, _, exitCode := runCLI(t, configPath, "status", "e2e")
		if exitCode != 0 {
			t.Fatalf("status exit = %d, want 0 (stdout: %s)", exitCode, stdout)
		}
		if !strings.Contains(stdout, "changed") {
			t.Fatalf("status stdout = %q, want it to report local changes", stdout)
		}
	}) {
		t.FailNow()
	}

	if !t.Run("sync propagates local changes to remote and keeps the marker", func(t *testing.T) {
		_, _, exitCode := runCLI(t, configPath, "sync", "e2e")
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
		_, _, exitCode := runCLI(t, configPath, "checkin", "e2e")
		if exitCode != 0 {
			t.Fatalf("checkin exit = %d, want 0", exitCode)
		}
		assertSnapshotsEqual(t, editedSnapshot, snapshot(t, remote))
		if _, err := os.Stat(markerPath(remote)); !os.IsNotExist(err) {
			t.Fatalf("expected marker to be removed after checkin, stat err = %v", err)
		}
	})
}
