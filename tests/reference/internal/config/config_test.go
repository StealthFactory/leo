package config

import (
	"os"
	"path/filepath"
	"testing"
)

// isolate points the loader at a fresh base dir and clears the env inputs that
// would otherwise leak in from the developer's shell.
func isolate(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("LEO_CONFIG", "")
	t.Setenv("LEO_PATH", "")
	return filepath.Join(base, "leo")
}

func TestLoadDefaults(t *testing.T) {
	leoDir := isolate(t)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.StorePath != filepath.Join(leoDir, "store.json") {
		t.Errorf("StorePath = %q", c.StorePath)
	}
	if c.ConfigPath != filepath.Join(leoDir, "config.toml") {
		t.Errorf("ConfigPath = %q", c.ConfigPath)
	}
	if c.GenLang != "bash" {
		t.Errorf("GenLang = %q, want bash", c.GenLang)
	}
	if len(c.Workspaces) != 1 || c.Workspaces[0].Name != "default" ||
		c.Workspaces[0].Path != filepath.Join(leoDir, "commands") {
		t.Errorf("Workspaces = %+v", c.Workspaces)
	}
}

func TestLoadTOML(t *testing.T) {
	leoDir := isolate(t)
	if err := os.MkdirAll(leoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := `
store_path = "/tmp/leo-x/store.json"
gen_lang = "python"

[[workspace]]
name = "work"
path = "/tmp/leo-work"

[[workspace]]
name = "personal"
path = "/tmp/leo-personal"
`
	if err := os.WriteFile(filepath.Join(leoDir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.StorePath != "/tmp/leo-x/store.json" {
		t.Errorf("StorePath = %q", c.StorePath)
	}
	if c.GenLang != "python" {
		t.Errorf("GenLang = %q", c.GenLang)
	}
	if len(c.Workspaces) != 2 || c.Workspaces[0].Name != "work" || c.Workspaces[1].Name != "personal" {
		t.Errorf("Workspaces = %+v", c.Workspaces)
	}
}

func TestLoadLeoPathPrepends(t *testing.T) {
	leoDir := isolate(t)
	t.Setenv("LEO_PATH", "/env/one"+string(os.PathListSeparator)+"/env/two")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	// env workspaces first (grouped "env"), then the default workspace.
	if len(c.Workspaces) != 3 {
		t.Fatalf("want 3 workspaces, got %+v", c.Workspaces)
	}
	if c.Workspaces[0].Name != "env" || c.Workspaces[0].Path != "/env/one" {
		t.Errorf("first workspace = %+v", c.Workspaces[0])
	}
	if c.Workspaces[1].Path != "/env/two" {
		t.Errorf("second workspace = %+v", c.Workspaces[1])
	}
	if c.Workspaces[2].Path != filepath.Join(leoDir, "commands") {
		t.Errorf("default workspace missing: %+v", c.Workspaces[2])
	}
}

func TestExpandTilde(t *testing.T) {
	leoDir := isolate(t)
	if err := os.MkdirAll(leoDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(leoDir, "config.toml"),
		[]byte(`store_path = "~/leo-store.json"`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	if c.StorePath != filepath.Join(home, "leo-store.json") {
		t.Errorf("~ not expanded: %q", c.StorePath)
	}
}
