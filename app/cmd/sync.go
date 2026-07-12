package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/andresbott/netcheckout/app/metainfo"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
	"github.com/andresbott/netcheckout/internal/lifecycle"
	"github.com/andresbott/netcheckout/internal/reconcile"
	"github.com/andresbott/netcheckout/internal/rsync"
	"github.com/spf13/cobra"
)

func newSyncCmd(cfgPath *string) *cobra.Command {
	return newSyncCmdWithRunner(cfgPath, lifecycle.Runner{Syncer: rsync.New(), ToolVersion: metainfo.Version})
}

func newSyncCmdWithRunner(cfgPath *string, r lifecycle.Runner) *cobra.Command {
	var force, dryRun, verbose bool
	cmd := &cobra.Command{
		Use:   "sync <profile> [relpath]",
		Short: "reconcile a held checkout in place (push, pull, stop on conflicts)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := resolvePath(*cfgPath)
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			name := args[0]
			p, ok := cfg.Profiles[name]
			if !ok {
				return fmt.Errorf("profile %q not found", name)
			}
			id, err := ident.Resolve(cfg)
			if err != nil {
				return err
			}
			runner := r
			if verbose {
				if s, ok := runner.Syncer.(*rsync.Syncer); ok {
					s.Output = cmd.ErrOrStderr()
				}
			}
			rel := ""
			if len(args) == 2 {
				rel = args[1]
			}
			opts := lifecycle.Options{Force: force, DryRun: dryRun}
			if !dryRun {
				out := cmd.OutOrStdout()
				opts.OnApply = func(e reconcile.Event) { printApplyEvent(out, e) }
			}
			rep, err := runner.Sync(context.Background(), name, p, id, rel, opts)
			if err != nil {
				printReconcileReport(cmd.OutOrStdout(), name, rep) // show conflicts before the non-zero exit
				return err
			}
			printReconcileReport(cmd.OutOrStdout(), name, rep)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "resolve conflicts local-wins (never overrides the lock check)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the reconcile plan without changing anything")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "stream rsync output")
	return cmd
}

// printApplyEvent renders one applied change live, in the same "verb → side
// path" shape as the status view, as sync/checkin carry the reconcile out.
func printApplyEvent(w io.Writer, e reconcile.Event) {
	verb := "modify"
	switch e.Kind {
	case reconcile.EventAdd:
		verb = "add"
	case reconcile.EventDelete:
		verb = "delete"
	}
	side := "local"
	if e.Side == reconcile.SideRemote {
		side = "remote"
	}
	_, _ = fmt.Fprintf(w, "  %-8s → %-6s  %s\n", verb, side, e.Path)
}

// printReconcileReport is shared by sync and checkin.
func printReconcileReport(w io.Writer, name string, rep lifecycle.Report) {
	if len(rep.Conflicts) > 0 {
		_, _ = fmt.Fprintf(w, "%s: %d conflict(s) — nothing written:\n", name, len(rep.Conflicts))
		for _, c := range rep.Conflicts {
			_, _ = fmt.Fprintf(w, "  ! %s\n", c)
		}
		return
	}
	verb := "reconciled"
	if rep.Released {
		verb = "checked in"
	}
	if rep.DryRun {
		verb = "dry-run"
	}
	_, _ = fmt.Fprintf(w, "%s: %s (pull %d, push %d, del-remote %d, del-local %d)\n",
		name, verb, len(rep.Pulled), len(rep.Pushed), len(rep.RemovedRemote), len(rep.RemovedLocal))
}
