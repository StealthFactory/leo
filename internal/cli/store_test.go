package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"leo/internal/config"
)

// runStore runs a `store` subcommand against cfg's store.
func runStore(t *testing.T, cfg *config.Config, stdin string, args ...string) (string, error) {
	t.Helper()
	return runCmd(t, newStoreCmd(cfg), stdin, args...)
}

func TestStoreSetAutoTypingAndGet(t *testing.T) {
	cfg := testCfg(t)

	cases := []struct {
		args    []string
		wantSet string // expected `set` confirmation
		getKey  string
		wantGet string // expected bare `get` output
	}{
		{[]string{"set", "url", "https://x.gif"}, "set url (string)\n", "url", "https://x.gif\n"},
		{[]string{"set", "retries", "5"}, "set retries (number)\n", "retries", "5\n"},
		{[]string{"set", "flag", "true"}, "set flag (boolean)\n", "flag", "true\n"},
		{[]string{"set", "zip", "--string", "7001"}, "set zip (string)\n", "zip", "7001\n"},
	}
	for _, c := range cases {
		if out, err := runStore(t, cfg, "", c.args...); err != nil || out != c.wantSet {
			t.Fatalf("%v: out=%q err=%v, want %q", c.args, out, err, c.wantSet)
		}
		if out, err := runStore(t, cfg, "", "get", c.getKey); err != nil || out != c.wantGet {
			t.Errorf("get %s: out=%q err=%v, want %q", c.getKey, out, err, c.wantGet)
		}
	}
}

func TestStoreTypeAndQuery(t *testing.T) {
	cfg := testCfg(t)
	if _, err := runStore(t, cfg, "", "set", "limits", `{"cpu":2,"mem":"4Gi"}`); err != nil {
		t.Fatal(err)
	}
	if out, _ := runStore(t, cfg, "", "type", "limits"); out != "object\n" {
		t.Errorf("type = %q, want object", out)
	}
	if out, err := runStore(t, cfg, "", "get", "limits", "--query", ".cpu"); err != nil || out != "2\n" {
		t.Errorf("query .cpu = %q err=%v, want 2", out, err)
	}
	if out, _ := runStore(t, cfg, "", "get", "limits", "--query", ".mem", "--raw"); out != "4Gi\n" {
		t.Errorf("query .mem --raw = %q, want 4Gi", out)
	}
}

func TestStoreSetForceJSONInvalid(t *testing.T) {
	cfg := testCfg(t)
	if _, err := runStore(t, cfg, "", "set", "x", "not json", "--json"); err == nil {
		t.Error("--json on invalid JSON should error")
	}
}

func TestStoreSetRequiresValue(t *testing.T) {
	cfg := testCfg(t)
	if _, err := runStore(t, cfg, "", "set", "lonely"); err == nil {
		t.Error("set with no value/--file/stdin should error")
	}
}

func TestStoreSetFromStdin(t *testing.T) {
	cfg := testCfg(t)
	if out, err := runStore(t, cfg, `{"a":1}`+"\n", "set", "blob", "-"); err != nil || out != "set blob (object)\n" {
		t.Fatalf("stdin set: out=%q err=%v", out, err)
	}
	if out, _ := runStore(t, cfg, "", "type", "blob"); out != "object\n" {
		t.Errorf("type blob = %q, want object", out)
	}
}

func TestStoreSetFromFile(t *testing.T) {
	cfg := testCfg(t)
	f := filepath.Join(t.TempDir(), "v.json")
	if err := os.WriteFile(f, []byte(`[1,2,3]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runStore(t, cfg, "", "set", "arr", "--file", f); err != nil || out != "set arr (array)\n" {
		t.Fatalf("file set: out=%q err=%v", out, err)
	}
}

func TestStoreDelete(t *testing.T) {
	cfg := testCfg(t)
	runStore(t, cfg, "", "set", "temp", "1")
	if out, err := runStore(t, cfg, "", "delete", "temp"); err != nil || out != "deleted temp\n" {
		t.Fatalf("delete: out=%q err=%v", out, err)
	}
	if _, err := runStore(t, cfg, "", "delete", "temp"); err == nil {
		t.Error("deleting a missing key should error")
	}
}

func TestStoreGetMissing(t *testing.T) {
	cfg := testCfg(t)
	if _, err := runStore(t, cfg, "", "get", "ghost"); err == nil {
		t.Error("get on a missing key should error")
	}
}

func TestStoreListAndSearch(t *testing.T) {
	cfg := testCfg(t)
	runStore(t, cfg, "", "set", "dancegif", "https://giphy.com/x.gif")
	runStore(t, cfg, "", "set", "retries", "5")

	list, _ := runStore(t, cfg, "", "list")
	if !strings.Contains(list, "dancegif\t") || !strings.Contains(list, "retries\t5") {
		t.Errorf("list missing entries:\n%s", list)
	}

	// key-substring search
	sr, _ := runStore(t, cfg, "", "search", "danc")
	if !strings.Contains(sr, "dancegif") || strings.Contains(sr, "retries") {
		t.Errorf("search danc = %q", sr)
	}

	// value search only with --values
	if out, _ := runStore(t, cfg, "", "search", "giphy"); strings.Contains(out, "dancegif") {
		t.Errorf("search without --values should not match value text: %q", out)
	}
	if out, _ := runStore(t, cfg, "", "search", "giphy", "--values"); !strings.Contains(out, "dancegif") {
		t.Errorf("search --values should match value text: %q", out)
	}
}
