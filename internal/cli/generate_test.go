package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateCreatesSubcommand(t *testing.T) {
	cfg := testCfg(t)
	dir := cfg.CommandPaths[0].Path

	if _, err := runCmd(t, newGenerateCmd(cfg), "", "hello"); err != nil {
		t.Fatalf("generate hello: %v", err)
	}
	p := filepath.Join(dir, "leo-hello")
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("leo-hello not created: %v", err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("mode = %o, want 755", fi.Mode().Perm())
	}
	b, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(b), "#!/usr/bin/env bash") {
		t.Errorf("missing bash shebang:\n%s", b)
	}
	if !strings.Contains(string(b), "leo hello") {
		t.Errorf("name not substituted into template:\n%s", b)
	}
}

func TestGenerateLanguages(t *testing.T) {
	cases := []struct {
		lang    string
		file    string
		shebang string
	}{
		{"zsh", "leo-a", "#!/usr/bin/env zsh"},
		{"py", "leo-a", "#!/usr/bin/env python3"},
		{"js", "leo-a", "#!/usr/bin/env node"},
		{"ts", "leo-a.mts", "#!/usr/bin/env node"},
	}
	for _, c := range cases {
		t.Run(c.lang, func(t *testing.T) {
			cfg := testCfg(t)
			if _, err := runCmd(t, newGenerateCmd(cfg), "", "a", "--lang", c.lang); err != nil {
				t.Fatalf("generate --lang %s: %v", c.lang, err)
			}
			b, err := os.ReadFile(filepath.Join(cfg.CommandPaths[0].Path, c.file))
			if err != nil {
				t.Fatalf("expected %s: %v", c.file, err)
			}
			if !strings.HasPrefix(string(b), c.shebang) {
				t.Errorf("%s: shebang = %q...", c.lang, string(b)[:min(len(b), 30)])
			}
		})
	}
}

func TestGenerateRefusesOverwrite(t *testing.T) {
	cfg := testCfg(t)
	if _, err := runCmd(t, newGenerateCmd(cfg), "", "dup"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCmd(t, newGenerateCmd(cfg), "", "dup"); err == nil {
		t.Error("second generate without --force should error")
	}
	if _, err := runCmd(t, newGenerateCmd(cfg), "", "dup", "--force"); err != nil {
		t.Errorf("generate --force should succeed: %v", err)
	}
}

func TestGenerateInvalidName(t *testing.T) {
	cfg := testCfg(t)
	if _, err := runCmd(t, newGenerateCmd(cfg), "", "bad name"); err == nil {
		t.Error("invalid subcommand name should error")
	}
}

func TestGenerateUnknownEnv(t *testing.T) {
	cfg := testCfg(t)
	if _, err := runCmd(t, newGenerateCmd(cfg), "", "x", "--env", "nope"); err == nil {
		t.Error("unknown --env should error")
	}
}

func TestGenerateBareShowsHelp(t *testing.T) {
	cfg := testCfg(t)
	out, err := runCmd(t, newGenerateCmd(cfg), "")
	if err != nil {
		t.Fatalf("bare generate should not error: %v", err)
	}
	for _, want := range []string{"Usage:", "leo generate", "Languages:", "--lang"} {
		if !strings.Contains(out, want) {
			t.Errorf("bare generate help missing %q:\n%s", want, out)
		}
	}
}
