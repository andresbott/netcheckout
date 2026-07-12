package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
)

func writeStatusTestConfig(t *testing.T, profiles map[string]config.Profile) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(p, &config.Config{Profiles: profiles}); err != nil {
		t.Fatal(err)
	}
	return p
}

// statusFixture creates local/ and remote/ roots and points NETCHECKOUT_STATE at
// a temp state dir so status.Compute resolves the baseline there.
func statusFixture(t *testing.T) (local, remote string) {
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
	return local, remote
}

func saveBaseline(t *testing.T, name, local string) {
	t.Helper()
	files, err := baseline.Snapshot(local, []string{"."})
	if err != nil {
		t.Fatal(err)
	}
	if err := baseline.Save(&baseline.Baseline{Profile: name, Relpaths: []string{"."}, Files: files}); err != nil {
		t.Fatal(err)
	}
}

func runStatus(t *testing.T, cfgPath, profile string) (string, error) {
	t.Helper()
	cmd := newStatusCmd(&cfgPath)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{profile})
	err := cmd.Execute()
	return buf.String(), err
}

func TestStatusCommandMissingArg(t *testing.T) {
	cfgPath := ""
	cmd := newStatusCmd(&cfgPath)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("want error for missing profile argument")
	}
}

func TestStatusCommandUnknownProfile(t *testing.T) {
	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: t.TempDir(), RemoteRoot: t.TempDir()},
	})
	_, err := runStatus(t, cfgPath, "missing-profile")
	if err == nil || !strings.Contains(err.Error(), `"missing-profile" not found`) {
		t.Fatalf("err = %v", err)
	}
}

func TestStatusCommandPrintsInSync(t *testing.T) {
	local, remote := statusFixture(t)
	writeCheckoutMarker(t, remote)
	saveBaseline(t, "work", local) // empty baseline, empty trees: in sync
	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: local, RemoteRoot: remote},
	})
	out, err := runStatus(t, cfgPath, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "work") || !strings.Contains(out, "in sync") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestStatusCommandPrintsPushDifference(t *testing.T) {
	local, remote := statusFixture(t)
	writeCheckoutMarker(t, remote)
	saveBaseline(t, "work", local) // empty baseline
	// A brand-new local file is a push (add).
	if err := os.WriteFile(filepath.Join(local, "report.pdf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: local, RemoteRoot: remote},
	})
	out, err := runStatus(t, cfgPath, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "push") || !strings.Contains(out, "+ report.pdf") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

// TestStatusCommandPrintsLocalDelete pins the fix at the CLI layer: a remote-side
// deletion is reported as a local delete, not a push.
func TestStatusCommandPrintsLocalDelete(t *testing.T) {
	local, remote := statusFixture(t)
	if err := os.WriteFile(filepath.Join(local, "keep.dat"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remote, "keep.dat"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeCheckoutMarker(t, remote)
	saveBaseline(t, "work", local)
	if err := os.Remove(filepath.Join(remote, "keep.dat")); err != nil {
		t.Fatal(err)
	}
	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: local, RemoteRoot: remote},
	})
	out, err := runStatus(t, cfgPath, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "del-local") || !strings.Contains(out, "- keep.dat") {
		t.Fatalf("want a local-delete of keep.dat, got:\n%s", out)
	}
	if strings.Contains(out, "push") {
		t.Fatalf("a remote delete must not be reported as a push:\n%s", out)
	}
}

func TestStatusCommandRemoteNotMounted(t *testing.T) {
	local := t.TempDir()
	missingRemote := filepath.Join(t.TempDir(), "gone")
	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: local, RemoteRoot: missingRemote},
	})
	_, err := runStatus(t, cfgPath, "work")
	if err == nil || !strings.Contains(err.Error(), "is not mounted") {
		t.Fatalf("err = %v", err)
	}
}

func TestStatusRegisteredOnRoot(t *testing.T) {
	root := newRootCommand()
	for _, c := range root.Commands() {
		if c.Name() == "status" {
			return
		}
	}
	t.Fatal("status command not registered on root")
}

func writeCheckoutMarker(t *testing.T, remoteRoot string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(remoteRoot, ".netcheckout.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStatusCommandReportsNotCheckedOut(t *testing.T) {
	local, remote := statusFixture(t)
	// No marker: the profile is not checked out.
	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: local, RemoteRoot: remote},
	})
	out, err := runStatus(t, cfgPath, "work")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "not checked out") {
		t.Fatalf("want 'not checked out', got:\n%s", out)
	}
	if strings.Contains(out, "in sync") {
		t.Fatalf("should not report in sync when not checked out:\n%s", out)
	}
}
