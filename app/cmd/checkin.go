package cmd

import (
	"context"
	"fmt"

	"github.com/andresbott/netcheckout/app/metainfo"
	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/ident"
	"github.com/andresbott/netcheckout/internal/lifecycle"
	"github.com/andresbott/netcheckout/internal/rsync"
	"github.com/spf13/cobra"
)

func newCheckinCmd(cfgPath *string) *cobra.Command {
	return newCheckinCmdWithRunner(cfgPath, lifecycle.Runner{Syncer: rsync.New(), ToolVersion: metainfo.Version})
}

func newCheckinCmdWithRunner(cfgPath *string, r lifecycle.Runner) *cobra.Command {
	var force, dryRun, clean, verbose bool
	cmd := &cobra.Command{
		Use:   "checkin <profile>",
		Short: "reconcile the whole profile, then remove the lock",
		Args:  cobra.ExactArgs(1),
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
			rep, err := runner.Checkin(context.Background(), name, p, id, lifecycle.Options{Force: force, DryRun: dryRun, Clean: clean})
			printReconcileReport(cmd.OutOrStdout(), name, rep)
			return err
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "resolve conflicts local-wins (never overrides the lock check)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the reconcile plan without changing anything")
	cmd.Flags().BoolVar(&clean, "clean", false, "remove the local working copy after a successful release")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "stream rsync output")
	return cmd
}
