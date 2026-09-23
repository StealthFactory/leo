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
	if len(c.CommandPaths) != 1 || c.CommandPaths[0].Name != "default" ||
		c.CommandPaths[0].Path != filepath.Join(leoDir, "commands") {
		t.Errorf("CommandPaths = %+v", c.CommandPaths)
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

[[command_path]]
name = "work"
path = "/tmp/leo-work"

[[command_path]]
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
	if len(c.CommandPaths) != 2 || c.CommandPaths[0].Name != "work" || c.CommandPaths[1].Name != "personal" {
		t.Errorf("CommandPaths = %+v", c.CommandPaths)
	}
}

func TestLoadLeoPathPrepends(t *testing.T) {
	leoDir := isolate(t)
	t.Setenv("LEO_PATH", "/env/one"+string(os.PathListSeparator)+"/env/two")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	// env sets first (grouped "env"), then the default set.
	if len(c.CommandPaths) != 3 {
		t.Fatalf("want 3 sets, got %+v", c.CommandPaths)
	}
	if c.CommandPaths[0].Name != "env" || c.CommandPaths[0].Path != "/env/one" {
		t.Errorf("first set = %+v", c.CommandPaths[0])
	}
	if c.CommandPaths[1].Path != "/env/two" {
		t.Errorf("second set = %+v", c.CommandPaths[1])
	}
	if c.CommandPaths[2].Path != filepath.Join(leoDir, "commands") {
		t.Errorf("default set missing: %+v", c.CommandPaths[2])
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

func TestInit(t *testing.T) {
	leoDir := isolate(t)
	path, err := Init(false)
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(leoDir, "config.toml") {
		t.Errorf("Init path = %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config not written: %v", err)
	}
	// Refuses to overwrite without force.
	if _, err := Init(false); err == nil {
		t.Error("second Init(false) should error")
	}
	if _, err := Init(true); err != nil {
		t.Errorf("Init(true) should overwrite: %v", err)
	}
	// The written config parses back into a usable default set.
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.CommandPaths) != 1 || c.CommandPaths[0].Name != "default" {
		t.Errorf("initialized config did not load a default set: %+v", c.CommandPaths)
	}
}
