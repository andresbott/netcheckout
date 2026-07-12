// Package status previews whether a profile's local and remote roots are in
// sync. It is a read-only dry-run of sync: it loads the checkout baseline and
// runs the same three-way reconcile engine (internal/reconcile) that sync uses
// to apply changes, so what status reports and what sync does can never diverge.
// The preview is grouped per configured subpath (target), or a single "(root)"
// group when the profile has no subpaths.
package status

import (
	"fmt"
	"os"

	"github.com/andresbott/netcheckout/internal/baseline"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/marker"
	"github.com/andresbott/netcheckout/internal/reconcile"
)

// Change is one planned copy: a path and whether it modifies an existing file
// (in the baseline) or adds a new one.
type Change struct {
	Path   string
	Modify bool // true = an existing file changed; false = a newly added file
}

// TargetStatus is the three-way reconcile plan for one target of a profile — the
// whole root when no subpaths are declared, or one declared subpath — split into
// the buckets a sync uses.
type TargetStatus struct {
	Subpath       string
	Push          []Change // local -> remote (add or modify)
	Pull          []Change // remote -> local (add or modify)
	LocalDeletes  []string // mirror a remote delete by removing the local file
	RemoteDeletes []string // propagate a local delete by removing the remote file
	Conflicts     []string // changed on both sides; a sync would stop
}

// InSync reports whether this target has no pending changes in any bucket.
func (t TargetStatus) InSync() bool {
	return len(t.Push)+len(t.Pull)+len(t.LocalDeletes)+len(t.RemoteDeletes)+len(t.Conflicts) == 0
}

// Label is a human-readable name for this target: "(root)" for the whole root,
// or the declared subpath.
func (t TargetStatus) Label() string {
	return label(t.Subpath)
}

func label(subpath string) string {
	if subpath == "" {
		return "(root)"
	}
	return subpath
}

// ProfileStatus is the reconcile preview across every target of a profile. When
// CheckedOut is false the profile has no marker; when CheckedOut is true but
// HasBaseline is false the profile is checked out but this machine holds no local
// baseline, so no plan could be computed and Targets is empty.
type ProfileStatus struct {
	CheckedOut  bool
	HasBaseline bool
	Targets     []TargetStatus
}

// InSync reports whether every target of the profile is in sync.
func (s ProfileStatus) InSync() bool {
	for _, t := range s.Targets {
		if !t.InSync() {
			return false
		}
	}
	return true
}

// Compute loads the profile's baseline and previews the three-way reconcile plan,
// grouped per target, against the current local and remote trees. It errors only
// for a real failure (the remote root missing, an invalid subpath, or a scan/hash
// failure); pending changes are a normal result captured in the returned
// ProfileStatus. name is the profile name (the baseline's state-file key).
func Compute(name string, p config.Profile) (ProfileStatus, error) {
	remoteRoot := config.ExpandRoot(p.RemoteRoot)
	localRoot := config.ExpandRoot(p.LocalRoot)
	if info, err := os.Stat(remoteRoot); err != nil || !info.IsDir() {
		return ProfileStatus{}, fmt.Errorf("remote root %s is not mounted", remoteRoot)
	}

	// No marker means the profile is not checked out: nothing to preview.
	_, exists, err := marker.Read(remoteRoot)
	if err != nil {
		return ProfileStatus{}, err
	}
	if !exists {
		return ProfileStatus{CheckedOut: false}, nil
	}

	// A marker without a local baseline means the checkout is held elsewhere (or
	// the baseline was lost): report checked out, but no plan can be computed.
	b, hasBase, err := baseline.Load(name)
	if err != nil {
		return ProfileStatus{}, err
	}
	if !hasBase {
		return ProfileStatus{CheckedOut: true, HasBaseline: false}, nil
	}

	targets, err := p.Targets()
	if err != nil {
		return ProfileStatus{}, err
	}

	out := ProfileStatus{CheckedOut: true, HasBaseline: true, Targets: make([]TargetStatus, 0, len(targets))}
	for _, t := range targets {
		rel := t.Subpath
		if rel == "" {
			rel = "."
		}
		// The shared engine scans only this target's subtree on both sides;
		// baseline entries outside it fall out as no-ops, so passing the whole
		// baseline manifest is safe and needs no pre-filtering.
		plan, err := reconcile.PlanFor(b.Files, localRoot, remoteRoot, []string{rel})
		if err != nil {
			return ProfileStatus{}, fmt.Errorf("%s: %w", label(t.Subpath), err)
		}
		out.Targets = append(out.Targets, targetFromPlan(t.Subpath, plan, b.Files))
	}
	return out, nil
}

// targetFromPlan projects a reconcile.Plan into a TargetStatus, labelling each
// pushed or pulled path as a modify (already in the baseline) or an add (not yet).
func targetFromPlan(subpath string, plan reconcile.Plan, base map[string]baseline.FileState) TargetStatus {
	changes := func(paths []string) []Change {
		if len(paths) == 0 {
			return nil
		}
		out := make([]Change, 0, len(paths))
		for _, p := range paths {
			_, inBase := base[p]
			out = append(out, Change{Path: p, Modify: inBase})
		}
		return out
	}
	return TargetStatus{
		Subpath:       subpath,
		Push:          changes(plan.Push),
		Pull:          changes(plan.Pull),
		LocalDeletes:  plan.LocalDeletes,
		RemoteDeletes: plan.RemoteDeletes,
		Conflicts:     plan.Conflicts,
	}
}
