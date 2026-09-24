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

// FileConfig holds the values persisted in config.toml. Unlike Config, these are
// the literal on-disk values (paths unexpanded), suitable for round-tripping
// through `leo config setup`.
type FileConfig struct {
	StorePath string
	GenLang   string
	Sets      []CommandPath
}

// ReadFileConfig parses config.toml at path into a FileConfig. A missing file
// yields a zero FileConfig and no error, so callers can fall back to defaults.
func ReadFileConfig(path string) (FileConfig, error) {
	var fc FileConfig
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fc, nil
		}
		return fc, fmt.Errorf("reading %s: %w", path, err)
	}
	var tc tomlConfig
	if err := toml.Unmarshal(b, &tc); err != nil {
		return fc, fmt.Errorf("parsing %s: %w", path, err)
	}
	fc.StorePath = tc.StorePath
	fc.GenLang = tc.GenLang
	for _, cp := range tc.CommandPath {
		name := cp.Name
		if name == "" {
			name = "default"
		}
		fc.Sets = append(fc.Sets, CommandPath{Name: name, Path: cp.Path})
	}
	return fc, nil
}

// WriteConfig renders a commented config.toml from fc and writes it to path
// (0644), creating the parent directory (0755) if needed.
func WriteConfig(path string, fc FileConfig) error {
	var b strings.Builder
	b.WriteString("# leo configuration (TOML). All keys are optional.\n\n")
	b.WriteString("# Where the object store lives.\n")
	fmt.Fprintf(&b, "store_path = %s\n\n", tomlString(fc.StorePath))
	b.WriteString("# Default language for `leo generate`: bash|zsh|python|node|typescript\n")
	fmt.Fprintf(&b, "gen_lang = %s\n\n", tomlString(fc.GenLang))
	b.WriteString("# One or more named command-path sets, searched in the order listed.\n")
	b.WriteString("# Earlier sets win on name collisions and appear first in `leo help`.\n")
	for _, s := range fc.Sets {
		b.WriteString("\n[[command_path]]\n")
		fmt.Fprintf(&b, "name = %s\n", tomlString(s.Name))
		fmt.Fprintf(&b, "path = %s\n", tomlString(s.Path))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// tomlString renders s as a TOML basic string with the two structural
// characters (backslash and double-quote) escaped, so an untrusted value cannot
// break out of its string or inject additional keys. Interactive input is read
// a line at a time, so it never contains the newline that would otherwise make
// a basic string invalid.
func tomlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
