package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/andresbott/netcheckout/internal/config"
	"github.com/andresbott/netcheckout/internal/rsync"
	"github.com/andresbott/netcheckout/internal/status"
	"github.com/spf13/cobra"
)

func newStatusCmd(cfgPath *string) *cobra.Command {
	return newStatusCmdWithDiffer(cfgPath, rsync.New())
}

func newStatusCmdWithDiffer(cfgPath *string, d status.Differ) *cobra.Command {
	return &cobra.Command{
		Use:   "status <profile>",
		Short: "show whether a profile's local folder is in sync with its remote",
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
			profile, ok := cfg.Profiles[name]
			if !ok {
				return fmt.Errorf("profile %q not found", name)
			}
			st, err := status.Compute(context.Background(), d, profile)
			if err != nil {
				return err
			}
			printStatus(cmd.OutOrStdout(), name, profile, st)
			return nil
		},
	}
}

func printStatus(w io.Writer, name string, p config.Profile, st status.ProfileStatus) {
	_, _ = fmt.Fprintf(w, "%s (local: %s, remote: %s)\n", name, p.LocalRoot, p.RemoteRoot)
	if st.InSync() {
		_, _ = fmt.Fprintln(w, "  in sync")
		return
	}
	for _, t := range st.Targets {
		_, _ = fmt.Fprintf(w, "  %s\n", t.Label())
		if t.LocalMissing {
			_, _ = fmt.Fprintf(w, "    not checked out locally -- pull (remote -> local) would create %d items\n", len(t.Pull.Changes))
			continue
		}
		printDirection(w, "push (local -> remote)", t.Push)
		printDirection(w, "pull (remote -> local)", t.Pull)
	}
}

func printDirection(w io.Writer, label string, d rsync.Diff) {
	if d.InSync {
		_, _ = fmt.Fprintf(w, "    %s: in sync\n", label)
		return
	}
	_, _ = fmt.Fprintf(w, "    %s: %d changes\n", label, len(d.Changes))
	for _, c := range d.Changes {
		_, _ = fmt.Fprintf(w, "      %s %s\n", changeMark(c.Type), c.Path)
	}
}

func changeMark(t rsync.ChangeType) string {
	switch t {
	case rsync.Created:
		return "+"
	case rsync.Deleted:
		return "-"
	default:
		return "M"
	}
}
