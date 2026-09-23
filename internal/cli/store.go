package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"leo/internal/config"
	"leo/internal/store"
)

func newStoreCmd(cfg *config.Config) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "store",
		Short:   "Object store for arbitrary JSON values",
		GroupID: groupBuiltin,
		Args:    cobra.NoArgs,
	}
	cmd.AddCommand(
		newStoreSetCmd(cfg),
		newStoreGetCmd(cfg),
		newStoreSearchCmd(cfg),
		newStoreTypeCmd(cfg),
		newStoreDeleteCmd(cfg),
		newStoreListCmd(cfg),
	)
	return cmd
}

func newStoreSetCmd(cfg *config.Config) *cobra.Command {
	var asString, asJSON bool
	var file string
	cmd := &cobra.Command{
		Use:   "set <key> [value]",
		Short: "Set a key (JSON auto-typed; --string/--json override; --file/- read input)",
		Args:  cobra.RangeArgs(1, 2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 0 {
				return storeKeyCompletion(cfg)(nil, args, toComplete)
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]

			var input string
			switch {
			case file != "":
				b, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				input = string(b)
			case len(args) == 2 && args[1] == "-":
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return err
				}
				input = strings.TrimRight(string(b), "\n")
			case len(args) == 2:
				input = args[1]
			default:
				return fmt.Errorf("provide a value, --file <path>, or - to read stdin")
			}

			value, err := store.DetectValue(input, asString, asJSON)
			if err != nil {
				return err
			}
			st, err := store.Open(cfg.StorePath)
			if err != nil {
				return err
			}
			old, existed, err := st.Set(key, value)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			if existed {
				fmt.Fprintf(w, "set %s (%s); previous value: %s\n", key, store.Kind(value), compact(old))
			} else {
				fmt.Fprintf(w, "set %s (%s)\n", key, store.Kind(value))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&asString, "string", false, "store the value as a string")
	cmd.Flags().BoolVar(&asJSON, "json", false, "require the value to be valid JSON")
	cmd.Flags().StringVar(&file, "file", "", "read the value from a file")
	return cmd
}

func newStoreGetCmd(cfg *config.Config) *cobra.Command {
	var query string
	var raw, compactOut bool
	cmd := &cobra.Command{
		Use:               "get <key>",
		Short:             "Get a key (top-level strings print unquoted; --query runs jq)",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: storeKeyCompletion(cfg),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open(cfg.StorePath)
			if err != nil {
				return err
			}
			val, ok := st.Get(args[0])
			if !ok {
				return fmt.Errorf("key not found: %s", args[0])
			}
			w := cmd.OutOrStdout()
			if query != "" {
				results, err := store.Query(val, query)
				if err != nil {
					return err
				}
				return printResults(w, results, raw, compactOut)
			}
			// Bare get: top-level strings print unquoted; others pretty-print
			// (or compact with -c).
			if store.Kind(val) == "string" {
				fmt.Fprintln(w, store.UnquoteString(val))
				return nil
			}
			if compactOut {
				fmt.Fprintln(w, compact(val))
				return nil
			}
			fmt.Fprintln(w, pretty(val))
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "jq expression to project the value")
	cmd.Flags().BoolVarP(&raw, "raw", "r", false, "unquote string results")
	cmd.Flags().BoolVarP(&compactOut, "compact", "c", false, "single-line output")
	return cmd
}

func newStoreSearchCmd(cfg *config.Config) *cobra.Command {
	var values bool
	var query string
	cmd := &cobra.Command{
		Use:   "search <substr>",
		Short: "Find keys by substring (--values also matches value text)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open(cfg.StorePath)
			if err != nil {
				return err
			}
			needle := strings.ToLower(args[0])
			w := cmd.OutOrStdout()
			for _, k := range st.Keys() {
				val, _ := st.Get(k)
				match := strings.Contains(strings.ToLower(k), needle)
				if !match && values {
					match = strings.Contains(strings.ToLower(string(val)), needle)
				}
				if !match {
					continue
				}
				if query != "" {
					results, err := store.Query(val, query)
					if err != nil {
						return err
					}
					for _, r := range results {
						fmt.Fprintf(w, "%s\t%s\n", k, compact(r))
					}
					continue
				}
				fmt.Fprintf(w, "%s\t%s\n", k, preview(val))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&values, "values", false, "also match against value text")
	cmd.Flags().StringVar(&query, "query", "", "jq expression to project each match")
	return cmd
}

func newStoreTypeCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:               "type <key>",
		Short:             "Print a key's JSON kind (object|array|string|number|boolean|null)",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: storeKeyCompletion(cfg),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open(cfg.StorePath)
			if err != nil {
				return err
			}
			val, ok := st.Get(args[0])
			if !ok {
				return fmt.Errorf("key not found: %s", args[0])
			}
			fmt.Fprintln(cmd.OutOrStdout(), store.Kind(val))
			return nil
		},
	}
}

func newStoreDeleteCmd(cfg *config.Config) *cobra.Command {
	return &cobra.Command{
		Use:               "delete <key>",
		Short:             "Delete a key",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: storeKeyCompletion(cfg),
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := store.Open(cfg.StorePath)
			if err != nil {
				return err
			}
			deleted, err := st.Delete(args[0])
			if err != nil {
				return err
			}
			if !deleted {
				return fmt.Errorf("key not found: %s", args[0])
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", args[0])
			return nil
		},
	}
}

func newStoreListCmd(cfg *config.Config) *cobra.Command {
	var query string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all key/value pairs (optionally projecting via --query)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := store.Open(cfg.StorePath)
			if err != nil {
				return err
			}
			w := cmd.OutOrStdout()
			for _, k := range st.Keys() {
				val, _ := st.Get(k)
				if query != "" {
					results, err := store.Query(val, query)
					if err != nil {
						return err
					}
					for _, r := range results {
						fmt.Fprintf(w, "%s\t%s\n", k, compact(r))
					}
					continue
				}
				fmt.Fprintf(w, "%s\t%s\n", k, compact(val))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "jq expression to project each value")
	return cmd
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

// preview returns a compact, length-limited rendering for search output.
func preview(raw json.RawMessage) string {
	s := compact(raw)
	const max = 80
	if len(s) > max {
		return s[:max-3] + "..."
	}
	return s
}
