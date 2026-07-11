// Package sanity computes lightweight, stat-only state for a profile: whether
// its roots exist, whether it is checked out, and whether its declared subpaths
// exist on the remote. It performs no rsync and no content comparison -- that is
// internal/status.
package sanity

import (
	"os"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/marker"
)

// Result is the lightweight, stat-only state of a profile.
type Result struct {
	LocalRoot  bool      // local_root exists on disk
	RemoteRoot bool      // remote_root exists and is a directory (mounted)
	CheckedOut bool      // a marker is present at the remote root
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
	remoteRoot := config.ExpandRoot(p.RemoteRoot)
	if info, err := os.Stat(remoteRoot); err == nil && info.IsDir() {
		r.RemoteRoot = true
	}
	// The marker is per-profile at the remote root (GOALS.md §5).
	if _, err := os.Stat(marker.Path(remoteRoot)); err == nil {
		r.CheckedOut = true
	}

	targets, err := p.Targets()
	if err != nil {
		return r
	}
	for _, t := range targets {
		if t.Subpath != "" {
			_, err := os.Stat(t.Remote)
			r.Subpaths = append(r.Subpaths, Subpath{Path: t.Subpath, Exists: err == nil})
		}
	}
	return r
}
