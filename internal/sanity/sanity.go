// Package sanity computes lightweight, stat-only state for a profile: whether
// its roots exist, whether it is checked out, and whether its declared subpaths
// exist on the remote. It performs no rsync and no content comparison -- that is
// internal/status.
package sanity

import (
	"os"
	"path/filepath"

	"github.com/andresbott/netcheckout/internal/config"
)

// markerFile is the checkout marker placed inside a checked-out remote folder.
// Matches GOALS.md §5 (name still "proposed").
const markerFile = ".netcheckout.json"

// Result is the lightweight, stat-only state of a profile.
type Result struct {
	LocalRoot  bool      // local_root exists on disk
	RemoteRoot bool      // remote_root exists and is a directory (mounted)
	CheckedOut bool      // a marker is present at any target (aggregate)
	Subpaths   []Subpath // one per declared subpath, in config order; empty for whole-root profiles
}

// Subpath is the existence of one declared subpath under the remote root.
type Subpath struct {
	Path   string
	Exists bool
}

// Check runs the stat-only checks for a profile. It returns no error: a missing
// path or unmounted remote is data (a false field), not a failure.
func Check(p config.Profile) Result {
	var r Result
	if _, err := os.Stat(config.ExpandRoot(p.LocalRoot)); err == nil {
		r.LocalRoot = true
	}
	if info, err := os.Stat(config.ExpandRoot(p.RemoteRoot)); err == nil && info.IsDir() {
		r.RemoteRoot = true
	}

	targets, err := p.Targets()
	if err != nil {
		return r // an invalid declared subpath; the roots are still meaningful
	}
	for _, t := range targets {
		if _, err := os.Stat(filepath.Join(t.Remote, markerFile)); err == nil {
			r.CheckedOut = true
		}
		if t.Subpath != "" {
			_, err := os.Stat(t.Remote)
			r.Subpaths = append(r.Subpaths, Subpath{Path: t.Subpath, Exists: err == nil})
		}
	}
	return r
}
