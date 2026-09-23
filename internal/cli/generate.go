package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"leo/internal/config"
	"leo/internal/scaffold"
)

func newGenerateCmd(cfg *config.Config) *cobra.Command {
	var setName, lang string
	var force bool
	cmd := &cobra.Command{
		Use:     "generate <name>",
		Aliases: []string{"gen", "new"},
		Short:   "Scaffold a new external subcommand (leo-<name>) into a command-path set",
		GroupID: groupBuiltin,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]

			// Choose the target set: --set by name, else the first configured set.
			target, err := chooseSet(cfg, setName)
			if err != nil {
				return err
			}

			language := lang
			if language == "" {
				language = cfg.GenLang
			}

			path, err := scaffold.Generate(target.Path, name, language, force)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created %s  (set: %s)\nrun it with: leo %s\n", path, target.Name, name)
			return nil
		},
	}
	cmd.Flags().StringVar(&setName, "set", "", "command-path set to write into (default: first configured set)")
	cmd.Flags().StringVar(&lang, "lang", "", "language: bash|zsh|python|node|typescript (aliases sh/py/js/ts)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing subcommand")
	_ = cmd.RegisterFlagCompletionFunc("lang", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return scaffold.Languages(), cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("set", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		names := make([]string, 0, len(cfg.CommandPaths))
		for _, cp := range cfg.CommandPaths {
			names = append(names, cp.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

// chooseSet returns the command-path set named name, or the first configured
// set when name is empty.
func chooseSet(cfg *config.Config, name string) (config.CommandPath, error) {
	if len(cfg.CommandPaths) == 0 {
		return config.CommandPath{}, fmt.Errorf("no command-path sets configured; run `leo config init`")
	}
	if name == "" {
		return cfg.CommandPaths[0], nil
	}
	for _, cp := range cfg.CommandPaths {
		if cp.Name == name {
			return cp, nil
		}
	}
	return config.CommandPath{}, fmt.Errorf("no command-path set named %q", name)
}
