// Package reconcile implements the three-way merge (GOALS.md §9.5) that powers
// sync and checkin: it classifies each path against the checkout baseline into a
// Plan of pulls, pushes, deletes, and conflicts, and applies that plan via rsync
// and direct removals.
package reconcile

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/andresbott/netcheckout/internal/baseline"
)

type action int

const (
	actNoop action = iota
	actPush
	actPull
	actRemoteDelete
	actLocalDelete
	actConflict
)

// Plan is the set of operations a clean reconcile will perform.
type Plan struct {
	Pull          []string
	Push          []string
	RemoteDeletes []string
	LocalDeletes  []string
	Conflicts     []string
}

// ConflictError reports that the same paths changed on both sides.
type ConflictError struct{ Paths []string }

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%d conflicting path(s) changed on both sides", len(e.Paths))
}

// classifyPath encodes the GOALS.md §9.5 table. "changed" is only meaningful
// when the path is in the baseline and present on that side.
func classifyPath(inBase, localPresent, localChanged, remotePresent, remoteChanged bool) action {
	if !inBase {
		switch {
		case localPresent && remotePresent:
			return actConflict // both independently added the same path
		case remotePresent:
			return actPull // remote addition
		case localPresent:
			return actPush // local addition
		default:
			return actNoop
		}
	}
	// In baseline.
	switch {
	case !localPresent && !remotePresent:
		return actNoop // deleted on both sides
	case !localPresent && remotePresent:
		if remoteChanged {
			return actConflict // local delete vs remote edit
		}
		return actRemoteDelete // propagate local delete
	case localPresent && !remotePresent:
		if localChanged {
			return actConflict // remote delete vs local edit
		}
		return actLocalDelete // mirror remote delete
	default: // present both sides
		switch {
		case localChanged && remoteChanged:
			return actConflict
		case localChanged:
			return actPush
		case remoteChanged:
			return actPull
		default:
			return actNoop
		}
	}
}

// sortedPlan sorts each slice in the plan so output is deterministic.
func sortedPlan(p *Plan) {
	sort.Strings(p.Pull)
	sort.Strings(p.Push)
	sort.Strings(p.RemoteDeletes)
	sort.Strings(p.LocalDeletes)
	sort.Strings(p.Conflicts)
}

// Classify compares the current local and remote manifests against the baseline
// and buckets every path into a Plan per the §9.5 table. localRoot/remoteRoot
// are needed to hash a file when the fast path is inconclusive.
func Classify(base, local, remote map[string]baseline.FileState, localRoot, remoteRoot string) (Plan, error) {
	seen := map[string]struct{}{}
	for p := range base {
		seen[p] = struct{}{}
	}
	for p := range local {
		seen[p] = struct{}{}
	}
	for p := range remote {
		seen[p] = struct{}{}
	}

	var plan Plan
	for p := range seen {
		bState, inBase := base[p]
		lState, lPresent := local[p]
		rState, rPresent := remote[p]

		lChanged, rChanged := false, false
		var err error
		if inBase && lPresent {
			if lChanged, err = baseline.Changed(bState, lState, filepath.Join(localRoot, filepath.FromSlash(p))); err != nil {
				return Plan{}, err
			}
		}
		if inBase && rPresent {
			if rChanged, err = baseline.Changed(bState, rState, filepath.Join(remoteRoot, filepath.FromSlash(p))); err != nil {
				return Plan{}, err
			}
		}

		switch classifyPath(inBase, lPresent, lChanged, rPresent, rChanged) {
		case actPush:
			plan.Push = append(plan.Push, p)
		case actPull:
			plan.Pull = append(plan.Pull, p)
		case actRemoteDelete:
			plan.RemoteDeletes = append(plan.RemoteDeletes, p)
		case actLocalDelete:
			plan.LocalDeletes = append(plan.LocalDeletes, p)
		case actConflict:
			plan.Conflicts = append(plan.Conflicts, p)
		case actNoop:
		}
	}
	sortedPlan(&plan)
	return plan, nil
}
