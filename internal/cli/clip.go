package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"leo/internal/clip"
	"leo/internal/config"
	"leo/internal/store"
)

func newClipCmd(cfg *config.Config) *cobra.Command {
	var forceFile, forceText, prettyOut bool
	cmd := &cobra.Command{
		Use:     "clip <key-or-path>...",
		Short:   "Copy a store value or path to the clipboard (macOS)",
		GroupID: groupBuiltin,
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !clip.Supported() {
				return clip.ErrUnsupported
			}
			if forceFile && forceText {
				return fmt.Errorf("--file and --text are mutually exclusive")
			}
			st, err := store.Open(cfg.StorePath)
			if err != nil {
				return err
			}

			// Resolve each argument: store key first (its value), then an
			// existing file path. Anything else is unresolved.
			texts := make([]string, len(args))
			allFiles := true
			var missing []string
			for i, arg := range args {
				text, isFile, ok := resolveClipArg(st, arg, prettyOut)
				texts[i] = text
				if !isFile {
					allFiles = false
				}
				if !ok {
					missing = append(missing, arg)
				}
			}

			// A bare argument that is neither a store key nor a file on disk is
			// almost certainly a mistyped key. Copy nothing and say so, instead
			// of silently copying the literal text. --text and --file are the
			// explicit opt-ins (literal text, or a path CopyFiles validates).
			if len(missing) > 0 && !forceText && !forceFile {
				return notInStore(missing)
			}

			fileMode := forceFile || (!forceText && allFiles)
			if fileMode {
				if err := clip.CopyFiles(texts); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "copied %d file(s) to clipboard\n", len(texts))
				return nil
			}
			if err := clip.CopyText(strings.Join(texts, "\n")); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "copied text to clipboard")
			return nil
		},
	}
	cmd.Flags().BoolVar(&forceFile, "file", false, "treat arguments as file paths (file objects)")
	cmd.Flags().BoolVar(&forceText, "text", false, "copy as text")
	cmd.Flags().BoolVar(&prettyOut, "pretty", false, "pretty-print JSON values when copying as text")
	return cmd
}

// resolveClipArg resolves a clip argument to its text form. isFile reports
// whether that text names an existing file (a file-object candidate); resolved
// reports whether the argument matched anything at all (a store key or an
// existing file). A store key resolves to its value: strings become their
// unquoted text; other JSON values become JSON text (compact, or pretty when
// requested). An argument that is neither a key nor a file is unresolved.
func resolveClipArg(st *store.Store, arg string, prettyOut bool) (text string, isFile, resolved bool) {
	if val, ok := st.Get(arg); ok {
		if store.Kind(val) == "string" {
			s := store.UnquoteString(val)
			return s, isExistingFile(s), true
		}
		if prettyOut {
			return pretty(val), false, true
		}
		return compact(val), false, true
	}
	if isExistingFile(arg) {
		return arg, true, true
	}
	return arg, false, false
}

// notInStore builds the friendly error shown when clip arguments match neither
// a store key nor a file on disk.
func notInStore(missing []string) error {
	if len(missing) == 1 {
		return fmt.Errorf("hmm, there's no %q in your store, so there's nothing to copy", missing[0])
	}
	quoted := make([]string, len(missing))
	for i, m := range missing {
		quoted[i] = fmt.Sprintf("%q", m)
	}
	return fmt.Errorf("hmm, none of these are in your store: %s (nothing copied)", strings.Join(quoted, ", "))
}

// isExistingFile reports whether p (after ~ expansion / abs resolution) is an
// existing filesystem path.
func isExistingFile(p string) bool {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				p = h
			} else {
				p = filepath.Join(h, p[2:])
			}
		}
	}
	if !filepath.IsAbs(p) {
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
	}
	_, err := os.Stat(p)
	return err == nil
}
