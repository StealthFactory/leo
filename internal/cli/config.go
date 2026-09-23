package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"leo/internal/config"
)

func newConfigCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "config",
		Short:   "Inspect and initialize leo configuration",
		GroupID: groupBuiltin,
		Args:    cobra.NoArgs,
	}

	var force bool
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Write a commented default config.toml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := config.Init(force)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
			return nil
		},
	}
	initCmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config")

	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Print effective resolved paths and command-path sets",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			w := cmd.OutOrStdout()
			fmt.Fprintf(w, "config:   %s\n", cfg.ConfigPath)
			fmt.Fprintf(w, "store:    %s\n", cfg.StorePath)
			fmt.Fprintf(w, "gen_lang: %s\n", cfg.GenLang)
			fmt.Fprintln(w, "command paths (in search order):")
			for i, cp := range cfg.CommandPaths {
				fmt.Fprintf(w, "  %d. [%s] %s\n", i+1, cp.Name, cp.Path)
			}
		},
	}

	pathCmd := &cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Args:  cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintln(cmd.OutOrStdout(), cfg.ConfigPath)
		},
	}

	cmd.AddCommand(initCmd, showCmd, pathCmd)
	return cmd
}
