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
		Short:   "Inspect and set up leo configuration",
		GroupID: groupBuiltin,
		Args:    cobra.NoArgs,
	}

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

	cmd.AddCommand(showCmd, pathCmd, newConfigSetupCmd(cfg))
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
	// re-ask, but stop (fail closed) once input is exhausted. The options are
	// shown in the prompt so the user knows the valid choices.
	langLabel := fmt.Sprintf("gen_lang [default language for leo generate] (%s)", strings.Join(scaffold.Languages(), "|"))
	for {
		v, eof := prompt(r, w, langLabel, genLang)
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

	// Offer to append new named sets (e.g. "work") until the user declines or
	// input runs out.
	for {
		ans, eof := prompt(r, w, "add another command-path set? [y/N]", "")
		if !isYes(ans) {
			break
		}
		name, eofN := prompt(r, w, "  set name", "")
		def := ""
		if name != "" {
			def = filepath.Join(cfg.BaseDir, name+"-commands")
		}
		path, eofP := prompt(r, w, "  set path", def)
		if name == "" || path == "" {
			fmt.Fprintln(w, "  skipped: a set needs both a name and a path")
		} else {
			sets = append(sets, config.CommandPath{Name: name, Path: path})
		}
		if eof || eofN || eofP {
			break
		}
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
	if def == "" {
		fmt.Fprintf(w, "%s: ", label)
	} else {
		fmt.Fprintf(w, "%s [%s]: ", label, def)
	}
	line, err := r.ReadString('\n')
	eof := errors.Is(err, io.EOF)
	if s := strings.TrimSpace(line); s != "" {
		return s, eof
	}
	return def, eof
}

func isYes(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes":
		return true
	}
	return false
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
