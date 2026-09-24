package cli

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"leo/internal/config"
)

// knownExts are the file extensions recognized on external subcommand files.
// The subcommand name is the basename minus the "leo-" prefix and any such
// extension.
var knownExts = map[string]bool{
	".sh": true, ".bash": true, ".zsh": true,
	".py": true,
	".js": true, ".mjs": true, ".cjs": true,
	".ts": true, ".mts": true, ".cts": true,
}

// registerExternals discovers leo-* scripts across the command-path sets (then
// $PATH) and registers each as a cobra command. First registration wins:
// built-ins beat externals, and earlier sets beat later ones.
func registerExternals(root *cobra.Command, cfg *config.Config) {
	seen := map[string]bool{}
	for _, c := range root.Commands() {
		seen[c.Name()] = true
		for _, a := range c.Aliases {
			seen[a] = true
		}
	}
	// Reserve names cobra adds lazily so an external can't collide with them.
	seen["help"] = true

	addedGroups := map[string]bool{}

	register := func(setName, groupID, file, name string) {
		if seen[name] {
			return // shadowed by a built-in or an earlier set
		}
		seen[name] = true
		if !addedGroups[groupID] {
			root.AddGroup(&cobra.Group{ID: groupID, Title: "[" + setName + "]"})
			addedGroups[groupID] = true
		}
		root.AddCommand(&cobra.Command{
			Use:                name,
			Short:              markerShort(file),
			GroupID:            groupID,
			DisableFlagParsing: true,
			RunE: func(cmd *cobra.Command, args []string) error {
				return runExternal(cmd, file, args, cfg)
			},
		})
	}

	scan := func(setName, dir string) {
		groupID := "set:" + setName
		entries, err := os.ReadDir(dir)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name, ok := externalName(e.Name())
			if !ok {
				continue
			}
			register(setName, groupID, filepath.Join(dir, e.Name()), name)
		}
	}

	for _, cp := range cfg.CommandPaths {
		scan(cp.Name, cp.Path)
	}
	// Finally, leo-* found on $PATH (lowest precedence).
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		if dir != "" {
			scan("$PATH", dir)
		}
	}
}

// externalName returns the subcommand name for a filename, and whether the file
// is a recognized external subcommand (extensionless leo-* or a known
// extension).
func externalName(filename string) (string, bool) {
	if !strings.HasPrefix(filename, "leo-") {
		return "", false
	}
	base := strings.TrimPrefix(filename, "leo-")
	if base == "" {
		return "", false
	}
	ext := filepath.Ext(base)
	if ext == "" {
		return base, true
	}
	if knownExts[ext] {
		return strings.TrimSuffix(base, ext), true
	}
	return "", false // unknown extension: not a subcommand file
}

// markerShort reads the `# leo:` / `// leo:` marker comment from a script and
// returns its text for cobra's Short help. Empty if none is found.
func markerShort(file string) string {
	f, err := os.Open(file)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for i := 0; sc.Scan() && i < 20; i++ {
		line := strings.TrimSpace(sc.Text())
		for _, prefix := range []string{"# leo:", "// leo:"} {
			if strings.HasPrefix(line, prefix) {
				return strings.TrimSpace(strings.TrimPrefix(line, prefix))
			}
		}
	}
	return ""
}

// runExternal execs an external subcommand file with args, injecting leo's
// environment. The file is invoked directly via an argument array (no shell),
// so arguments are never re-parsed by a shell. The child inherits the command's
// stdio (the real terminal in normal use) and its exit code is propagated.
func runExternal(cmd *cobra.Command, file string, args []string, cfg *config.Config) error {
	c := exec.Command(file, args...)
	c.Stdin = cmd.InOrStdin()
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	c.Env = childEnv(cfg)

	if err := c.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			os.Exit(ee.ExitCode())
		}
		return err
	}
	return nil
}

// childEnv builds the environment for an external subcommand: the current
// environment plus LEO_STORE/LEO_CONFIG/LEO_COMMAND_PATHS/LEO_BIN, with
// LEO_BIN's directory prepended to PATH so a script can call `leo ...`.
func childEnv(cfg *config.Config) []string {
	bin, _ := os.Executable()
	binDir := filepath.Dir(bin)

	paths := make([]string, 0, len(cfg.CommandPaths))
	for _, cp := range cfg.CommandPaths {
		paths = append(paths, cp.Path)
	}
	sep := string(os.PathListSeparator)

	overrides := map[string]string{
		"LEO_STORE":         cfg.StorePath,
		"LEO_CONFIG":        cfg.ConfigPath,
		"LEO_COMMAND_PATHS": strings.Join(paths, sep),
		"LEO_BIN":           bin,
		"PATH":              binDir + sep + os.Getenv("PATH"),
	}

	out := make([]string, 0, len(os.Environ())+len(overrides))
	for _, kv := range os.Environ() {
		key := kv[:strings.IndexByte(kv, '=')+1]
		name := strings.TrimSuffix(key, "=")
		if _, ok := overrides[name]; ok {
			continue // replaced below
		}
		out = append(out, kv)
	}
	for k, v := range overrides {
		out = append(out, k+"="+v)
	}
	return out
}
