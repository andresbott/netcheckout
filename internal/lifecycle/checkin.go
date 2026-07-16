package lifecycle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
)

// validateCleanTarget guards checkin --clean's os.RemoveAll: it refuses the
// filesystem root, anything at depth <= 2 (/home, /home/user, /etc, ...), and
// the user's home directory itself. ValidateRoot only requires an absolute
// path, so a config typo could otherwise point --clean at a catastrophic
// target. A real working copy lives at least one level below home or three
// below /.
func validateCleanTarget(root string) error {
	clean := filepath.Clean(root)
	if !filepath.IsAbs(clean) {
		return fmt.Errorf("refusing --clean: local root %q is not an absolute path", root)
	}
	if home, err := os.UserHomeDir(); err == nil && clean == filepath.Clean(home) {
		return fmt.Errorf("refusing --clean: local root %q is your home directory", root)
	}
	// Depth = number of path elements below /: "/" is 0, "/home" 1, "/home/user" 2.
	depth := len(strings.Split(strings.Trim(clean, string(filepath.Separator)), string(filepath.Separator)))
	if clean == string(filepath.Separator) || depth <= 2 {
		return fmt.Errorf("refusing --clean: local root %q is too shallow to be a working copy", root)
	}
	return nil
}

// Checkin releases the whole profile. It verifies — via the same three-way
// engine sync uses (a read-only Diff) — that local and remote are already in
// sync, then removes the marker and clears local state. It copies nothing:
// moving data is sync's job. If anything is still pending (a pull, push, delete,
// or conflict) checkin refuses and leaves the marker in place, pointing the user
// at sync. There is no --force — releasing requires a this-machine-owned,
// fully-synced profile. --clean additionally removes the local working copy
// after a successful release.
func (r Runner) Checkin(ctx context.Context, name string, p config.Profile, id ident.Ident, opts Options) (Report, error) {
	rep := Report{Action: "checkin", DryRun: opts.DryRun}
	// Validate the --clean target up front: a refusal here must leave the
	// checkout fully intact (nothing released, nothing removed).
	if opts.Clean {
		if err := validateCleanTarget(config.ExpandRoot(p.LocalRoot)); err != nil {
			return rep, err
		}
	}
	pf, err := r.preflightProfile(ctx, name, p, id, "", "checkin")
	if err != nil {
		return rep, err
	}
	syncer := r.syncer(baseline.Store(name))
	plan, err := syncer.Diff(ctx, pf.local, pf.remote, engineOptions(pf, Options{}, pf.state.Files))
	if err != nil {
		return rep, userFacingEngineErr(err)
	}

	// Surface the pending plan so both a dry-run preview and the "unsynced"
	// failure path show exactly what is blocking the release.
	fillPlan(&rep, plan)

	if opts.DryRun {
		return rep, nil
	}
	if !plan.InSync {
		return rep, fmt.Errorf("profile %q has unsynced changes — run 'netcheckout sync %s' before checking in", name, name)
	}

	if err := pf.acc.Remove(ctx); err != nil {
		return rep, err
	}
	if err := baseline.Remove(name); err != nil {
		return rep, err
	}
	rep.Released = true

	if opts.Clean {
		if err := os.RemoveAll(config.ExpandRoot(p.LocalRoot)); err != nil {
			return rep, err
		}
	}
	return rep, nil
}
