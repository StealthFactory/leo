package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDetectValueAutoTyping(t *testing.T) {
	cases := []struct {
		in       string
		wantKind string
		wantRaw  string
	}{
		{`https://giphy.com/x.gif`, "string", `"https://giphy.com/x.gif"`},
		{`1.2.3`, "string", `"1.2.3"`},
		{`07001`, "string", `"07001"`},
		{`+15551234567`, "string", `"+15551234567"`},
		{`hello world`, "string", `"hello world"`},
		{`42`, "number", `42`},
		{`3.14`, "number", `3.14`},
		{`true`, "boolean", `true`},
		{`null`, "null", `null`},
		{`{"cpu":2,"mem":"4Gi"}`, "object", `{"cpu":2,"mem":"4Gi"}`},
		{`[1,2,3]`, "array", `[1,2,3]`},
		{`"42"`, "string", `"42"`}, // shell-quoted escape hatch
	}
	for _, c := range cases {
		got, err := DetectValue(c.in, false, false)
		if err != nil {
			t.Fatalf("DetectValue(%q) error: %v", c.in, err)
		}
		if k := Kind(got); k != c.wantKind {
			t.Errorf("DetectValue(%q) kind = %q, want %q", c.in, k, c.wantKind)
		}
		if string(got) != c.wantRaw {
			t.Errorf("DetectValue(%q) raw = %s, want %s", c.in, got, c.wantRaw)
		}
	}
}

func TestDetectValueForce(t *testing.T) {
	got, err := DetectValue("7001", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `"7001"` {
		t.Errorf("--string got %s, want \"7001\"", got)
	}
	if _, err := DetectValue("not json", false, true); err == nil {
		t.Error("--json on invalid JSON should error")
	}
	if _, err := DetectValue("x", true, true); err == nil {
		t.Error("--string and --json together should error")
	}
}

func TestSetGetRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	val := json.RawMessage(`{"cpu":2,"mem":"4Gi"}`)
	if _, _, err := s.Set("limits", val, false); err != nil {
		t.Fatal(err)
	}

	// Reload from disk and confirm exact round-trip.
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := s2.Get("limits")
	if !ok {
		t.Fatal("limits missing after reload")
	}
	var a, b map[string]any
	json.Unmarshal(got, &a)
	json.Unmarshal(val, &b)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("round-trip mismatch: %s vs %s", got, val)
	}
}

func TestSetReturnsPrevious(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s, _ := Open(path)
	if _, existed, _ := s.Set("k", json.RawMessage(`1`), false); existed {
		t.Error("first set should not report existed")
	}
	old, existed, _ := s.Set("k", json.RawMessage(`2`), false)
	if !existed || string(old) != `1` {
		t.Errorf("second set old=%s existed=%v, want 1/true", old, existed)
	}
}

func TestDeletePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s, _ := Open(path)
	s.Set("secret", json.RawMessage(`"shh"`), false)

	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("store mode = %o, want 600", perm)
	}

	deleted, err := s.Delete("secret")
	if err != nil || !deleted {
		t.Fatalf("Delete = %v, %v", deleted, err)
	}
	if deleted, _ := s.Delete("secret"); deleted {
		t.Error("second delete should report false")
	}
}

func TestQuery(t *testing.T) {
	out, err := Query(json.RawMessage(`{"cpu":2,"mem":"4Gi"}`), ".cpu")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || string(out[0]) != "2" {
		t.Errorf("query got %v, want [2]", out)
	}
}

func TestKind(t *testing.T) {
	cases := map[string]string{
		`{}`:        "object",
		`[]`:        "array",
		`"hi"`:      "string",
		`42`:        "number",
		`-1.5`:      "number",
		`true`:      "boolean",
		`false`:     "boolean",
		`null`:      "null",
		``:          "null",
		`  {"a":1}`: "object",
	}
	for raw, want := range cases {
		if got := Kind(json.RawMessage(raw)); got != want {
			t.Errorf("Kind(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestUnquoteString(t *testing.T) {
	if got := UnquoteString(json.RawMessage(`"hello"`)); got != "hello" {
		t.Errorf("UnquoteString of a string = %q", got)
	}
	// Non-string values come back as their raw text.
	if got := UnquoteString(json.RawMessage(`42`)); got != "42" {
		t.Errorf("UnquoteString of a number = %q", got)
	}
}

// readFileJSON decodes the store file as a plain JSON object, with each value
// compacted.
func readFileJSON(t *testing.T, path string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("store file isn't a JSON object: %v\n%s", err, b)
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		var buf bytes.Buffer
		json.Compact(&buf, v)
		out[k] = buf.String()
	}
	return out
}

func TestSecretsLiveInStoreFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "store.json")

	s, _ := Open(path)
	s.Set("plain", json.RawMessage(`1`), false)
	if _, ok := readFileJSON(t, path)[MetaKey]; ok {
		t.Error("no secrets should mean no metadata key")
	}
	if _, _, err := s.Set("token", json.RawMessage(`"shh"`), true); err != nil {
		t.Fatal(err)
	}

	// Values keep their shape; the secret mark sits under the reserved key.
	m := readFileJSON(t, path)
	if m["token"] != `"shh"` {
		t.Errorf("token = %s", m["token"])
	}
	if m[MetaKey] != `{"secrets":["token"]}` {
		t.Errorf("%s = %s", MetaKey, m[MetaKey])
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("only store.json should exist, got %d entries", len(entries))
	}

	s2, _ := Open(path)
	if !s2.IsSecret("token") || s2.IsSecret("plain") {
		t.Error("secret marks should survive a reload")
	}
	if _, ok := s2.Get(MetaKey); ok || !reflect.DeepEqual(s2.Keys(), []string{"plain", "token"}) {
		t.Errorf("metadata must not appear as a value: keys = %q", s2.Keys())
	}

	if changed, err := s2.MarkSecret("plain", true); !changed || err != nil {
		t.Errorf("MarkSecret(plain) = %v, %v", changed, err)
	}
	if changed, _ := s2.MarkSecret("plain", true); changed {
		t.Error("re-marking should report no change")
	}
	if _, err := s2.MarkSecret("ghost", true); err == nil {
		t.Error("marking a missing key should error")
	}
	if readFileJSON(t, path)[MetaKey] != `{"secrets":["plain","token"]}` {
		t.Errorf("marks should be sorted: %s", readFileJSON(t, path)[MetaKey])
	}

	// Setting without the secret flag unmarks; deleting drops the mark.
	s2.Set("plain", json.RawMessage(`2`), false)
	s2.Delete("token")
	if s2.IsSecret("plain") || s2.IsSecret("token") {
		t.Error("marks should be cleared")
	}
	if _, ok := readFileJSON(t, path)[MetaKey]; ok {
		t.Error("metadata key should be removed once no secrets remain")
	}
}

func TestMetaKeyReserved(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "store.json"))
	s.Set("a", json.RawMessage(`1`), false)
	if _, _, err := s.Set(MetaKey, json.RawMessage(`1`), false); !errors.Is(err, ErrReserved) {
		t.Errorf("Set(%s) err = %v", MetaKey, err)
	}
	if _, err := s.MarkSecret(MetaKey, true); !errors.Is(err, ErrReserved) {
		t.Errorf("MarkSecret(%s) err = %v", MetaKey, err)
	}
	if _, err := s.Delete(MetaKey); !errors.Is(err, ErrReserved) {
		t.Errorf("Delete(%s) err = %v", MetaKey, err)
	}
}

func TestMetaUnknownFieldsPreserved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	os.WriteFile(path, []byte(`{"$leo":{"future":true,"secrets":["a"]},"a":1}`), 0o600)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Delete("a")
	if got := readFileJSON(t, path)[MetaKey]; got != `{"future":true}` {
		t.Errorf("unknown metadata should survive a save, got %s", got)
	}
}

func TestSecretsStaleAndCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")

	// A mark for a key that's gone must not make a later key secret.
	os.WriteFile(path, []byte(`{"$leo":{"secrets":["a","gone"]},"a":1}`), 0o600)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s.IsSecret("a") || s.IsSecret("gone") {
		t.Error("stale mark should be ignored")
	}
	s.Set("gone", json.RawMessage(`2`), false)
	if s.IsSecret("gone") {
		t.Error("re-created key inherited a stale mark")
	}

	// Bad metadata fails closed instead of unmasking everything.
	for _, bad := range []string{`{"$leo":"x","a":1}`, `{"$leo":null,"a":1}`, `{"$leo":{"secrets":"a"},"a":1}`} {
		os.WriteFile(path, []byte(bad), 0o600)
		if _, err := Open(path); err == nil {
			t.Errorf("Open(%s) should error", bad)
		}
	}
}

func TestSearchRanking(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "store.json"))
	for k, v := range map[string]string{
		"api":            `1`,
		"api.token":      `"abc"`,
		"apiary":         `2`,
		"github.api":     `3`,
		"rapid":          `4`,
		"awsPrimaryKey":  `5`,
		"deploy.host":    `"example.com"`,
		"notes":          `"call the api team"`,
		"hidden":         `"api secret"`,
		"unrelated":      `6`,
		"a_long_p_i_key": `7`,
	} {
		s.Set(k, json.RawMessage(v), k == "hidden")
	}

	got := s.Search("API")
	want := []string{
		"api",            // exact
		"api.token",      // prefix (alphabetical within tier)
		"apiary",         // prefix
		"github.api",     // word start
		"rapid",          // substring
		"awsPrimaryKey",  // subsequence a..p..i, tighter span
		"a_long_p_i_key", // subsequence, looser span
		"notes",          // value match; "hidden" is secret so its value isn't searched
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Search(API) =\n  %q\nwant\n  %q", got, want)
	}

	if got := s.Search("dh"); !reflect.DeepEqual(got, []string{"deploy.host"}) {
		t.Errorf("Search(dh) = %q", got)
	}
	if got := s.Search("key"); got[0] != "awsPrimaryKey" {
		t.Errorf("camelCase word start should rank first: %q", got)
	}
	if got := s.Search("example"); !reflect.DeepEqual(got, []string{"deploy.host"}) {
		t.Errorf("value search = %q", got)
	}
	if got := s.Search("hidden"); !reflect.DeepEqual(got, []string{"hidden"}) {
		t.Errorf("secret keys still match by name: %q", got)
	}
	if got := s.Search("zzz"); len(got) != 0 {
		t.Errorf("no match should be empty: %q", got)
	}
}
