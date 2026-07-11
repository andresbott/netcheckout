package reconcile

import "testing"

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
