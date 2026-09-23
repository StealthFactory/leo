// Package config resolves leo's configuration: the base dir (XDG-aware),
// the store path, the default generate language, and the ordered list of
// command-path sets (from LEO_PATH and the TOML config).
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// CommandPath is a named directory that leo scans for external `leo-*`
// subcommands. Order matters: earlier sets win on name collisions and are
// listed first in `leo help`.
type CommandPath struct {
	Name string // group label, e.g. "default", "work", "env"
	Path string // resolved, absolute directory
}

// Config is the effective, fully-resolved configuration for a run.
type Config struct {
	BaseDir      string        // ${XDG_CONFIG_HOME:-~/.config}/leo
	ConfigPath   string        // resolved config.toml path
	StorePath    string        // resolved store.json path
	GenLang      string        // default `leo generate` language
	CommandPaths []CommandPath // ordered: LEO_PATH (env) first, then config sets
}

// tomlConfig mirrors the on-disk config.toml schema. All keys are optional.
type tomlConfig struct {
	StorePath   string `toml:"store_path"`
	GenLang     string `toml:"gen_lang"`
	CommandPath []struct {
		Name string `toml:"name"`
		Path string `toml:"path"`
	} `toml:"command_path"`
}

// home returns the user's home directory, falling back to $HOME.
func home() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return os.Getenv("HOME")
}

// expandPath expands a leading ~ and any $VARS in p. It never fails: an
// unexpandable input is returned as-is so callers get a usable (if literal)
// path rather than an error deep in a code path.
func expandPath(p string) string {
	if p == "" {
		return p
	}
	p = os.ExpandEnv(p)
	switch {
	case p == "~":
		return home()
	case strings.HasPrefix(p, "~/"):
		return filepath.Join(home(), p[2:])
	}
	return p
}

// baseDir returns ${XDG_CONFIG_HOME:-~/.config}/leo.
func baseDir() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(expandPath(x), "leo")
	}
	return filepath.Join(home(), ".config", "leo")
}

// Load resolves the effective configuration from env + config.toml.
func Load() (*Config, error) {
	base := baseDir()

	cfgPath := os.Getenv("LEO_CONFIG")
	if cfgPath == "" {
		cfgPath = filepath.Join(base, "config.toml")
	} else {
		cfgPath = expandPath(cfgPath)
	}

	c := &Config{
		BaseDir:    base,
		ConfigPath: cfgPath,
		StorePath:  filepath.Join(base, "store.json"),
		GenLang:    "bash",
	}

	var tc tomlConfig
	if b, err := os.ReadFile(cfgPath); err == nil {
		if err := toml.Unmarshal(b, &tc); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", cfgPath, err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading %s: %w", cfgPath, err)
	}

	if tc.StorePath != "" {
		c.StorePath = expandPath(tc.StorePath)
	}
	if tc.GenLang != "" {
		c.GenLang = tc.GenLang
	}

	// Command-path sets: LEO_PATH env sets first (group "env"), then config
	// sets in listed order. If neither is present, a single "default" set.
	seen := map[string]bool{}
	add := func(name, path string) {
		path = expandPath(path)
		if path == "" || seen[path] {
			return
		}
		seen[path] = true
		c.CommandPaths = append(c.CommandPaths, CommandPath{Name: name, Path: path})
	}

	if lp := os.Getenv("LEO_PATH"); lp != "" {
		for _, dir := range filepath.SplitList(lp) {
			if strings.TrimSpace(dir) != "" {
				add("env", dir)
			}
		}
	}

	if len(tc.CommandPath) > 0 {
		for _, cp := range tc.CommandPath {
			name := cp.Name
			if name == "" {
				name = "default"
			}
			add(name, cp.Path)
		}
	} else {
		add("default", filepath.Join(base, "commands"))
	}

	return c, nil
}

// defaultConfigTOML is written by `leo config init`.
const defaultConfigTOML = `# leo configuration (TOML). All keys are optional.

# Where the object store lives.
store_path = "%s"

# Default language for ` + "`leo generate`" + `: bash|zsh|python|node|typescript
gen_lang = "bash"

# One or more named command-path sets, searched in the order listed.
# Earlier sets win on name collisions and appear first in ` + "`leo help`" + `.
[[command_path]]
name = "default"
path = "%s"

# Example of a second set (uncomment and adjust):
# [[command_path]]
# name = "work"
# path = "~/work/leo-commands"
`

// Init writes a commented default config.toml. It refuses to overwrite an
// existing file unless force is true.
func Init(force bool) (string, error) {
	base := baseDir()
	cfgPath := os.Getenv("LEO_CONFIG")
	if cfgPath == "" {
		cfgPath = filepath.Join(base, "config.toml")
	} else {
		cfgPath = expandPath(cfgPath)
	}

	if !force {
		if _, err := os.Stat(cfgPath); err == nil {
			return cfgPath, fmt.Errorf("%s already exists (use --force to overwrite)", cfgPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return cfgPath, err
	}
	content := fmt.Sprintf(defaultConfigTOML,
		filepath.Join(base, "store.json"),
		filepath.Join(base, "commands"),
	)
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		return cfgPath, err
	}
	return cfgPath, nil
}
