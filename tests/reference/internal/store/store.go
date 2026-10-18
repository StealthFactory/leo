// Package store implements leo's object store: a JSON object mapping keys to
// arbitrary JSON values, persisted atomically at mode 0600 (values may be
// secrets), plus jq querying via gojq and ranked key search.
//
// leo's own bookkeeping, such as which keys are secret, lives in the same file
// under the reserved MetaKey, so a value and its secret mark are always
// written together.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/itchyny/gojq"
)

// MetaKey is the reserved top-level key holding leo's metadata:
//
//	{"$leo": {"secrets": ["api.token"]}, "api.token": "hunter2"}
//
// It is never exposed as a value and can't be set or deleted.
const MetaKey = "$leo"

// ErrReserved is returned when a caller tries to use MetaKey as a key.
var ErrReserved = errors.New(`"` + MetaKey + `" is reserved for leo's own data; pick another key`)

// Store is an in-memory view of the on-disk object store. Values are kept as
// raw JSON so types round-trip exactly.
type Store struct {
	path    string
	data    map[string]json.RawMessage
	secrets map[string]bool
	// meta holds the MetaKey object, including fields this version doesn't
	// know about, so they survive a save.
	meta map[string]json.RawMessage
}

// Open loads the store at path, returning an empty store if the file does not
// exist yet.
func Open(path string) (*Store, error) {
	s := &Store{
		path:    path,
		data:    map[string]json.RawMessage{},
		secrets: map[string]bool{},
		meta:    map[string]json.RawMessage{},
	}
	b, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
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
	if err := s.loadMeta(); err != nil {
		return nil, fmt.Errorf("parsing store %s: %w", path, err)
	}
	return s, nil
}

// loadMeta moves MetaKey out of the values and reads the secret marks. Bad
// metadata is an error rather than being ignored, so a problem never unmasks
// secrets. Marks for keys no longer in the store are dropped.
func (s *Store) loadMeta() error {
	raw, ok := s.data[MetaKey]
	if !ok {
		return nil
	}
	delete(s.data, MetaKey)
	if err := json.Unmarshal(raw, &s.meta); err != nil || s.meta == nil {
		return fmt.Errorf("%q must be an object of leo metadata", MetaKey)
	}
	var keys []string
	if raw, ok := s.meta["secrets"]; ok {
		if err := json.Unmarshal(raw, &keys); err != nil {
			return fmt.Errorf("%q secrets must be a list of keys", MetaKey)
		}
	}
	for _, k := range keys {
		if _, ok := s.data[k]; ok {
			s.secrets[k] = true
		}
	}
	return nil
}

// save writes values and metadata atomically (temp file + rename) at mode
// 0600, pretty printed with sorted keys for clean diffs. MetaKey is written
// only while there is metadata to keep. The parent dir is created if missing.
func (s *Store) save() error {
	out := make(map[string]json.RawMessage, len(s.data)+1)
	for k, v := range s.data {
		out[k] = v
	}
	if len(s.secrets) > 0 {
		keys := make([]string, 0, len(s.secrets))
		for k := range s.secrets {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b, err := json.Marshal(keys)
		if err != nil {
			return err
		}
		s.meta["secrets"] = b
	} else {
		delete(s.meta, "secrets")
	}
	if len(s.meta) > 0 {
		b, err := json.Marshal(s.meta)
		if err != nil {
			return err
		}
		out[MetaKey] = b
	}
	// MarshalIndent sorts map keys and re-indents embedded RawMessage values.
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
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

// IsSecret reports whether key is marked secret.
func (s *Store) IsSecret(key string) bool {
	return s.secrets[key]
}

// Set stores value under key, marked secret or not, and persists both in one
// write. It returns the previous value (if any) and whether the key already
// existed.
func (s *Store) Set(key string, value json.RawMessage, secret bool) (old json.RawMessage, existed bool, err error) {
	if key == MetaKey {
		return nil, false, ErrReserved
	}
	old, existed = s.data[key]
	// Store a private copy so later mutation of value can't corrupt the map.
	cp := make(json.RawMessage, len(value))
	copy(cp, value)
	s.data[key] = cp
	if secret {
		s.secrets[key] = true
	} else {
		delete(s.secrets, key)
	}
	if err := s.save(); err != nil {
		return old, existed, err
	}
	return old, existed, nil
}

// MarkSecret marks an existing key secret (or not) without changing its value.
// It reports whether the mark changed.
func (s *Store) MarkSecret(key string, secret bool) (bool, error) {
	if key == MetaKey {
		return false, ErrReserved
	}
	if _, ok := s.data[key]; !ok {
		return false, fmt.Errorf("key not found: %s", key)
	}
	if s.secrets[key] == secret {
		return false, nil
	}
	if secret {
		s.secrets[key] = true
	} else {
		delete(s.secrets, key)
	}
	return true, s.save()
}

// Delete removes key and its secret mark and persists. It reports whether the
// key existed.
func (s *Store) Delete(key string) (bool, error) {
	if key == MetaKey {
		return false, ErrReserved
	}
	if _, ok := s.data[key]; !ok {
		return false, nil
	}
	delete(s.data, key)
	delete(s.secrets, key)
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

// All returns a copy of every key/value pair.
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
// URLs, "1.2.3", "07001", "+1555..." fail JSON parsing and stay strings;
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
