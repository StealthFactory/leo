// Package cli wires leo's cobra command tree: built-in commands plus external
// leo-* subcommands discovered across the configured command-path sets.
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"leo/internal/config"
)

const groupBuiltin = "builtin"

// Execute builds the root command and runs it, translating errors into a
// non-zero exit. version is injected at build time via -ldflags.
func Execute(version string) {
	root, err := NewRoot(version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "leo:", err)
		os.Exit(1)
	}
	if err := root.Execute(); err != nil {
		// SilenceErrors is set on the root, so print our own clean message
		// (no cobra "Error:" prefix, no usage dump) and exit non-zero.
		fmt.Fprintln(os.Stderr, "leo:", err)
		os.Exit(1)
	}
}

// NewRoot constructs the fully-wired root command.
func NewRoot(version string) (*cobra.Command, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	root := &cobra.Command{
		Use:           "leo",
		Short:         "a helpful assistant",
		Long:          "Leo, a helpful assistant.\n\nA tiny command-line sidekick with a JSON object store, a macOS clipboard\nhelper, and git-style external subcommands (leo-<name>) you can add without\nrecompiling. No plugins, no rebuilds.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// leo ships no shell completion (it can slow shell startup), so keep cobra
	// from auto-adding its own `completion` command.
	root.CompletionOptions.DisableDefaultCmd = true

	root.AddGroup(&cobra.Group{ID: groupBuiltin, Title: "Built-in commands:"})

	root.AddCommand(
		newStoreCmd(cfg),
		newClipCmd(cfg),
		newGenerateCmd(cfg),
		newConfigCmd(cfg),
		newVersionCmd(version),
	)

	// Discover and register external leo-* subcommands on the tree.
	registerExternals(root, cfg)

	return root, nil
}

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print the leo version",
		GroupID: groupBuiltin,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "leo %s\n", version)
		},
	}
}
