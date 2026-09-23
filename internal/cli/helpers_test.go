package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"leo/internal/config"
)

// testCfg returns a Config rooted at a fresh temp dir, with a single "default"
// command-path set. Nothing is created on disk until a command writes to it.
func testCfg(t *testing.T) *config.Config {
	t.Helper()
	dir := t.TempDir()
	return &config.Config{
		BaseDir:      dir,
		ConfigPath:   filepath.Join(dir, "config.toml"),
		StorePath:    filepath.Join(dir, "store.json"),
		GenLang:      "bash",
		CommandPaths: []config.CommandPath{{Name: "default", Path: filepath.Join(dir, "commands")}},
	}
}

// runCmd executes cmd with the given stdin and args, capturing stdout and
// stderr into one buffer, and returns that output plus any error. cobra's own
// error/usage printing is silenced so the buffer holds only what the command
// deliberately writes.
func runCmd(t *testing.T, cmd *cobra.Command, stdin string, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	if stdin != "" {
		cmd.SetIn(strings.NewReader(stdin))
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}
