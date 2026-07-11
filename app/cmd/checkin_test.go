package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/lifecycle"
	"github.com/andresbott/netcheckout/internal/marker"
)

func TestCheckinCommandReleases(t *testing.T) {
	cfgPath, remote := heldCmdFixture(t) // shared with sync_test.go (same package)
	cfg, _ := config.Load(cfgPath)
	lroot := cfg.Profiles["work"].LocalRoot
	_ = os.WriteFile(filepath.Join(lroot, "keep.txt"), []byte("FINAL"), 0o644)

	cmd := newCheckinCmdWithRunner(&cfgPath, lifecycle.Runner{Syncer: cmdCopySyncer{}, ToolVersion: "test"})
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"work"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := marker.Read(remote); ok {
		t.Error("checkin must remove the marker")
	}
}

func TestCheckinRegisteredOnRoot(t *testing.T) {
	for _, c := range newRootCommand().Commands() {
		if c.Name() == "checkin" {
			return
		}
	}
	t.Fatal("checkin not registered")
}
