// Package scaffold implements `leo generate`: rendering an embedded template
// for a new external subcommand into a chosen command-path set.
package scaffold

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"text/template"

	"leo/templates"
)

// langSpec maps a normalized language to its template file and output suffix.
type langSpec struct {
	tmpl   string // template filename in the embedded FS
	suffix string // appended to leo-<name> ("" for extensionless)
}

var langs = map[string]langSpec{
	"bash":       {"subcommand.sh.tmpl", ""},
	"zsh":        {"subcommand.zsh.tmpl", ""},
	"python":     {"subcommand.py.tmpl", ""},
	"node":       {"subcommand.js.tmpl", ""},
	"typescript": {"subcommand.ts.tmpl", ".mts"},
}

// aliases normalizes shorthand language names.
var aliases = map[string]string{
	"sh":         "bash",
	"py":         "python",
	"js":         "node",
	"javascript": "node",
	"ts":         "typescript",
}

// nameRe restricts subcommand names to a safe, filesystem- and shell-friendly
// set so `generate` can never write outside the target dir or produce an
// unusable command name.
var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// NormalizeLang resolves aliases and validates a language, returning the
// canonical name.
func NormalizeLang(lang string) (string, error) {
	if canonical, ok := aliases[lang]; ok {
		lang = canonical
	}
	if _, ok := langs[lang]; !ok {
		return "", fmt.Errorf("unknown language %q (want bash|zsh|python|node|typescript)", lang)
	}
	return lang, nil
}

// Languages returns the supported canonical language names.
func Languages() []string {
	return []string{"bash", "zsh", "python", "node", "typescript"}
}

// Generate renders the template for lang into dir as leo-<name> (chmod 0755),
// creating dir if missing. It refuses to overwrite an existing file unless
// force is true. It returns the path written.
func Generate(dir, name, lang string, force bool) (string, error) {
	if !nameRe.MatchString(name) {
		return "", fmt.Errorf("invalid subcommand name %q (use letters, digits, - and _)", name)
	}
	lang, err := NormalizeLang(lang)
	if err != nil {
		return "", err
	}
	spec := langs[lang]

	raw, err := templates.FS.ReadFile(spec.tmpl)
	if err != nil {
		return "", fmt.Errorf("loading template: %w", err)
	}
	tmpl, err := template.New(spec.tmpl).Parse(string(raw))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct{ Name string }{Name: name}); err != nil {
		return "", err
	}

	target := filepath.Join(dir, "leo-"+name+spec.suffix)
	if !force {
		if _, err := os.Stat(target); err == nil {
			return target, fmt.Errorf("%s already exists (use --force to overwrite)", target)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(target, buf.Bytes(), 0o755); err != nil {
		return "", err
	}
	return target, nil
}
