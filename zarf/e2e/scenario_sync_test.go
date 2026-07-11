//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSyncPullsRemoteAdd(t *testing.T) {
	requireRsync(t)
	local, remote := newFixture(t)
	writeRandomFile(t, filepath.Join(remote, "seed.dat"))
	cfg := writeConfig(t, "e2e@localhost", "e2e", local, remote)
	state := t.TempDir()
	env := []string{"NETCHECKOUT_STATE=" + state}

	if _, _, code := runCLIEnv(t, cfg, env, "checkout", "e2e"); code != 0 {
		t.Fatalf("checkout exit %d", code)
	}
	// Add a brand-new file on the remote only.
	writeRandomFile(t, filepath.Join(remote, "remote-add.dat"))
	if _, _, code := runCLIEnv(t, cfg, env, "sync", "e2e"); code != 0 {
		t.Fatalf("sync exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(local, "remote-add.dat")); err != nil {
		t.Errorf("remote-only add should be pulled down: %v", err)
	}
}

func TestSyncDisambiguatesDeleteVsAdd(t *testing.T) {
	requireRsync(t)
	local, remote := newFixture(t)
	writeRandomFile(t, filepath.Join(remote, "from-checkout.dat"))
	cfg := writeConfig(t, "e2e@localhost", "e2e", local, remote)
	state := t.TempDir()
	env := []string{"NETCHECKOUT_STATE=" + state}

	if _, _, code := runCLIEnv(t, cfg, env, "checkout", "e2e"); code != 0 {
		t.Fatalf("checkout exit %d", code)
	}
	// (a) delete a checked-out file locally.
	if err := os.Remove(filepath.Join(local, "from-checkout.dat")); err != nil {
		t.Fatal(err)
	}
	// (b) add a brand-new file on the remote.
	writeRandomFile(t, filepath.Join(remote, "brand-new.dat"))

	if _, _, code := runCLIEnv(t, cfg, env, "sync", "e2e"); code != 0 {
		t.Fatalf("sync exit %d", code)
	}
	// (a) the local delete propagated: gone on the remote.
	if _, err := os.Stat(filepath.Join(remote, "from-checkout.dat")); !os.IsNotExist(err) {
		t.Error("local delete should propagate to the remote (file must be gone)")
	}
	// (b) the remote add was pulled: present locally.
	if _, err := os.Stat(filepath.Join(local, "brand-new.dat")); err != nil {
		t.Errorf("remote add should be pulled locally: %v", err)
	}
}

func TestSyncConflictStopsWithoutWriting(t *testing.T) {
	requireRsync(t)
	local, remote := newFixture(t)
	writeRandomFile(t, filepath.Join(remote, "F.dat"))
	cfg := writeConfig(t, "e2e@localhost", "e2e", local, remote)
	state := t.TempDir()
	env := []string{"NETCHECKOUT_STATE=" + state}

	if _, _, code := runCLIEnv(t, cfg, env, "checkout", "e2e"); code != 0 {
		t.Fatalf("checkout exit %d", code)
	}
	// Edit F on BOTH sides with distinct content.
	if err := os.WriteFile(filepath.Join(local, "F.dat"), []byte("LOCAL-EDIT"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remote, "F.dat"), []byte("REMOTE-EDIT"), 0o644); err != nil {
		t.Fatal(err)
	}
	remoteBefore := snapshot(t, remote)

	stdout, _, code := runCLIEnv(t, cfg, env, "sync", "e2e")
	if code == 0 {
		t.Fatalf("sync should exit non-zero on a conflict; stdout: %s", stdout)
	}
	// Remote byte-for-byte unchanged.
	assertSnapshotsEqual(t, remoteBefore, snapshot(t, remote))
	if _, err := os.Stat(markerPath(remote)); err != nil {
		t.Errorf("marker must be untouched on conflict: %v", err)
	}
}

func TestSyncFailsFastWithoutLock(t *testing.T) {
	requireRsync(t)
	local, remote := newFixture(t)
	writeRandomFile(t, filepath.Join(remote, "x.dat"))
	cfg := writeConfig(t, "e2e@localhost", "e2e", local, remote)
	state := t.TempDir()
	env := []string{"NETCHECKOUT_STATE=" + state}
	remoteBefore := snapshot(t, remote)

	// No marker at all.
	if _, _, code := runCLIEnv(t, cfg, env, "sync", "e2e"); code == 0 {
		t.Fatal("sync must fail fast with no marker")
	}
	assertSnapshotsEqual(t, remoteBefore, snapshot(t, remote))

	// A foreign marker.
	if err := os.WriteFile(markerPath(remote), []byte(`{"checked_out_by":"alice@nas","host":"nas","profile":"e2e"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, code := runCLIEnv(t, cfg, env, "sync", "e2e"); code == 0 {
		t.Fatal("sync must fail fast against a foreign marker")
	}
	assertSnapshotsEqual(t, remoteBefore, snapshot(t, remote))
}

func TestSyncDryRunMutatesNothing(t *testing.T) {
	requireRsync(t)
	local, remote := newFixture(t)
	writeRandomFile(t, filepath.Join(remote, "d.dat"))
	cfg := writeConfig(t, "e2e@localhost", "e2e", local, remote)
	state := t.TempDir()
	env := []string{"NETCHECKOUT_STATE=" + state}

	if _, _, code := runCLIEnv(t, cfg, env, "checkout", "e2e"); code != 0 {
		t.Fatalf("checkout exit %d", code)
	}
	writeRandomFile(t, filepath.Join(local, "local-only.dat"))
	remoteBefore := snapshot(t, remote)

	if _, _, code := runCLIEnv(t, cfg, env, "sync", "e2e", "--dry-run"); code != 0 {
		t.Fatalf("dry-run sync exit %d", code)
	}
	assertSnapshotsEqual(t, remoteBefore, snapshot(t, remote))
}
