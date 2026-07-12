package lifecycle

import (
	"context"
	"os"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
	"github.com/andresbott/netcheckout/internal/marker"
)

// Checkin reconciles the whole profile like Sync, then removes the marker and
// clears local state. --clean also removes the local working copy. A conflict
// stops before any release (the marker stays).
func (r Runner) Checkin(ctx context.Context, name string, p config.Profile, id ident.Ident, opts Options) (Report, error) {
	rep := Report{Action: "checkin", DryRun: opts.DryRun}
	_, _, err := r.reconcileProfile(ctx, name, p, id, "", opts, &rep)
	if err != nil {
		return rep, err
	}
	if opts.DryRun {
		return rep, nil
	}

	remoteRoot := config.ExpandRoot(p.RemoteRoot)
	if err := marker.Remove(remoteRoot); err != nil {
		return rep, err
	}
	if err := baseline.Remove(name); err != nil {
		return rep, err
	}
	rep.Released = true

	if opts.Clean {
		localRoot := config.ExpandRoot(p.LocalRoot)
		if err := os.RemoveAll(localRoot); err != nil {
			return rep, err
		}
	}
	return rep, nil
}
