package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeLang(t *testing.T) {
	cases := map[string]string{
		"sh":         "bash",
		"bash":       "bash",
		"py":         "python",
		"python":     "python",
		"js":         "node",
		"javascript": "node",
		"node":       "node",
		"ts":         "typescript",
		"typescript": "typescript",
	}
	for in, want := range cases {
		got, err := NormalizeLang(in)
		if err != nil || got != want {
			t.Errorf("NormalizeLang(%q) = (%q,%v), want %q", in, got, err, want)
		}
	}
	if _, err := NormalizeLang("cobol"); err == nil {
		t.Error("unknown language should error")
	}
}

func TestGenerate(t *testing.T) {
	cases := []struct {
		lang    string
		file    string
		shebang string
	}{
		{"bash", "leo-x", "#!/usr/bin/env bash"},
		{"zsh", "leo-x", "#!/usr/bin/env zsh"},
		{"python", "leo-x", "#!/usr/bin/env python3"},
		{"node", "leo-x", "#!/usr/bin/env node"},
		{"typescript", "leo-x.mts", "#!/usr/bin/env node"},
	}
	for _, c := range cases {
		t.Run(c.lang, func(t *testing.T) {
			dir := t.TempDir()
			path, err := Generate(dir, "x", c.lang, false)
			if err != nil {
				t.Fatal(err)
			}
			if filepath.Base(path) != c.file {
				t.Errorf("file = %q, want %q", filepath.Base(path), c.file)
			}
			fi, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if fi.Mode().Perm() != 0o755 {
				t.Errorf("mode = %o, want 755", fi.Mode().Perm())
			}
			b, _ := os.ReadFile(path)
			if !strings.HasPrefix(string(b), c.shebang) {
				t.Errorf("%s shebang wrong:\n%s", c.lang, b)
			}
			if !strings.Contains(string(b), "leo:") {
				t.Errorf("%s missing leo: marker", c.lang)
			}
		})
	}
}

func TestGenerateRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	if _, err := Generate(dir, "dup", "bash", false); err != nil {
		t.Fatal(err)
	}
	if _, err := Generate(dir, "dup", "bash", false); err == nil {
		t.Error("overwrite without force should error")
	}
	if _, err := Generate(dir, "dup", "bash", true); err != nil {
		t.Errorf("force overwrite should succeed: %v", err)
	}
}

func TestGenerateInvalidName(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{"", "bad name", "has/slash", "-leading"} {
		if _, err := Generate(dir, bad, "bash", false); err == nil {
			t.Errorf("name %q should be rejected", bad)
		}
	}
}
