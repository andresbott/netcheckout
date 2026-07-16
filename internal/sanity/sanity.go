// Package sanity computes lightweight, stat-only state for a profile: whether
// its roots exist, whether it is checked out, and whether its declared subpaths
// exist on the remote. It performs no rsync and no content comparison -- that is
// internal/status. UnlistedLocal additionally performs a bounded local-only
// directory walk.
package sanity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/marker"
)

// Result is the lightweight state of a profile.
type Result struct {
	LocalRoot     bool      // local_root exists on disk
	RemoteRoot    bool      // remote_root exists and is a directory (mounted)
	CheckedOut    bool      // a marker is present at the remote root
	Subpaths      []Subpath // one per declared subpath, in config order; empty for whole-root profiles
	UnlistedLocal []string  // local paths (relative to local_root) holding content outside all declared subpaths
}

// Subpath is the existence of one declared subpath under the remote root.
type Subpath struct {
	Path   string
	Exists bool
}

// Check runs the lightweight checks for a profile. It returns no error: a
// missing path or unreachable remote is data (a false field), not a failure.
// A mounted-path remote is stat-only; a URL remote (ssh:// or rsync://) probes
// reachability and checkout state with a single marker fetch over rsync, and
// omits the per-subpath existence rows (no cheap remote stat exists).
func Check(p config.Profile) Result {
	var r Result
	if _, err := os.Stat(config.ExpandRoot(p.LocalRoot)); err == nil {
		r.LocalRoot = true
	}

	if p.RemoteIsLocalPath() {
		checkMountedRemote(p, &r)
	} else if e, err := p.RemoteEndpoint(); err == nil {
		if _, found, err := marker.ForEndpoint(e).Read(context.Background()); err == nil {
			// The fetch reached the endpoint (found or cleanly absent).
			r.RemoteRoot = true
			r.CheckedOut = found
		}
	}

	// Best-effort: a walk error is swallowed (Check reports data, not failures).
	if unlisted, err := UnlistedLocal(p); err == nil {
		r.UnlistedLocal = unlisted
	}
	return r
}

// checkMountedRemote fills the remote-side fields for a mounted-path remote:
// root presence, marker presence, and per-subpath existence — all stat-only.
func checkMountedRemote(p config.Profile, r *Result) {
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
		return
	}
	for _, t := range targets {
		if t.Subpath != "" {
			_, err := os.Stat(t.Remote)
			r.Subpaths = append(r.Subpaths, Subpath{Path: t.Subpath, Exists: err == nil})
		}
	}
}

// UnlistedLocal walks local_root and returns the shallowest paths (relative to
// local_root, slash-separated) that hold at least one regular file but fall outside
// every declared subpath. Loose regular files directly under local_root are reported
// individually. It returns nil when the profile declares no subpaths (whole root => all
// content is in scope) or when local_root does not exist. Symlinks are not regular files
// and never, by themselves, make a directory "contain files".
func UnlistedLocal(p config.Profile) ([]string, error) {
	if len(p.Subpaths) == 0 {
		return nil, nil
	}
	subs := make([]string, 0, len(p.Subpaths))
	for _, s := range p.Subpaths {
		if err := config.ValidateSubpath(s); err != nil {
			return nil, fmt.Errorf("subpath %q: %w", s, err)
		}
		subs = append(subs, filepath.ToSlash(filepath.Clean(s)))
	}
	localRoot := config.ExpandRoot(p.LocalRoot)
	if info, err := os.Stat(localRoot); err != nil || !info.IsDir() {
		return nil, nil
	}
	var flagged []string
	if err := walkUnlisted(localRoot, ".", subs, &flagged); err != nil {
		return nil, err
	}
	sort.Strings(flagged)
	if len(flagged) == 0 {
		return nil, nil
	}
	return flagged, nil
}

// walkUnlisted recurses under localRoot at relative rel, appending uncovered entries.
func walkUnlisted(localRoot, rel string, subs []string, out *[]string) error {
	dir := localRoot
	if rel != "." {
		dir = filepath.Join(localRoot, filepath.FromSlash(rel))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		childRel := e.Name()
		if rel != "." {
			childRel = rel + "/" + e.Name()
		}
		if e.IsDir() {
			switch {
			case isCovered(childRel, subs):
				// fully in scope; skip.
			case isAncestorOfSubpath(childRel, subs):
				if err := walkUnlisted(localRoot, childRel, subs, out); err != nil {
					return err
				}
			default:
				hasFile, err := containsRegularFile(filepath.Join(localRoot, filepath.FromSlash(childRel)))
				if err != nil {
					return err
				}
				if hasFile {
					*out = append(*out, childRel)
				}
			}
			continue
		}
		// regular file (skip symlinks and other non-regular entries)
		if e.Type().IsRegular() && !isCovered(childRel, subs) {
			*out = append(*out, childRel)
		}
	}
	return nil
}

// isCovered reports whether rel equals or lives under any declared subpath.
func isCovered(rel string, subs []string) bool {
	for _, s := range subs {
		if rel == s || strings.HasPrefix(rel, s+"/") {
			return true
		}
	}
	return false
}

// isAncestorOfSubpath reports whether any declared subpath lives under rel.
func isAncestorOfSubpath(rel string, subs []string) bool {
	for _, s := range subs {
		if strings.HasPrefix(s, rel+"/") {
			return true
		}
	}
	return false
}

// containsRegularFile reports whether dir holds at least one regular file at any depth.
func containsRegularFile(dir string) (bool, error) {
	found := false
	err := filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found, err
}
