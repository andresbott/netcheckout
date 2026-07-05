package cmd

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/rsync"
)

// fakeDiffer returns a canned Diff per direction, or a fixed error for every
// call if err is set.
type fakeDiffer struct {
	diffs map[rsync.Direction]rsync.Diff
	err   error
}

func (f fakeDiffer) Diff(_ context.Context, j rsync.Job) (rsync.Diff, error) {
	if f.err != nil {
		return rsync.Diff{}, f.err
	}
	return f.diffs[j.Direction], nil
}

func writeStatusTestConfig(t *testing.T, profiles map[string]config.Profile) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(p, &config.Config{Profiles: profiles}); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStatusCommandMissingArg(t *testing.T) {
	cfgPath := ""
	cmd := newStatusCmdWithDiffer(&cfgPath, fakeDiffer{})
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
	cmd := newStatusCmdWithDiffer(&cfgPath, fakeDiffer{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"missing-profile"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `"missing-profile" not found`) {
		t.Fatalf("err = %v", err)
	}
}

func TestStatusCommandPrintsInSync(t *testing.T) {
	remoteRoot := t.TempDir()
	localRoot := t.TempDir()
	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: localRoot, RemoteRoot: remoteRoot},
	})
	d := fakeDiffer{diffs: map[rsync.Direction]rsync.Diff{
		rsync.Pull: {InSync: true},
		rsync.Push: {InSync: true},
	}}
	cmd := newStatusCmdWithDiffer(&cfgPath, d)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"work"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "work") || !strings.Contains(out, "in sync") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestStatusCommandPrintsDifferences(t *testing.T) {
	remoteRoot := t.TempDir()
	localRoot := t.TempDir()
	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: localRoot, RemoteRoot: remoteRoot},
	})
	d := fakeDiffer{diffs: map[rsync.Direction]rsync.Diff{
		rsync.Pull: {InSync: true},
		rsync.Push: {Changes: []rsync.Change{{Path: "report.pdf", Type: rsync.Created}}},
	}}
	cmd := newStatusCmdWithDiffer(&cfgPath, d)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"work"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "+ report.pdf") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestStatusCommandRemoteNotMounted(t *testing.T) {
	localRoot := t.TempDir()
	missingRemote := filepath.Join(t.TempDir(), "gone")
	cfgPath := writeStatusTestConfig(t, map[string]config.Profile{
		"work": {LocalRoot: localRoot, RemoteRoot: missingRemote},
	})
	cmd := newStatusCmdWithDiffer(&cfgPath, fakeDiffer{})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"work"})
	err := cmd.Execute()
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
