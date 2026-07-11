// Package reconcile implements the three-way merge (GOALS.md §9.5) that powers
// sync and checkin: it classifies each path against the checkout baseline into a
// Plan of pulls, pushes, deletes, and conflicts, and applies that plan via rsync
// and direct removals.
package reconcile

import "fmt"

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
