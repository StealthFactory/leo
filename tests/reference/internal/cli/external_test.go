package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"leo/internal/config"
)

func TestExternalName(t *testing.T) {
	cases := []struct {
		file     string
		wantName string
		wantOK   bool
	}{
		{"leo-hello", "hello", true},
		{"leo-deploy.sh", "deploy", true},
		{"leo-build.mts", "build", true},
		{"leo-tool.py", "tool", true},
		{"leo-app.js", "app", true},
		{"leo-", "", false},
		{"leo-notes.txt", "", false}, // unknown extension
		{"notleo", "", false},
		{"leo", "", false},
	}
	for _, c := range cases {
		name, ok := externalName(c.file)
		if name != c.wantName || ok != c.wantOK {
			t.Errorf("externalName(%q) = (%q,%v), want (%q,%v)", c.file, name, ok, c.wantName, c.wantOK)
		}
	}
}

func TestMarkerShort(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	bash := write("leo-a", "#!/usr/bin/env bash\n# leo: greet someone\necho hi\n")
	node := write("leo-b", "#!/usr/bin/env node\n// leo: build the thing\n")
	none := write("leo-c", "#!/usr/bin/env bash\necho hi\n")

	if got := markerShort(bash); got != "greet someone" {
		t.Errorf("bash marker = %q", got)
	}
	if got := markerShort(node); got != "build the thing" {
		t.Errorf("node marker = %q", got)
	}
	if got := markerShort(none); got != "" {
		t.Errorf("no marker should be empty, got %q", got)
	}
}

func findCmd(root *cobra.Command, name string) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}

func TestRegisterExternalsDiscoveryAndShadowing(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("leo-hello", "#!/usr/bin/env bash\n# leo: say hello\necho hi\n")
	mustWrite("leo-tool.py", "#!/usr/bin/env python3\n# leo: py tool\n")
	mustWrite("leo-store", "#!/usr/bin/env bash\n# leo: should be shadowed\n") // collides with builtin
	mustWrite("leo-notes.txt", "not a subcommand\n")                           // unknown extension
	mustWrite("random", "ignored\n")                                           // no leo- prefix

	cfg := testCfg(t)
	cfg.Workspaces = []config.Workspace{{Name: "default", Path: dir}}

	root := &cobra.Command{Use: "leo"}
	root.AddCommand(&cobra.Command{Use: "store", Short: "builtin store"}) // pre-existing builtin

	registerExternals(root, cfg)

	if c := findCmd(root, "hello"); c == nil || c.Short != "say hello" || c.GroupID != "workspace:default" {
		t.Errorf("hello not registered correctly: %+v", c)
	}
	if c := findCmd(root, "tool"); c == nil || c.Short != "py tool" {
		t.Errorf("tool not registered correctly: %+v", c)
	}
	if c := findCmd(root, "store"); c == nil || c.Short != "builtin store" {
		t.Error("builtin store should shadow leo-store")
	}
	if findCmd(root, "notes") != nil {
		t.Error("unknown-extension file should not register")
	}
	if findCmd(root, "random") != nil {
		t.Error("non leo- file should not register")
	}

	// The workspace's group should have been added.
	hasGroup := false
	for _, g := range root.Groups() {
		if g.ID == "workspace:default" {
			hasGroup = true
		}
	}
	if !hasGroup {
		t.Error("group workspace:default was not added")
	}
}

func TestRegisterExternalsEarlierWorkspaceWins(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	os.WriteFile(filepath.Join(first, "leo-dup"), []byte("#!/usr/bin/env bash\n# leo: from first\n"), 0o755)
	os.WriteFile(filepath.Join(second, "leo-dup"), []byte("#!/usr/bin/env bash\n# leo: from second\n"), 0o755)

	cfg := testCfg(t)
	cfg.Workspaces = []config.Workspace{
		{Name: "a", Path: first},
		{Name: "b", Path: second},
	}
	root := &cobra.Command{Use: "leo"}
	registerExternals(root, cfg)

	if c := findCmd(root, "dup"); c == nil || c.Short != "from first" {
		t.Errorf("earlier workspace should win: %+v", c)
	}
}

func TestChildEnv(t *testing.T) {
	cfg := testCfg(t)
	env := childEnv(cfg)

	m := map[string]string{}
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			m[kv[:i]] = kv[i+1:]
		}
	}
	if m["LEO_STORE"] != cfg.StorePath {
		t.Errorf("LEO_STORE = %q, want %q", m["LEO_STORE"], cfg.StorePath)
	}
	if m["LEO_CONFIG"] != cfg.ConfigPath {
		t.Errorf("LEO_CONFIG = %q, want %q", m["LEO_CONFIG"], cfg.ConfigPath)
	}
	if m["LEO_WORKSPACE_PATHS"] != cfg.Workspaces[0].Path {
		t.Errorf("LEO_WORKSPACE_PATHS = %q, want %q", m["LEO_WORKSPACE_PATHS"], cfg.Workspaces[0].Path)
	}
	if m["LEO_BIN"] == "" {
		t.Error("LEO_BIN should be set")
	}
	if !strings.HasPrefix(m["PATH"], filepath.Dir(m["LEO_BIN"])) {
		t.Errorf("PATH should start with LEO_BIN's dir: %q", m["PATH"])
	}
}

func TestExternalRunsWithInjectedEnv(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "leo-greet")
	body := "#!/usr/bin/env bash\n# leo: greet someone\necho \"greet:$1 store=$LEO_STORE\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := testCfg(t)
	cfg.Workspaces = []config.Workspace{{Name: "default", Path: dir}}

	root := &cobra.Command{Use: "leo"}
	registerExternals(root, cfg)
	if findCmd(root, "greet") == nil {
		t.Fatal("greet not registered")
	}

	out, err := runCmd(t, root, "", "greet", "alice")
	if err != nil {
		t.Fatalf("running external: %v (out=%q)", err, out)
	}
	if !strings.Contains(out, "greet:alice") {
		t.Errorf("external output missing args: %q", out)
	}
	if !strings.Contains(out, "store="+cfg.StorePath) {
		t.Errorf("external did not receive LEO_STORE: %q", out)
	}
}

func TestExternalInterruptHelper(t *testing.T) {
	script := os.Getenv("LEO_TEST_INTERRUPT_SCRIPT")
	if script == "" {
		return
	}
	if err := runExternal(&cobra.Command{}, script, nil, testCfg(t)); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func TestExternalTerminalInterrupt(t *testing.T) {
	cases := []struct {
		name string
		trap string
		want int
	}{
		{"handled", "trap 'exit 0' INT", 0},
		{"handled-error", "trap 'exit 7' INT", 7},
		{"unhandled", "trap - INT", 130},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			script := filepath.Join(t.TempDir(), "leo-interrupt")
			body := "#!/bin/sh\n" + tc.trap + "\nprintf 'ready\\n'\nwhile :; do :; done\n"
			if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
				t.Fatal(err)
			}
			c := exec.Command(os.Args[0], "-test.run=^TestExternalInterruptHelper$")
			c.Env = append(os.Environ(), "LEO_TEST_INTERRUPT_SCRIPT="+script)
			c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			stdout, err := c.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			c.Stderr = os.Stderr
			if err := c.Start(); err != nil {
				t.Fatal(err)
			}
			ready := make(chan error, 1)
			go func() {
				line, err := bufio.NewReader(stdout).ReadString('\n')
				if err == nil && line != "ready\n" {
					err = fmt.Errorf("unexpected startup output: %q", line)
				}
				ready <- err
			}()
			wait := make(chan error, 1)
			go func() { wait <- c.Wait() }()
			finished := false
			defer func() {
				if !finished {
					syscall.Kill(-c.Process.Pid, syscall.SIGKILL)
					<-wait
				}
			}()
			select {
			case err := <-ready:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("external command did not become ready")
			}
			if err := syscall.Kill(-c.Process.Pid, syscall.SIGINT); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-wait:
				finished = true
				if tc.want == 0 && err != nil {
					t.Fatalf("handled interrupt: %v", err)
				}
				if got := c.ProcessState.ExitCode(); got != tc.want {
					t.Fatalf("exit status = %d, want %d", got, tc.want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("external command did not stop after Ctrl-C")
			}
		})
	}
}
