package lifecycle

import (
	"context"
	"fmt"
	"os"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
	"github.com/andresbott/netcheckout/internal/marker"
)

// Checkin releases the whole profile. It verifies — via the same three-way
// reconcile engine sync uses (classifyProfile) — that local and remote are
// already in sync, then removes the marker and clears local state. It copies
// nothing: moving data is sync's job. If anything is still pending (a pull,
// push, delete, or conflict) checkin refuses and leaves the marker in place,
// pointing the user at sync. There is no --force — releasing requires a
// this-machine-owned, fully-synced profile. --clean additionally removes the
// local working copy after a successful release.
func (r Runner) Checkin(ctx context.Context, name string, p config.Profile, id ident.Ident, opts Options) (Report, error) {
	rep := Report{Action: "checkin", DryRun: opts.DryRun}
	pf, err := r.classifyProfile(name, p, id, "", opts, &rep)
	if err != nil {
		return rep, err
	}

	// Surface the pending plan so both a dry-run preview and the "unsynced"
	// failure path show exactly what is blocking the release.
	plan := pf.plan
	rep.Pulled = plan.Pull
	rep.Pushed = plan.Push
	rep.RemovedRemote = plan.RemoteDeletes
	rep.RemovedLocal = plan.LocalDeletes
	rep.Conflicts = plan.Conflicts

	if opts.DryRun {
		return rep, nil
	}
	if !plan.Empty() {
		return rep, fmt.Errorf("profile %q has unsynced changes — run 'netcheckout sync %s' before checking in", name, name)
	}

	if err := marker.Remove(pf.remoteRoot); err != nil {
		return rep, err
	}
	if err := baseline.Remove(name); err != nil {
		return rep, err
	}
	rep.Released = true

	if opts.Clean {
		if err := os.RemoveAll(pf.localRoot); err != nil {
			return rep, err
		}
	}
	return rep, nil
}
