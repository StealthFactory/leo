// Package store implements leo's object store: a JSON object mapping keys to
// arbitrary JSON values, persisted atomically at mode 0600 (values may be
// secrets), plus key ranking for tab-completion and jq querying via gojq.
package store

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/itchyny/gojq"
)

// Store is an in-memory view of the on-disk object store. Values are kept as
// raw JSON so types round-trip exactly.
type Store struct {
	path string
	data map[string]json.RawMessage
}

// Open loads the store at path, returning an empty store if the file does not
// exist yet.
func Open(path string) (*Store, error) {
	s := &Store{path: path, data: map[string]json.RawMessage{}}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, fmt.Errorf("reading store %s: %w", path, err)
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return s, nil
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("parsing store %s: %w", path, err)
	}
	if s.data == nil {
		s.data = map[string]json.RawMessage{}
	}
	return s, nil
}

// save writes the store atomically (temp file + rename) at mode 0600, pretty
// printed with sorted keys for clean diffs. The parent dir is created if
// missing.
func (s *Store) save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// MarshalIndent sorts map keys and re-indents embedded RawMessage values.
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	tmp, err := os.CreateTemp(dir, ".store-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we bail before the rename.
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.path)
}

// Get returns the raw JSON value for key and whether it exists.
func (s *Store) Get(key string) (json.RawMessage, bool) {
	v, ok := s.data[key]
	return v, ok
}

// Set stores value under key and persists. It returns the previous value (if
// any) and whether the key already existed.
func (s *Store) Set(key string, value json.RawMessage) (old json.RawMessage, existed bool, err error) {
	old, existed = s.data[key]
	// Store a private copy so later mutation of value can't corrupt the map.
	cp := make(json.RawMessage, len(value))
	copy(cp, value)
	s.data[key] = cp
	if err := s.save(); err != nil {
		return old, existed, err
	}
	return old, existed, nil
}

// Delete removes key and persists. It reports whether the key existed.
func (s *Store) Delete(key string) (bool, error) {
	if _, ok := s.data[key]; !ok {
		return false, nil
	}
	delete(s.data, key)
	return true, s.save()
}

// Keys returns all keys sorted alphabetically.
func (s *Store) Keys() []string {
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// All returns a copy of every key/value pair (used by `store list`).
func (s *Store) All() map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(s.data))
	for k, v := range s.data {
		out[k] = v
	}
	return out
}

// Kind reports the JSON kind of a raw value: object, array, string, number,
// boolean, or null.
func Kind(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "null"
	}
	switch trimmed[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// DetectValue turns raw user input into a stored JSON value.
//
//   - forceString: always store input as a JSON string.
//   - forceJSON:   input must be valid JSON, or an error is returned.
//   - default:     valid JSON is stored as that type; otherwise a JSON string.
//
// URLs, "1.2.3", "07001", "+1555…" fail JSON parsing and stay strings;
// {...}, [...], 42, true, null are typed. Quoting is the escape hatch: the
// shell-quoted input `"42"` is valid JSON and stays the string "42".
func DetectValue(input string, forceString, forceJSON bool) (json.RawMessage, error) {
	if forceString && forceJSON {
		return nil, fmt.Errorf("--string and --json are mutually exclusive")
	}
	if forceString {
		b, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		return json.RawMessage(b), nil
	}
	if json.Valid([]byte(input)) {
		var buf bytes.Buffer
		if err := json.Compact(&buf, []byte(input)); err != nil {
			return nil, err
		}
		return json.RawMessage(buf.Bytes()), nil
	}
	if forceJSON {
		return nil, fmt.Errorf("value is not valid JSON")
	}
	b, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// RankKeys returns keys matching prefix, ranked case-insensitively:
//  1. prefix match, 2. substring match, 3. subsequence match (chars in order,
//     not necessarily contiguous). Alphabetical within each tier; tiers are
//     concatenated. An empty prefix returns every key (all are prefix matches).
func (s *Store) RankKeys(prefix string) []string {
	keys := s.Keys() // already alphabetical
	p := strings.ToLower(prefix)

	var pref, sub, subseq []string
	for _, k := range keys {
		lk := strings.ToLower(k)
		switch {
		case strings.HasPrefix(lk, p):
			pref = append(pref, k)
		case strings.Contains(lk, p):
			sub = append(sub, k)
		case isSubsequence(p, lk):
			subseq = append(subseq, k)
		}
	}
	out := make([]string, 0, len(pref)+len(sub)+len(subseq))
	out = append(out, pref...)
	out = append(out, sub...)
	out = append(out, subseq...)
	return out
}

// isSubsequence reports whether every char of needle appears in haystack in
// order (not necessarily contiguously). An empty needle always matches.
func isSubsequence(needle, haystack string) bool {
	if needle == "" {
		return true
	}
	i := 0
	for j := 0; j < len(haystack) && i < len(needle); j++ {
		if haystack[j] == needle[i] {
			i++
		}
	}
	return i == len(needle)
}

// Query runs a gojq expression over input and returns each result as compacted
// JSON. A query yielding a bare string is returned quoted; callers may unquote
// with UnquoteString when --raw is requested.
func Query(input json.RawMessage, expr string) ([]json.RawMessage, error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}
	var v any
	if len(bytes.TrimSpace(input)) > 0 {
		if err := json.Unmarshal(input, &v); err != nil {
			return nil, err
		}
	}
	var out []json.RawMessage
	iter := query.Run(v)
	for {
		res, ok := iter.Next()
		if !ok {
			break
		}
		if err, ok := res.(error); ok {
			return nil, err
		}
		b, err := json.Marshal(res)
		if err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(b))
	}
	return out, nil
}

// UnquoteString returns the unquoted contents of a JSON string value, or the
// raw text unchanged if it is not a JSON string.
func UnquoteString(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	return string(raw)
}
