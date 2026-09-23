package store

import (
	"encoding/json"
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
	if _, _, err := s.Set("limits", val); err != nil {
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
	if _, existed, _ := s.Set("k", json.RawMessage(`1`)); existed {
		t.Error("first set should not report existed")
	}
	old, existed, _ := s.Set("k", json.RawMessage(`2`))
	if !existed || string(old) != `1` {
		t.Errorf("second set old=%s existed=%v, want 1/true", old, existed)
	}
}

func TestDeletePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s, _ := Open(path)
	s.Set("secret", json.RawMessage(`"shh"`))

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

func TestRankKeysOrdering(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.json")
	s, _ := Open(path)
	for _, k := range []string{"food", "format", "before", "flow", "unrelated"} {
		s.Set(k, json.RawMessage(`1`))
	}
	// "fo": prefix {food, format}, substring {before}, subsequence {flow}.
	got := s.RankKeys("fo")
	want := []string{"food", "format", "before", "flow"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RankKeys(fo) = %v, want %v", got, want)
	}

	// Empty prefix returns every key alphabetically.
	if got := s.RankKeys(""); len(got) != 5 {
		t.Errorf("RankKeys(\"\") returned %d keys, want 5", len(got))
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
