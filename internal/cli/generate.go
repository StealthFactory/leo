package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"leo/internal/config"
	"leo/internal/scaffold"
)

func newGenerateCmd(cfg *config.Config) *cobra.Command {
	var workspaceName, lang string
	var force bool
	cmd := &cobra.Command{
		Use:     "generate <name>",
		Aliases: []string{"gen", "new"},
		Short:   "Scaffold a new external subcommand (leo-<name>) into a workspace",
		Long: "Scaffold a new external subcommand as an executable leo-<name> and drop it\n" +
			"into a workspace, so it runs as `leo <name>` right away and shows up\n" +
			"in `leo help`.\n\n" +
			"Languages: bash, zsh, python, node, typescript (aliases: sh, py, js, ts).\n" +
			"Without --lang it uses gen_lang from your config (default bash). Without\n" +
			"--workspace it writes into the first configured workspace.",
		Example: "  leo generate deploy\n" +
			"  leo generate deploy --lang python --workspace work\n" +
			"  leo gen hello --lang ts",
		GroupID: groupBuiltin,
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Bare `leo generate` shows help instead of a terse arg error, so it
			// behaves like the other subcommands when run without arguments.
			if len(args) == 0 {
				return cmd.Help()
			}
			name := args[0]

			// Choose the target workspace: --workspace by name, else the first configured workspace.
			target, err := chooseWorkspace(cfg, workspaceName)
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
			fmt.Fprintf(cmd.OutOrStdout(), "created %s  (workspace: %s)\nrun it with: leo %s\n", path, target.Name, name)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspaceName, "workspace", "", "workspace to write into (default: first configured workspace)")
	cmd.Flags().StringVar(&lang, "lang", "", "language: bash|zsh|python|node|typescript (aliases sh/py/js/ts)")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing subcommand")
	return cmd
}

// chooseWorkspace returns the workspace named name, or the first configured
// workspace when name is empty.
func chooseWorkspace(cfg *config.Config, name string) (config.Workspace, error) {
	if len(cfg.Workspaces) == 0 {
		return config.Workspace{}, fmt.Errorf("no workspaces configured; run `leo config setup`")
	}
	if name == "" {
		return cfg.Workspaces[0], nil
	}
	for _, cp := range cfg.Workspaces {
		if cp.Name == name {
			return cp, nil
		}
	}
	return config.Workspace{}, fmt.Errorf("no workspace named %q", name)
}
