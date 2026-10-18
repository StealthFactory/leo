// Package templates embeds the subcommand scaffolding templates used by
// `leo generate`.
package templates

import "embed"

// FS holds the embedded *.tmpl files.
//
//go:embed *.tmpl
var FS embed.FS
