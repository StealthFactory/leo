package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"leo/internal/config"
	"leo/internal/scaffold"
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

	cmd.AddCommand(initCmd, showCmd, pathCmd, newConfigSetupCmd(cfg))
	return cmd
}

func newConfigSetupCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Interactively set config values (Enter keeps the shown value)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigSetup(cmd, cfg)
		},
	}
}

// runConfigSetup walks each config value, showing the current (or default)
// value in brackets; entering nothing keeps it. The result is written back to
// config.toml.
func runConfigSetup(cmd *cobra.Command, cfg *config.Config) error {
	fc, err := config.ReadFileConfig(cfg.ConfigPath)
	if err != nil {
		return err
	}

	store := firstNonEmpty(fc.StorePath, cfg.StorePath)
	genLang := firstNonEmpty(fc.GenLang, cfg.GenLang)
	sets := fc.Sets
	if len(sets) == 0 {
		sets = []config.CommandPath{{Name: "default", Path: filepath.Join(cfg.BaseDir, "commands")}}
	}

	r := bufio.NewReader(cmd.InOrStdin())
	w := cmd.OutOrStdout()

	store, _ = prompt(r, w, "store_path", store)

	// gen_lang is validated against the known languages; on a bad entry we
	// re-ask, but stop (fail closed) once input is exhausted.
	for {
		v, eof := prompt(r, w, "gen_lang", genLang)
		canon, err := scaffold.NormalizeLang(v)
		if err == nil {
			genLang = canon
			break
		}
		fmt.Fprintf(w, "  %v\n", err)
		if eof {
			return err
		}
	}

	for i := range sets {
		sets[i].Path, _ = prompt(r, w, fmt.Sprintf("command path [%s]", sets[i].Name), sets[i].Path)
	}

	if err := config.WriteConfig(cfg.ConfigPath, config.FileConfig{
		StorePath: store,
		GenLang:   genLang,
		Sets:      sets,
	}); err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote %s\n", cfg.ConfigPath)
	return nil
}

// prompt writes "label [def]: " and reads one line. It returns def when the
// user enters nothing, and reports whether input has ended (EOF) so callers can
// stop re-prompting.
func prompt(r *bufio.Reader, w io.Writer, label, def string) (string, bool) {
	fmt.Fprintf(w, "%s [%s]: ", label, def)
	line, err := r.ReadString('\n')
	eof := errors.Is(err, io.EOF)
	if s := strings.TrimSpace(line); s != "" {
		return s, eof
	}
	return def, eof
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
