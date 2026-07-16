package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
	"github.com/andresbott/netcheckout/internal/marker"
)

// heldFixture builds a checked-out profile whose local and remote agree on one
// file, with the base established by a real first sync (so mtimes in the base
// come from rsync listings, exactly as production does).
func heldFixture(t *testing.T) (name string, p config.Profile, id ident.Ident) {
	t.Helper()
	requireRsync(t)
	local, remote := fixture(t) // remote holds file.txt; local missing
	id = testIdent()
	p = config.Profile{LocalRoot: local, RemoteRoot: remote}
	if _, err := (Runner{}).Checkout(context.Background(), "work", p, id, "", Options{}); err != nil {
		t.Fatalf("fixture checkout: %v", err)
	}
	// First sync pulls file.txt down and records the base.
	if _, err := (Runner{}).Sync(context.Background(), "work", p, id, "", Options{}); err != nil {
		t.Fatalf("fixture first sync: %v", err)
	}
	return "work", p, id
}

func TestSyncFailFastWithoutMarker(t *testing.T) {
	name, p, id := heldFixture(t)
	_ = marker.Remove(config.ExpandRoot(p.RemoteRoot))
	if _, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{}); err == nil {
		t.Fatal("sync must fail fast when no marker exists")
	}
}

func TestSyncRefusesForeignMarker(t *testing.T) {
	name, p, id := heldFixture(t)
	remote := config.ExpandRoot(p.RemoteRoot)
	_ = marker.Write(remote, &marker.Marker{CheckedOutBy: "other@laptop", Host: "laptop", Profile: name})
	if _, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{}); err == nil {
		t.Fatal("sync must refuse a foreign marker")
	}
}

// Force resolves same-file conflicts local-wins; it must NEVER override the
// lock ownership check (GOALS §9.5, and the sync --force help text).
func TestSyncForceDoesNotOverrideForeignLock(t *testing.T) {
	name, p, id := heldFixture(t)
	remote := config.ExpandRoot(p.RemoteRoot)
	_ = marker.Write(remote, &marker.Marker{CheckedOutBy: "other@laptop", Host: "laptop", Profile: name})
	if _, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{Force: true}); err == nil {
		t.Fatal("sync --force must still refuse a foreign marker")
	}
	// The foreign marker is untouched.
	m, _, _ := marker.Read(remote)
	if m == nil || m.CheckedOutBy != "other@laptop" {
		t.Errorf("foreign marker must be preserved, got %+v", m)
	}
}

// Checkin has no --force at all (GOALS §9): a Force option set by any caller
// must not let it past a foreign lock.
func TestCheckinForceDoesNotOverrideForeignLock(t *testing.T) {
	name, p, id := heldFixture(t)
	remote := config.ExpandRoot(p.RemoteRoot)
	_ = marker.Write(remote, &marker.Marker{CheckedOutBy: "other@laptop", Host: "laptop", Profile: name})
	if _, err := (Runner{}).Checkin(context.Background(), name, p, id, Options{Force: true}); err == nil {
		t.Fatal("checkin must refuse a foreign marker regardless of Force")
	}
	if m, _, _ := marker.Read(remote); m == nil || m.CheckedOutBy != "other@laptop" {
		t.Errorf("foreign marker must be preserved, got %+v", m)
	}
}

func TestSyncFirstPullCopiesRemoteDown(t *testing.T) {
	requireRsync(t)
	local, remote := fixture(t)
	id := testIdent()
	p := config.Profile{LocalRoot: local, RemoteRoot: remote}
	if _, err := (Runner{}).Checkout(context.Background(), "work", p, id, "", Options{}); err != nil {
		t.Fatal(err)
	}
	rep, err := (Runner{}).Sync(context.Background(), "work", p, id, "", Options{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(local, "file.txt")); string(got) != "data" {
		t.Errorf("local file.txt = %q", got)
	}
	if len(rep.Pulled) != 1 || rep.Pulled[0] != "file.txt" {
		t.Errorf("rep.Pulled = %v", rep.Pulled)
	}
}

func TestSyncPushesLocalEdit(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	// Edit locally after checkout (different size => detected without mtime games).
	if err := os.WriteFile(filepath.Join(local, "file.txt"), []byte("EDITED-LONGER"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := Runner{ToolVersion: "test", Now: func() time.Time { return time.Unix(500, 0).UTC() }}
	rep, err := r.Sync(context.Background(), name, p, id, "", Options{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(remote, "file.txt")); string(got) != "EDITED-LONGER" {
		t.Errorf("remote file.txt = %q", got)
	}
	m, _, _ := marker.Read(remote)
	if !m.OwnedBy(id.By, id.Host) {
		t.Error("marker ownership must be preserved")
	}
	if !m.LastSyncAt.Equal(time.Unix(500, 0).UTC()) {
		t.Errorf("last_sync_at = %v", m.LastSyncAt)
	}
	if len(rep.Pushed) != 1 {
		t.Errorf("rep.Pushed = %v", rep.Pushed)
	}
}

func TestSyncForwardsApplyEvents(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	if err := os.WriteFile(filepath.Join(local, "file.txt"), []byte("EDITED-LONGER"), 0o644); err != nil {
		t.Fatal(err)
	}
	var events []Event
	_, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{
		OnApply: func(e Event) { events = append(events, e) },
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	want := Event{Kind: EventModify, Side: SideRemote, Path: "file.txt"}
	if len(events) != 1 || events[0] != want {
		t.Fatalf("events = %+v, want [%+v]", events, want)
	}
}

func TestSyncAddEventForNewFile(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	if err := os.WriteFile(filepath.Join(local, "new.txt"), []byte("N"), 0o644); err != nil {
		t.Fatal(err)
	}
	var events []Event
	if _, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{
		OnApply: func(e Event) { events = append(events, e) },
	}); err != nil {
		t.Fatal(err)
	}
	want := Event{Kind: EventAdd, Side: SideRemote, Path: "new.txt"}
	if len(events) != 1 || events[0] != want {
		t.Fatalf("events = %+v, want [%+v]", events, want)
	}
}

func TestSyncDryRunEmitsNoEventsAndWritesNothing(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	if err := os.WriteFile(filepath.Join(local, "file.txt"), []byte("EDITED-LONGER"), 0o644); err != nil {
		t.Fatal(err)
	}
	var events []Event
	rep, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{
		DryRun:  true,
		OnApply: func(e Event) { events = append(events, e) },
	})
	if err != nil {
		t.Fatalf("dry-run sync: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("dry run must emit no events, got %+v", events)
	}
	if len(rep.Pushed) != 1 {
		t.Errorf("dry-run plan: %+v", rep)
	}
	if got, _ := os.ReadFile(filepath.Join(remote, "file.txt")); string(got) != "data" {
		t.Errorf("dry run wrote to the remote: %q", got)
	}
}

func TestSyncConflictStops(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	// Edit both sides to different sizes => conflict.
	if err := os.WriteFile(filepath.Join(local, "file.txt"), []byte("local-version"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remote, "file.txt"), []byte("R"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{})
	var ce *ConflictError
	if !errors.As(err, &ce) {
		t.Fatalf("err = %v, want ConflictError", err)
	}
	if len(rep.Conflicts) != 1 || rep.Conflicts[0] != "file.txt" {
		t.Errorf("rep.Conflicts = %v", rep.Conflicts)
	}
	// Nothing was written on either side.
	if got, _ := os.ReadFile(filepath.Join(local, "file.txt")); string(got) != "local-version" {
		t.Errorf("local overwritten: %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(remote, "file.txt")); string(got) != "R" {
		t.Errorf("remote overwritten: %q", got)
	}
}

func TestSyncForceResolvesConflictLocalWins(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	if err := os.WriteFile(filepath.Join(local, "file.txt"), []byte("local-version"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remote, "file.txt"), []byte("R"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{Force: true})
	if err != nil {
		t.Fatalf("force sync: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(remote, "file.txt")); string(got) != "local-version" {
		t.Errorf("remote = %q, want local content", got)
	}
	if len(rep.Pushed) != 1 {
		t.Errorf("rep.Pushed = %v", rep.Pushed)
	}
}

func TestSyncDryRunOverConflictDoesNotError(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	if err := os.WriteFile(filepath.Join(local, "file.txt"), []byte("local-version"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remote, "file.txt"), []byte("R"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run over conflict: %v", err)
	}
	if len(rep.Conflicts) != 1 {
		t.Errorf("rep.Conflicts = %v", rep.Conflicts)
	}
}

func TestSyncPropagatesLocalDelete(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	// A second synced file keeps the tree non-empty after the delete, so the
	// engine's empty-endpoint valve (deleting the LAST file needs --force or
	// AcceptEmpty) is not what this test exercises.
	if err := os.WriteFile(filepath.Join(local, "other.txt"), []byte("O"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(local, "file.txt")); err != nil {
		t.Fatal(err)
	}
	rep, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(remote, "file.txt")); !os.IsNotExist(err) {
		t.Error("remote file must be deleted")
	}
	if len(rep.RemovedRemote) != 1 {
		t.Errorf("rep.RemovedRemote = %v", rep.RemovedRemote)
	}
}

func TestSyncScopedRelpathOnlyTouchesScope(t *testing.T) {
	requireRsync(t)
	local, remote := fixture(t)
	// Remote gets a second top-level dir beside file.txt.
	if err := os.MkdirAll(filepath.Join(remote, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remote, "docs", "d.txt"), []byte("D"), 0o644); err != nil {
		t.Fatal(err)
	}
	id := testIdent()
	p := config.Profile{LocalRoot: local, RemoteRoot: remote}
	if _, err := (Runner{}).Checkout(context.Background(), "work", p, id, "", Options{}); err != nil {
		t.Fatal(err)
	}
	// Scoped sync of docs only: file.txt must not be pulled.
	rep, err := (Runner{}).Sync(context.Background(), "work", p, id, "docs", Options{})
	if err != nil {
		t.Fatalf("scoped sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(local, "docs", "d.txt")); err != nil {
		t.Errorf("scoped file not pulled: %v", err)
	}
	if _, err := os.Stat(filepath.Join(local, "file.txt")); !os.IsNotExist(err) {
		t.Error("out-of-scope file must not be pulled")
	}
	if len(rep.Pulled) != 1 || rep.Pulled[0] != "docs/d.txt" {
		t.Errorf("rep.Pulled = %v", rep.Pulled)
	}
}

// A missing local root while the baseline records synced files is what an
// unmounted local disk (or a wrongly deleted working copy) looks like.
// Recreating it as an empty dir would classify every baseline file as a local
// delete and push the deletions to the remote — sync must refuse instead.
func TestSyncRefusesMissingLocalRootWithBaseline(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	if err := os.RemoveAll(local); err != nil {
		t.Fatal(err)
	}
	if _, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{}); err == nil {
		t.Fatal("sync must refuse a missing local root when the baseline records files")
	}
	// The remote is untouched.
	if got, _ := os.ReadFile(filepath.Join(remote, "file.txt")); string(got) != "data" {
		t.Errorf("remote file.txt = %q, want untouched", got)
	}
	// And the local root was NOT recreated.
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Error("sync must not create the local root over a non-empty baseline")
	}
}

// A dry-run right after checkout (local root not yet created) must preview the
// pull plan without creating the local root — GOALS §9.5: dry-run mutates nothing.
func TestSyncDryRunFreshCheckoutCreatesNothing(t *testing.T) {
	requireRsync(t)
	local, remote := fixture(t)
	id := testIdent()
	p := config.Profile{LocalRoot: local, RemoteRoot: remote}
	if _, err := (Runner{}).Checkout(context.Background(), "work", p, id, "", Options{}); err != nil {
		t.Fatal(err)
	}
	rep, err := (Runner{}).Sync(context.Background(), "work", p, id, "", Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run sync: %v", err)
	}
	if len(rep.Pulled) != 1 || rep.Pulled[0] != "file.txt" {
		t.Errorf("rep.Pulled = %v", rep.Pulled)
	}
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Error("dry-run must not create the local root")
	}
}

// Checkin only diffs (it copies nothing), so it too must not create the local
// root as a side effect.
func TestCheckinFreshCheckoutCreatesNoLocalRoot(t *testing.T) {
	requireRsync(t)
	local, remote := fixture(t)
	id := testIdent()
	p := config.Profile{LocalRoot: local, RemoteRoot: remote}
	if _, err := (Runner{}).Checkout(context.Background(), "work", p, id, "", Options{}); err != nil {
		t.Fatal(err)
	}
	// Refuses (the remote file is an un-pulled change) — and mutates nothing.
	if _, err := (Runner{}).Checkin(context.Background(), "work", p, id, Options{}); err == nil {
		t.Fatal("checkin must refuse an unsynced fresh checkout")
	}
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Error("checkin must not create the local root")
	}
}

// The baseline is keyed by profile name; if the profile's roots were edited
// since the checkout, merging against the old manifest would manufacture
// deletes and conflicts against the wrong trees. Sync must refuse instead.
func TestSyncRefusesEditedRoots(t *testing.T) {
	name, p, id := heldFixture(t)
	// Re-point the profile at different roots, as a config edit would.
	edited := p
	edited.RemoteRoot = t.TempDir()
	// Keep it "checked out" at the new remote so preflight reaches the state check.
	if err := marker.Write(edited.RemoteRoot, &marker.Marker{CheckedOutBy: id.By, Host: id.Host, Profile: name}); err != nil {
		t.Fatal(err)
	}
	_, err := (Runner{}).Sync(context.Background(), name, edited, id, "", Options{})
	if err == nil {
		t.Fatal("sync must refuse when the profile's roots changed since checkout")
	}
	if !strings.Contains(err.Error(), "root") {
		t.Errorf("error should mention the root mismatch, got: %v", err)
	}
}

// Deleting the only checked-out file is a legitimate small delete: it must
// propagate on a plain sync (the fraction guard's absolute floor keeps small
// deletions out of the mass-deletion valve, and a MISSING local root — the
// unmounted-disk case — is a hard error before any delete is planned).
func TestSyncDeletesLastFile(t *testing.T) {
	name, p, id := heldFixture(t)
	local := config.ExpandRoot(p.LocalRoot)
	remote := config.ExpandRoot(p.RemoteRoot)
	if err := os.Remove(filepath.Join(local, "file.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := (Runner{}).Sync(context.Background(), name, p, id, "", Options{}); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(remote, "file.txt")); !os.IsNotExist(err) {
		t.Error("remote file must be deleted")
	}
}

// A mass deletion (e.g. a directory rename shows up as delete-all + add-all)
// trips the engine's delete-fraction guard; the error must tell the user about
// --allow-deletes, not about a Go API option, and --allow-deletes must let the
// legitimate rename through.
func TestSyncMassDeleteNeedsAllowDeletes(t *testing.T) {
	requireRsync(t)
	local, remote := fixture(t)
	// 12 files under docs/ (above the guard's absolute floor of 10).
	for i := 0; i < 12; i++ {
		dir := filepath.Join(remote, "docs")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%02d.txt", i)), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	id := testIdent()
	p := config.Profile{LocalRoot: local, RemoteRoot: remote}
	if _, err := (Runner{}).Checkout(context.Background(), "work", p, id, "", Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := (Runner{}).Sync(context.Background(), "work", p, id, "", Options{}); err != nil {
		t.Fatal(err)
	}
	// Rename docs -> papers locally: 12 deletes + 12 adds against a 13-file base.
	if err := os.Rename(filepath.Join(local, "docs"), filepath.Join(local, "papers")); err != nil {
		t.Fatal(err)
	}
	_, err := (Runner{}).Sync(context.Background(), "work", p, id, "", Options{})
	if err == nil {
		t.Fatal("mass delete must trip the guard")
	}
	if !strings.Contains(err.Error(), "--allow-deletes") {
		t.Errorf("error must point at --allow-deletes, got: %v", err)
	}
	if _, err := (Runner{}).Sync(context.Background(), "work", p, id, "", Options{AllowDeletes: true}); err != nil {
		t.Fatalf("sync --allow-deletes after rename: %v", err)
	}
	if _, err := os.Stat(filepath.Join(remote, "papers", "f00.txt")); err != nil {
		t.Error("renamed dir must land on the remote")
	}
	// The old files must be gone (the empty docs/ dir itself may linger —
	// the engine tracks regular files only).
	if _, err := os.Stat(filepath.Join(remote, "docs", "f00.txt")); !os.IsNotExist(err) {
		t.Error("old files must be deleted from the remote")
	}
}
