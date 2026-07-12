// Package lifecycle orchestrates the mutating checkout actions (checkout in M3;
// sync and checkin in M4), driving rsync, the marker, and the local baseline.
// It is the single seam the CLI and the TUI both call.
package lifecycle

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/andresbott/netcheckout/internal/marker"
	"github.com/andresbott/netcheckout/internal/reconcile"
	"github.com/andresbott/netcheckout/internal/rsync"
)

// Options are the shared flags every mutating action understands.
type Options struct {
	Force  bool
	DryRun bool
	Clean  bool // checkin only
	// OnApply, when non-nil, is called once per applied change as Sync/Checkin
	// carry the reconcile out, giving callers live per-file progress. It runs on
	// the goroutine driving the action and is never called on a dry run.
	OnApply func(reconcile.Event)
}

// Report describes what an action did (or would do, for a dry run).
type Report struct {
	Action        string
	Pulled        []string
	Pushed        []string
	RemovedRemote []string
	RemovedLocal  []string
	Conflicts     []string
	Marker        *marker.Marker
	DryRun        bool
	Released      bool
}

// Syncer is the rsync surface lifecycle needs; *rsync.Syncer satisfies it.
type Syncer interface {
	Sync(ctx context.Context, j rsync.Job) (rsync.Result, error)
	Diff(ctx context.Context, j rsync.Job) (rsync.Diff, error)
}

// Runner carries the injectable dependencies for the actions.
type Runner struct {
	Syncer      Syncer
	ToolVersion string
	Now         func() time.Time
}

func (r Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now().UTC()
}

// normalizeRelpath maps "" or "." to "." (whole root) and cleans anything else,
// stripping a leading "./".
func normalizeRelpath(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" || rel == "." {
		return "."
	}
	return filepath.ToSlash(filepath.Clean(rel))
}
