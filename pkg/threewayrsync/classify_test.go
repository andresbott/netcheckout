package threewayrsync

import (
	"testing"
	"time"
)

func TestClassifyPathTable(t *testing.T) {
	type side struct{ present, changed bool }
	cases := []struct {
		name      string
		inBase    bool
		local     side
		remote    side
		converged bool
		want      action
	}{
		{"local edit, remote unchanged", true, side{true, true}, side{true, false}, false, actPush},
		{"remote edit, local unchanged", true, side{true, false}, side{true, true}, false, actPull},
		{"both edited (diverged)", true, side{true, true}, side{true, true}, false, actConflict},
		{"both edited identically (converged)", true, side{true, true}, side{true, true}, true, actNoop},
		{"remote addition", false, side{false, false}, side{true, false}, false, actPull},
		{"local addition", false, side{true, false}, side{false, false}, false, actPush},
		{"both added same path (diverged)", false, side{true, false}, side{true, false}, false, actConflict},
		{"both added identical (converged)", false, side{true, false}, side{true, false}, true, actNoop},
		{"local delete (remote unchanged)", true, side{false, false}, side{true, false}, false, actRemoteDelete},
		{"remote delete (local unchanged)", true, side{true, false}, side{false, false}, false, actLocalDelete},
		{"both unchanged", true, side{true, false}, side{true, false}, false, actNoop},
		{"both deleted", true, side{false, false}, side{false, false}, false, actNoop},
		{"local delete vs remote edit", true, side{false, false}, side{true, true}, false, actConflict},
		{"remote delete vs local edit", true, side{true, true}, side{false, false}, false, actConflict},
		{"not in base, neither present", false, side{false, false}, side{false, false}, false, actNoop},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := classifyPath(c.inBase, c.local.present, c.local.changed, c.remote.present, c.remote.changed, c.converged)
			if got != c.want {
				t.Errorf("classifyPath = %v, want %v", got, c.want)
			}
		})
	}
}

func TestClassifyConvergedBothSidesEqualIsNoop(t *testing.T) {
	base := Manifest{"x": {Size: 1, ModTime: time.Unix(100, 0)}}
	// Both sides changed to the SAME new state => converged, not a conflict.
	local := Manifest{"x": {Size: 2, ModTime: time.Unix(200, 0)}}
	remote := Manifest{"x": {Size: 2, ModTime: time.Unix(200, 0)}}
	plan := Classify(base, local, remote)
	if len(plan.Conflicts) != 0 {
		t.Errorf("converged change must not conflict: %+v", plan)
	}
	if !plan.InSync {
		t.Errorf("converged change must be in sync: %+v", plan)
	}
}

func TestClassifyBucketsPushPullDelete(t *testing.T) {
	t0 := time.Unix(100, 0)
	t1 := time.Unix(200, 0)
	base := Manifest{
		"push.txt": {Size: 1, ModTime: t0},
		"pull.txt": {Size: 1, ModTime: t0},
		"del.txt":  {Size: 1, ModTime: t0},
	}
	local := Manifest{
		"push.txt": {Size: 2, ModTime: t1}, // locally edited
		"pull.txt": {Size: 1, ModTime: t0}, // unchanged
		// del.txt removed locally; remote unchanged => RemoteDelete
	}
	remote := Manifest{
		"push.txt": {Size: 1, ModTime: t0}, // unchanged
		"pull.txt": {Size: 2, ModTime: t1}, // remotely edited
		"del.txt":  {Size: 1, ModTime: t0}, // unchanged
	}
	plan := Classify(base, local, remote)
	if len(plan.Push) != 1 || plan.Push[0] != "push.txt" {
		t.Errorf("Push = %v", plan.Push)
	}
	if len(plan.Pull) != 1 || plan.Pull[0] != "pull.txt" {
		t.Errorf("Pull = %v", plan.Pull)
	}
	if len(plan.RemoteDeletes) != 1 || plan.RemoteDeletes[0] != "del.txt" {
		t.Errorf("RemoteDeletes = %v", plan.RemoteDeletes)
	}
	if plan.InSync {
		t.Errorf("plan should not be in sync: %+v", plan)
	}
}
