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

func newCheckoutCmd(cfgPath *string) *cobra.Command {
	return newCheckoutCmdWithRunner(cfgPath, lifecycle.Runner{Syncer: rsync.New(), ToolVersion: metainfo.Version})
}

func newCheckoutCmdWithRunner(cfgPath *string, r lifecycle.Runner) *cobra.Command {
	var force, dryRun, verbose bool
	cmd := &cobra.Command{
		Use:   "checkout <profile> [relpath]",
		Short: "pull a profile's remote folder to local and lock it",
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
			rep, err := runner.Checkout(context.Background(), name, p, id, rel, opts)
			if err != nil {
				return err
			}
			printCheckoutReport(cmd.OutOrStdout(), name, rep)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "override an existing lock held by someone else")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show the plan without transferring or writing a marker")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "stream rsync output")
	return cmd
}

func printCheckoutReport(w io.Writer, name string, rep lifecycle.Report) {
	if rep.DryRun {
		_, _ = fmt.Fprintf(w, "%s: dry-run — would pull %d items and write a marker\n", name, len(rep.Pulled))
		for _, p := range rep.Pulled {
			_, _ = fmt.Fprintf(w, "  + %s\n", p)
		}
		return
	}
	_, _ = fmt.Fprintf(w, "%s: checked out (%d items pulled)\n", name, len(rep.Pulled))
}
