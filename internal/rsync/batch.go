package rsync

import (
	"context"
	"fmt"
)

// DiffAll runs Diff for each job in order, stopping at the first error and
// returning the diffs gathered so far plus an error naming the failing job.
func (s *Syncer) DiffAll(ctx context.Context, jobs []Job) ([]Diff, error) {
	out := make([]Diff, 0, len(jobs))
	for i, j := range jobs {
		d, err := s.Diff(ctx, j)
		if err != nil {
			return out, fmt.Errorf("job %d: %w", i, err)
		}
		out = append(out, d)
	}
	return out, nil
}

// SyncAll runs Sync for each job in order, stopping at the first error and
// returning the results gathered so far plus an error naming the failing job.
func (s *Syncer) SyncAll(ctx context.Context, jobs []Job) ([]Result, error) {
	out := make([]Result, 0, len(jobs))
	for i, j := range jobs {
		r, err := s.Sync(ctx, j)
		if err != nil {
			return out, fmt.Errorf("job %d: %w", i, err)
		}
		out = append(out, r)
	}
	return out, nil
}
