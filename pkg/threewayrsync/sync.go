package threewayrsync

import (
	"context"
	"os"
)

// Syncer performs three-way syncs between a local and a remote endpoint using rsync,
// persisting the base manifest through a pluggable Store.
type Syncer struct {
	Store Store
	Bin   string // rsync binary; "" => "rsync"
	run   runner // unexported test seam; nil => execRun
}

// New returns a Syncer that shells out to rsync on PATH, using store for base state.
func New(store Store) *Syncer {
	return &Syncer{Store: store, Bin: "rsync", run: execRun}
}

func (s *Syncer) bin() string {
	if s.Bin == "" {
		return "rsync"
	}
	return s.Bin
}

func (s *Syncer) runner() runner {
	if s.run == nil {
		return execRun
	}
	return s.run
}

// list enumerates an endpoint into a Manifest via a dry-run itemize against an empty
// temporary directory: the empty dest makes rsync report the whole source tree.
func (s *Syncer) list(ctx context.Context, e Endpoint, exclude []string) (Manifest, error) {
	empty, err := os.MkdirTemp("", "threewayrsync-empty-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(empty) }()
	args := buildListArgs(e, empty, exclude)
	res, err := s.runner()(ctx, s.bin(), args, nil)
	if err != nil {
		return nil, &Error{Op: "list", Args: args, Stderr: res.stderr, ExitCode: res.exitCode, Err: err}
	}
	return parseListOutput(res.stdout)
}

// computePlan loads the base, enumerates both endpoints, and classifies. It returns the
// plan plus the base and the two live manifests (Sync needs them to derive the merged base).
func (s *Syncer) computePlan(ctx context.Context, local, remote Endpoint, opts Options) (Plan, Manifest, Manifest, Manifest, error) {
	if err := validate(local, remote); err != nil {
		return Plan{}, nil, nil, nil, err
	}
	base, _, err := s.Store.LoadBase()
	if err != nil {
		return Plan{}, nil, nil, nil, err
	}
	if base == nil {
		base = Manifest{}
	}
	localM, err := s.list(ctx, local, opts.Exclude)
	if err != nil {
		return Plan{}, nil, nil, nil, err
	}
	remoteM, err := s.list(ctx, remote, opts.Exclude)
	if err != nil {
		return Plan{}, nil, nil, nil, err
	}
	return Classify(base, localM, remoteM), base, localM, remoteM, nil
}

// Diff performs a dry run: it loads the base, enumerates both endpoints, and classifies
// every path into a Plan. It has no side effects.
func (s *Syncer) Diff(ctx context.Context, local, remote Endpoint, opts Options) (Plan, error) {
	plan, _, _, _, err := s.computePlan(ctx, local, remote, opts)
	return plan, err
}
