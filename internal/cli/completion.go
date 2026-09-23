package cli

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "completion",
		Short:   "Generate or install shell completion scripts",
		GroupID: groupBuiltin,
		Long: "Generate shell completion scripts, or install zsh completion for you.\n\n" +
			"Day-to-day you never run this: after `leo completion install` you just\n" +
			"press <TAB>. The generators (bash/zsh/fish/powershell) print a script to\n" +
			"stdout if you prefer to wire it up yourself.",
	}

	gen := func(use, short string, fn func(*cobra.Command, *bytes.Buffer) error) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			Args:  cobra.NoArgs,
			RunE: func(c *cobra.Command, _ []string) error {
				var buf bytes.Buffer
				if err := fn(c.Root(), &buf); err != nil {
					return err
				}
				_, err := c.OutOrStdout().Write(buf.Bytes())
				return err
			},
		}
	}

	cmd.AddCommand(
		gen("bash", "Print the bash completion script", func(r *cobra.Command, b *bytes.Buffer) error { return r.GenBashCompletionV2(b, true) }),
		gen("zsh", "Print the zsh completion script", func(r *cobra.Command, b *bytes.Buffer) error { return r.GenZshCompletion(b) }),
		gen("fish", "Print the fish completion script", func(r *cobra.Command, b *bytes.Buffer) error { return r.GenFishCompletion(b, true) }),
		gen("powershell", "Print the PowerShell completion script", func(r *cobra.Command, b *bytes.Buffer) error { return r.GenPowerShellCompletionWithDesc(b) }),
		newCompletionInstallCmd(),
	)
	return cmd
}

func newCompletionInstallCmd() *cobra.Command {
	var shell, dir string
	var dryRun, force bool
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install completion into a directory on your shell's completion path",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sh := shell
			if sh == "" {
				sh = detectShell()
			}
			switch sh {
			case "zsh":
				return installZsh(cmd, dir, dryRun, force)
			case "bash", "fish", "powershell":
				return fmt.Errorf("automatic install currently supports zsh; for %s run: leo completion %s > <your completion dir>", sh, sh)
			default:
				return fmt.Errorf("could not detect shell; pass --shell zsh")
			}
		},
	}
	cmd.Flags().StringVar(&shell, "shell", "", "shell to install for (default: autodetect)")
	cmd.Flags().StringVar(&dir, "dir", "", "target directory (default: a dir already on your completion path)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print what would happen without writing")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite / edit rc file even if unnecessary")
	return cmd
}

// detectShell returns the base name of $SHELL, or "".
func detectShell() string {
	if s := os.Getenv("SHELL"); s != "" {
		return filepath.Base(s)
	}
	return ""
}

// installZsh writes the _leo completion function into an on-fpath directory,
// falling back to ~/.zsh/completions (adding an fpath+= line to ~/.zshrc only
// if that directory isn't already on fpath).
func installZsh(cmd *cobra.Command, dir string, dryRun, force bool) error {
	w := cmd.OutOrStdout()

	var buf bytes.Buffer
	if err := cmd.Root().GenZshCompletion(&buf); err != nil {
		return err
	}

	fpath := zshFpath()
	target := dir
	needRcEdit := false
	if target == "" {
		if d := firstWritableDir(fpath); d != "" {
			target = d
		} else {
			target = filepath.Join(userHome(), ".zsh", "completions")
			needRcEdit = !dirOnFpath(fpath, target)
		}
	}

	dest := filepath.Join(target, "_leo")
	if dryRun {
		fmt.Fprintf(w, "would write %s\n", dest)
		if needRcEdit {
			fmt.Fprintf(w, "would add `fpath+=(%s)` to %s\n", target, filepath.Join(userHome(), ".zshrc"))
		}
		return nil
	}

	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dest, buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote %s\n", dest)

	if needRcEdit {
		if err := ensureFpathLine(target, force); err != nil {
			return err
		}
		fmt.Fprintf(w, "added fpath entry to %s\n", filepath.Join(userHome(), ".zshrc"))
	}

	fmt.Fprintln(w, "run `exec zsh` (or open a new shell) once so compinit picks up _leo.")
	return nil
}

// zshFpath returns the entries of zsh's $fpath, or nil if zsh is unavailable.
func zshFpath() []string {
	out, err := exec.Command("zsh", "-c", "print -rl -- $fpath").Output()
	if err != nil {
		return nil
	}
	var dirs []string
	for _, line := range strings.Split(string(out), "\n") {
		if d := strings.TrimSpace(line); d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// firstWritableDir picks the first writable fpath entry, preferring a
// site-functions directory (the conventional home for third-party completions).
func firstWritableDir(fpath []string) string {
	var fallback string
	for _, d := range fpath {
		if !isWritableDir(d) {
			continue
		}
		if strings.Contains(d, "site-functions") {
			return d
		}
		if fallback == "" {
			fallback = d
		}
	}
	return fallback
}

func isWritableDir(d string) bool {
	fi, err := os.Stat(d)
	if err != nil || !fi.IsDir() {
		return false
	}
	// Probe writability by creating and removing a temp file.
	tmp, err := os.CreateTemp(d, ".leo-wtest-*")
	if err != nil {
		return false
	}
	name := tmp.Name()
	tmp.Close()
	os.Remove(name)
	return true
}

func dirOnFpath(fpath []string, dir string) bool {
	abs, _ := filepath.Abs(dir)
	for _, d := range fpath {
		if da, _ := filepath.Abs(d); da == abs {
			return true
		}
	}
	return false
}

// ensureFpathLine appends an `fpath+=` + compinit block to ~/.zshrc if a line
// referencing dir is not already present.
func ensureFpathLine(dir string, force bool) error {
	rc := filepath.Join(userHome(), ".zshrc")
	existing, err := os.ReadFile(rc)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !force && strings.Contains(string(existing), dir) {
		return nil // already referenced
	}
	line := fmt.Sprintf("\n# added by `leo completion install`\nfpath+=(%q)\nautoload -Uz compinit && compinit\n", dir)
	f, err := os.OpenFile(rc, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

func userHome() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return os.Getenv("HOME")
}
