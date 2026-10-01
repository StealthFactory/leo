package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"leo/internal/config"
	"leo/internal/store"
)

// storeMode is what a `leo kv` invocation does, decided by its flags and the
// number of arguments.
type storeMode int

const (
	modeList storeMode = iota
	modeGet
	modeSet
	modeDelete
	modeSearch
	modeMark
)

// secretMask replaces a secret value wherever leo would otherwise display it
// (lists, search results, prompts, and confirmations).
const secretMask = "SECRET"

// storeFlagModes lists the mode-specific flags and where they apply, so misuse
// such as `leo kv --string foo` errors instead of being silently ignored.
var storeFlagModes = []struct {
	name  string
	modes []storeMode
	where string
}{
	{"string", []storeMode{modeSet}, "when setting a value"},
	{"json", []storeMode{modeSet}, "when setting a value"},
	{"file", []storeMode{modeSet}, "when setting a value"},
	{"force", []storeMode{modeSet}, "when setting a value"},
	{"secret", []storeMode{modeSet, modeMark}, "when setting a value or marking a key"},
	{"query", []storeMode{modeGet, modeList, modeSearch}, "when getting, listing, or searching"},
	{"raw", []storeMode{modeGet}, "when getting a value"},
	{"compact", []storeMode{modeGet}, "when getting a value"},
}

type storeOpts struct {
	del, search, force bool
	secret             bool
	asString, asJSON   bool
	file, query        string
	raw, compactOut    bool
}

// isTerminal reports whether r is an interactive terminal. It is a variable so
// tests can simulate one.
var isTerminal = func(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

func newStoreCmd(cfg *config.Config) *cobra.Command {
	var o storeOpts
	cmd := &cobra.Command{
		Use:     "store [flags] [key [value]]",
		Aliases: []string{"kv"},
		Short:   "Get, set, search, and delete values in the JSON store (alias: kv)",
		Long: "Get, set, search, list, and delete values in leo's JSON store.\n\n" +
			"  leo kv                           list every key and value\n" +
			"  leo kv <key>                     print a value\n" +
			"  leo kv <key> <value>             set a value (asks before replacing one)\n" +
			"  leo kv --secret <key> <value>    set a value that lists as SECRET\n" +
			"  leo kv -s <text>                 search keys and values, best matches first\n" +
			"  leo kv -d <key>                  delete a key\n\n" +
			"Values are auto-typed: valid JSON keeps its type, anything else is stored\n" +
			"as a string. Use --string or --json to override, --file <path> to read a\n" +
			"file, or - as the value to read stdin.\n\n" +
			"Secrets show as SECRET in lists, search results, and prompts, while\n" +
			"`leo kv <key>` still prints the real value. Updating a secret keeps it\n" +
			"secret. `leo kv --secret <key>` marks an existing key and --secret=false\n" +
			"unmarks it. Secrets are masked, not encrypted: the store file holds them\n" +
			"in plain text, and lists which keys are secret under the reserved key $leo.\n\n" +
			"Search ignores case and ranks exact key matches first, then keys that\n" +
			"start with the text, keys with a word that starts with it (after . - _ /\n" +
			"or a camelCase hump), keys containing it, and keys containing its letters\n" +
			"in order (dh finds deploy.host). Values containing the text come last;\n" +
			"secret values are never searched.",
		Example: "  leo kv greeting \"hello there\"\n" +
			"  leo kv greeting\n" +
			"  leo kv -f greeting hi\n" +
			"  leo kv --string zip 7001\n" +
			"  leo kv --query '.cpu' limits\n" +
			"  pbpaste | leo kv --secret api.token -    # keeps the secret out of shell history\n" +
			"  leo kv --secret api.token                # mark an existing key secret\n" +
			"  leo kv -s api\n" +
			"  leo kv -d greeting",
		GroupID: groupBuiltin,
		Args:    cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			var mode storeMode
			switch {
			case o.del && o.search:
				return fmt.Errorf("-d and -s can't be used together")
			case o.del:
				if len(args) != 1 {
					return fmt.Errorf("-d takes exactly one key: leo kv -d <key>")
				}
				mode = modeDelete
			case o.search:
				if len(args) != 1 {
					return fmt.Errorf("-s takes one search term (quote it if it has spaces): leo kv -s <text>")
				}
				mode = modeSearch
			case len(args) == 0:
				mode = modeList
			case len(args) == 2 || o.file != "":
				mode = modeSet
			case cmd.Flags().Changed("secret"):
				mode = modeMark
			default:
				mode = modeGet
			}
			if err := checkStoreFlags(cmd, mode); err != nil {
				return err
			}

			st, err := store.Open(cfg.StorePath)
			if err != nil {
				return err
			}
			switch mode {
			case modeList:
				return printEntries(cmd.OutOrStdout(), st, st.Keys(), o.query, compact)
			case modeSearch:
				return printEntries(cmd.OutOrStdout(), st, st.Search(args[0]), o.query, preview)
			case modeGet:
				return storeGet(cmd, st, args[0], o)
			case modeSet:
				return storeSet(cmd, st, args, o)
			case modeMark:
				return storeMark(cmd, st, args[0], o.secret)
			default:
				return storeDelete(cmd, st, args[0])
			}
		},
	}
	f := cmd.Flags()
	f.BoolVarP(&o.del, "delete", "d", false, "delete the key")
	f.BoolVarP(&o.search, "search", "s", false, "search keys and values, best matches first")
	f.BoolVarP(&o.force, "force", "f", false, "replace an existing value without asking")
	f.BoolVar(&o.secret, "secret", false, "mark the value secret so it lists as SECRET (--secret=false unmarks)")
	f.BoolVar(&o.asString, "string", false, "store the value as a string")
	f.BoolVar(&o.asJSON, "json", false, "require the value to be valid JSON")
	f.StringVar(&o.file, "file", "", "read the value from a file")
	f.StringVar(&o.query, "query", "", "jq expression to project the value (each value when listing or searching)")
	f.BoolVarP(&o.raw, "raw", "r", false, "unquote string query results")
	f.BoolVarP(&o.compactOut, "compact", "c", false, "single-line output")
	return cmd
}

// checkStoreFlags rejects mode-specific flags that don't apply to mode.
func checkStoreFlags(cmd *cobra.Command, mode storeMode) error {
	for _, fm := range storeFlagModes {
		if !cmd.Flags().Changed(fm.name) || slices.Contains(fm.modes, mode) {
			continue
		}
		switch mode {
		case modeDelete:
			return fmt.Errorf("--%s can't be combined with -d", fm.name)
		case modeSearch:
			return fmt.Errorf("--%s can't be combined with -s", fm.name)
		}
		return fmt.Errorf("--%s only applies %s", fm.name, fm.where)
	}
	return nil
}

// printEntries prints one "key<TAB>value" line per key, rendering values with
// render, or one line per --query result. Secrets print as SECRET and are
// never queried, since query output (or a query error) could reveal them.
func printEntries(w io.Writer, st *store.Store, keys []string, query string, render func(json.RawMessage) string) error {
	for _, k := range keys {
		if st.IsSecret(k) {
			fmt.Fprintf(w, "%s\t%s\n", k, secretMask)
			continue
		}
		val, _ := st.Get(k)
		if query == "" {
			fmt.Fprintf(w, "%s\t%s\n", k, render(val))
			continue
		}
		results, err := store.Query(val, query)
		if err != nil {
			return err
		}
		for _, r := range results {
			fmt.Fprintf(w, "%s\t%s\n", k, compact(r))
		}
	}
	return nil
}

func storeGet(cmd *cobra.Command, st *store.Store, key string, o storeOpts) error {
	val, ok := st.Get(key)
	if !ok {
		return fmt.Errorf("key not found: %s", key)
	}
	w := cmd.OutOrStdout()
	if o.query != "" {
		results, err := store.Query(val, o.query)
		if err != nil {
			return err
		}
		return printResults(w, results, o.raw, o.compactOut)
	}
	// Bare get: top-level strings print unquoted; others pretty-print (or
	// compact with -c).
	if store.Kind(val) == "string" {
		fmt.Fprintln(w, store.UnquoteString(val))
		return nil
	}
	if o.compactOut {
		fmt.Fprintln(w, compact(val))
		return nil
	}
	fmt.Fprintln(w, pretty(val))
	return nil
}

func storeSet(cmd *cobra.Command, st *store.Store, args []string, o storeOpts) error {
	key := args[0]

	var input string
	fromStdin := false
	switch {
	case o.file != "":
		b, err := os.ReadFile(o.file)
		if err != nil {
			return err
		}
		input = string(b)
	case args[1] == "-":
		b, err := io.ReadAll(cmd.InOrStdin())
		if err != nil {
			return err
		}
		input = strings.TrimRight(string(b), "\n")
		fromStdin = true
	default:
		input = args[1]
	}

	value, err := store.DetectValue(input, o.asString, o.asJSON)
	if err != nil {
		return err
	}

	// An existing secret stays secret unless --secret says otherwise.
	wasSecret := st.IsSecret(key)
	secret := wasSecret
	if cmd.Flags().Changed("secret") {
		secret = o.secret
	}

	w := cmd.OutOrStdout()
	if cur, exists := st.Get(key); exists {
		if compact(cur) == compact(value) {
			if secret == wasSecret {
				fmt.Fprintf(w, "%s unchanged (same value)\n", key)
				return nil
			}
			return storeMark(cmd, st, key, secret)
		}
		if !o.force {
			ok, err := confirmOverwrite(cmd, key, display(cur, wasSecret), display(value, secret), fromStdin)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintf(w, "%s unchanged\n", key)
				return nil
			}
		}
	}

	old, existed, err := st.Set(key, value, secret)
	if err != nil {
		return err
	}
	kind := store.Kind(value)
	if secret {
		kind += ", secret"
	}
	if existed {
		fmt.Fprintf(w, "set %s (%s); previous value: %s\n", key, kind, display(old, wasSecret))
	} else {
		fmt.Fprintf(w, "set %s (%s)\n", key, kind)
	}
	return nil
}

// display renders a value for confirmations and prompts: masked when secret,
// otherwise a compact, length-limited preview.
func display(v json.RawMessage, secret bool) string {
	if secret {
		return secretMask
	}
	return preview(v)
}

// confirmOverwrite asks before replacing an existing value. Without a terminal
// to ask on (piped/scripted input, or the value itself came from stdin) it
// refuses rather than guessing; --force skips the question.
func confirmOverwrite(cmd *cobra.Command, key, cur, next string, fromStdin bool) (bool, error) {
	in := cmd.InOrStdin()
	if fromStdin || !isTerminal(in) {
		return false, fmt.Errorf("%s already exists; use -f to replace it", key)
	}
	w := cmd.ErrOrStderr()
	fmt.Fprintf(w, "%s already exists: %s\n", key, cur)
	ans, _ := prompt(bufio.NewReader(in), w, fmt.Sprintf("update it to %s? [y/N]", next), "")
	return isYes(ans), nil
}

// storeMark marks an existing key secret (or not) without touching its value.
func storeMark(cmd *cobra.Command, st *store.Store, key string, secret bool) error {
	changed, err := st.MarkSecret(key, secret)
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	switch {
	case secret && changed:
		fmt.Fprintf(w, "%s is now secret\n", key)
	case secret:
		fmt.Fprintf(w, "%s is already secret\n", key)
	case changed:
		fmt.Fprintf(w, "%s is no longer secret\n", key)
	default:
		fmt.Fprintf(w, "%s isn't secret\n", key)
	}
	return nil
}

func storeDelete(cmd *cobra.Command, st *store.Store, key string) error {
	deleted, err := st.Delete(key)
	if err != nil {
		return err
	}
	if !deleted {
		return fmt.Errorf("key not found: %s", key)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", key)
	return nil
}

// printResults prints jq query results: --raw unquotes string results, --compact
// forces single-line, otherwise values are pretty-printed.
func printResults(w io.Writer, results []json.RawMessage, raw, compactOut bool) error {
	for _, r := range results {
		switch {
		case raw && store.Kind(r) == "string":
			fmt.Fprintln(w, store.UnquoteString(r))
		case compactOut:
			fmt.Fprintln(w, compact(r))
		default:
			fmt.Fprintln(w, pretty(r))
		}
	}
	return nil
}

// compact returns single-line JSON.
func compact(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// pretty returns indented JSON.
func pretty(raw json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

// preview returns a compact, length-limited rendering of a value.
func preview(raw json.RawMessage) string {
	s := compact(raw)
	const max = 80
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
