package reconcile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/andresbott/netcheckout/internal/baseline"
)

func TestClassifyPathTable(t *testing.T) {
	// s(present, changed) builds a side state.
	type side struct{ present, changed bool }
	cases := []struct {
		name   string
		inBase bool
		local  side
		remote side
		want   action
	}{
		{"local edit, remote unchanged", true, side{true, true}, side{true, false}, actPush},
		{"remote edit, local unchanged", true, side{true, false}, side{true, true}, actPull},
		{"both edited", true, side{true, true}, side{true, true}, actConflict},
		{"remote addition", false, side{false, false}, side{true, false}, actPull},
		{"local addition", false, side{true, false}, side{false, false}, actPush},
		{"local delete (remote unchanged)", true, side{false, false}, side{true, false}, actRemoteDelete},
		{"remote delete (local unchanged)", true, side{true, false}, side{false, false}, actLocalDelete},
		{"both unchanged", true, side{true, false}, side{true, false}, actNoop},
		{"both deleted", true, side{false, false}, side{false, false}, actNoop},
		{"local delete vs remote edit", true, side{false, false}, side{true, true}, actConflict},
		{"remote delete vs local edit", true, side{true, true}, side{false, false}, actConflict},
		{"both added same path", false, side{true, false}, side{true, false}, actConflict},
		{"not in base, neither present", false, side{false, false}, side{false, false}, actNoop},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyPath(c.inBase, c.local.present, c.local.changed, c.remote.present, c.remote.changed)
			if got != c.want {
				t.Errorf("classifyPath = %v, want %v", got, c.want)
			}
		})
	}
}

func TestClassifyDisambiguatesDeleteFromAdd(t *testing.T) {
	localRoot := t.TempDir()
	remoteRoot := t.TempDir()
	// gone.txt: was in base, present on remote, absent locally => local delete => RemoteDelete.
	if err := os.WriteFile(filepath.Join(remoteRoot, "gone.txt"), []byte("g"), 0o644); err != nil {
		t.Fatal(err)
	}
	// new.txt: NOT in base, present on remote only => remote addition => Pull.
	if err := os.WriteFile(filepath.Join(remoteRoot, "new.txt"), []byte("n"), 0o644); err != nil {
		t.Fatal(err)
	}
	goneInfo, _ := os.Stat(filepath.Join(remoteRoot, "gone.txt"))
	base := map[string]baseline.FileState{
		"gone.txt": {Size: 1, ModTime: goneInfo.ModTime(), Hash: hashOf(t, filepath.Join(remoteRoot, "gone.txt"))},
	}
	local := map[string]baseline.FileState{} // both absent locally
	remote := map[string]baseline.FileState{
		"gone.txt": {Size: goneInfo.Size(), ModTime: goneInfo.ModTime()},
		"new.txt":  {Size: 1, ModTime: goneInfo.ModTime()},
	}
	plan, err := Classify(base, local, remote, localRoot, remoteRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.RemoteDeletes) != 1 || plan.RemoteDeletes[0] != "gone.txt" {
		t.Errorf("gone.txt should be a RemoteDelete, plan=%+v", plan)
	}
	if len(plan.Pull) != 1 || plan.Pull[0] != "new.txt" {
		t.Errorf("new.txt should be a Pull, plan=%+v", plan)
	}
}

func hashOf(t *testing.T, p string) string {
	t.Helper()
	h, err := baseline.HashFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
