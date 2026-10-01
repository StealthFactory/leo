package cli

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"leo/internal/config"
)

// runStore runs `leo kv` with args against cfg's store.
func runStore(t *testing.T, cfg *config.Config, stdin string, args ...string) (string, error) {
	t.Helper()
	return runCmd(t, newStoreCmd(cfg), stdin, args...)
}

// fakeTTY makes the overwrite prompt treat stdin as an interactive terminal
// (or not) for the rest of the test.
func fakeTTY(t *testing.T, tty bool) {
	t.Helper()
	orig := isTerminal
	isTerminal = func(io.Reader) bool { return tty }
	t.Cleanup(func() { isTerminal = orig })
}

func TestStoreSetAutoTypingAndGet(t *testing.T) {
	cfg := testCfg(t)

	cases := []struct {
		args    []string
		wantSet string // expected set confirmation
		getKey  string
		wantGet string // expected bare get output
	}{
		{[]string{"url", "https://x.gif"}, "set url (string)\n", "url", "https://x.gif\n"},
		{[]string{"retries", "5"}, "set retries (number)\n", "retries", "5\n"},
		{[]string{"flag", "true"}, "set flag (boolean)\n", "flag", "true\n"},
		{[]string{"--string", "zip", "7001"}, "set zip (string)\n", "zip", "7001\n"},
		{[]string{"zip2", "--string", "7002"}, "set zip2 (string)\n", "zip2", "7002\n"}, // flags after args still parse
	}
	for _, c := range cases {
		if out, err := runStore(t, cfg, "", c.args...); err != nil || out != c.wantSet {
			t.Fatalf("%v: out=%q err=%v, want %q", c.args, out, err, c.wantSet)
		}
		if out, err := runStore(t, cfg, "", c.getKey); err != nil || out != c.wantGet {
			t.Errorf("get %s: out=%q err=%v, want %q", c.getKey, out, err, c.wantGet)
		}
	}
}

func TestStoreQuery(t *testing.T) {
	cfg := testCfg(t)
	if _, err := runStore(t, cfg, "", "limits", `{"cpu":2,"mem":"4Gi"}`); err != nil {
		t.Fatal(err)
	}
	if out, err := runStore(t, cfg, "", "--query", ".cpu", "limits"); err != nil || out != "2\n" {
		t.Errorf("query .cpu = %q err=%v, want 2", out, err)
	}
	if out, _ := runStore(t, cfg, "", "--raw", "--query", ".mem", "limits"); out != "4Gi\n" {
		t.Errorf("query .mem --raw = %q, want 4Gi", out)
	}
	if out, _ := runStore(t, cfg, "", "-r", "--query", "type", "limits"); out != "object\n" {
		t.Errorf("query type = %q, want object", out)
	}
	if out, _ := runStore(t, cfg, "", "-c", "limits"); out != `{"cpu":2,"mem":"4Gi"}`+"\n" {
		t.Errorf("-c = %q", out)
	}
}

func TestStoreSetForceJSONInvalid(t *testing.T) {
	cfg := testCfg(t)
	if _, err := runStore(t, cfg, "", "--json", "x", "not json"); err == nil {
		t.Error("--json on invalid JSON should error")
	}
}

func TestStoreSetFromStdin(t *testing.T) {
	cfg := testCfg(t)
	if out, err := runStore(t, cfg, `{"a":1}`+"\n", "blob", "-"); err != nil || out != "set blob (object)\n" {
		t.Fatalf("stdin set: out=%q err=%v", out, err)
	}
	if out, _ := runStore(t, cfg, "", "-c", "blob"); out != `{"a":1}`+"\n" {
		t.Errorf("get blob = %q", out)
	}
}

func TestStoreSetFromFile(t *testing.T) {
	cfg := testCfg(t)
	f := filepath.Join(t.TempDir(), "v.json")
	if err := os.WriteFile(f, []byte(`[1,2,3]`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := runStore(t, cfg, "", "--file", f, "arr"); err != nil || out != "set arr (array)\n" {
		t.Fatalf("file set: out=%q err=%v", out, err)
	}
}

func TestStoreUpdatePromptsAndConfirms(t *testing.T) {
	cfg := testCfg(t)
	fakeTTY(t, true)
	runStore(t, cfg, "", "foo", "bar")

	out, err := runStore(t, cfg, "y\n", "foo", "baz")
	if err != nil {
		t.Fatalf("confirmed update: %v", err)
	}
	for _, want := range []string{`foo already exists: "bar"`, `update it to "baz"? [y/N]`, `set foo (string); previous value: "bar"`} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if got, _ := runStore(t, cfg, "", "foo"); got != "baz\n" {
		t.Errorf("after update foo = %q, want baz", got)
	}
}

func TestStoreUpdateDeclined(t *testing.T) {
	cfg := testCfg(t)
	fakeTTY(t, true)
	runStore(t, cfg, "", "foo", "bar")

	for _, answer := range []string{"n\n", "\n", "whatever\n"} {
		out, err := runStore(t, cfg, answer, "foo", "baz")
		if err != nil || !strings.HasSuffix(out, "foo unchanged\n") {
			t.Errorf("answer %q: out=%q err=%v", answer, out, err)
		}
	}
	if got, _ := runStore(t, cfg, "", "foo"); got != "bar\n" {
		t.Errorf("declined update changed foo to %q", got)
	}
}

func TestStoreUpdateNonInteractiveNeedsForce(t *testing.T) {
	cfg := testCfg(t)
	fakeTTY(t, false)
	runStore(t, cfg, "", "foo", "bar")

	if _, err := runStore(t, cfg, "y\n", "foo", "baz"); err == nil || !strings.Contains(err.Error(), "-f") {
		t.Errorf("non-interactive update should refuse and mention -f, got %v", err)
	}
	if out, err := runStore(t, cfg, "", "-f", "foo", "baz"); err != nil || !strings.Contains(out, "set foo (string)") {
		t.Errorf("-f update: out=%q err=%v", out, err)
	}
	if got, _ := runStore(t, cfg, "", "foo"); got != "baz\n" {
		t.Errorf("after -f foo = %q, want baz", got)
	}
}

func TestStoreUpdateFromStdinNeedsForce(t *testing.T) {
	cfg := testCfg(t)
	fakeTTY(t, true) // even on a terminal, stdin is spent on the value
	runStore(t, cfg, "", "foo", "bar")

	if _, err := runStore(t, cfg, "baz\n", "foo", "-"); err == nil {
		t.Error("stdin update without -f should refuse")
	}
	if _, err := runStore(t, cfg, "baz\n", "--force", "foo", "-"); err != nil {
		t.Errorf("stdin update with --force: %v", err)
	}
}

func TestStoreSetSameValueSkipsPrompt(t *testing.T) {
	cfg := testCfg(t)
	fakeTTY(t, false)
	runStore(t, cfg, "", "limits", `{"cpu":2}`)
	if out, err := runStore(t, cfg, "", "limits", `{ "cpu": 2 }`); err != nil || out != "limits unchanged (same value)\n" {
		t.Errorf("same value: out=%q err=%v", out, err)
	}
}

func TestStoreDelete(t *testing.T) {
	cfg := testCfg(t)
	runStore(t, cfg, "", "temp", "1")
	if out, err := runStore(t, cfg, "", "-d", "temp"); err != nil || out != "deleted temp\n" {
		t.Fatalf("delete: out=%q err=%v", out, err)
	}
	if _, err := runStore(t, cfg, "", "-d", "temp"); err == nil {
		t.Error("deleting a missing key should error")
	}
	if _, err := runStore(t, cfg, "", "-d"); err == nil {
		t.Error("-d without a key should error")
	}
	if _, err := runStore(t, cfg, "", "-d", "a", "b"); err == nil {
		t.Error("-d with a value should error")
	}
}

func TestStoreGetMissing(t *testing.T) {
	cfg := testCfg(t)
	if _, err := runStore(t, cfg, "", "ghost"); err == nil {
		t.Error("get on a missing key should error")
	}
}

func TestStoreKeysNamedLikeOldSubcommands(t *testing.T) {
	cfg := testCfg(t)
	for _, k := range []string{"set", "get", "list", "delete", "search", "type", "help"} {
		if _, err := runStore(t, cfg, "", k, "v-"+k); err != nil {
			t.Fatalf("set %s: %v", k, err)
		}
		if out, err := runStore(t, cfg, "", k); err != nil || out != "v-"+k+"\n" {
			t.Errorf("get %s: out=%q err=%v", k, out, err)
		}
	}
}

func TestStoreMetaKeyReserved(t *testing.T) {
	cfg := testCfg(t)
	runStore(t, cfg, "", "--secret", "token", "x")
	for _, args := range [][]string{{"$leo", "x"}, {"--secret", "$leo"}, {"-d", "$leo"}, {"$leo"}} {
		if _, err := runStore(t, cfg, "", args...); err == nil {
			t.Errorf("%v should error", args)
		}
	}
	if out, _ := runStore(t, cfg, "", "-s", "leo"); out != "" {
		t.Errorf("metadata must not show up in search: %q", out)
	}
}

func TestStoreList(t *testing.T) {
	cfg := testCfg(t)
	if out, err := runStore(t, cfg, ""); err != nil || out != "" {
		t.Errorf("empty list: out=%q err=%v", out, err)
	}
	runStore(t, cfg, "", "dancegif", "https://giphy.com/x.gif")
	runStore(t, cfg, "", "retries", "5")

	want := "dancegif\t\"https://giphy.com/x.gif\"\nretries\t5\n"
	if out, err := runStore(t, cfg, ""); err != nil || out != want {
		t.Errorf("list: out=%q err=%v, want %q", out, err, want)
	}
	if out, _ := runStore(t, cfg, "", "--query", "type"); out != "dancegif\t\"string\"\nretries\t\"number\"\n" {
		t.Errorf("list --query type = %q", out)
	}
}

func TestStoreRejectsMisplacedFlags(t *testing.T) {
	cfg := testCfg(t)
	runStore(t, cfg, "", "foo", "bar")
	cases := [][]string{
		{"--string", "foo"},          // set-only flag on a get
		{"-f", "foo"},                // force without a value
		{"--query", ".", "foo", "x"}, // query on a set
		{"-r"},                       // raw on a list
		{"-d", "--force", "foo"},     // extra flag with delete
		{"a", "b", "c"},              // too many args
	}
	for _, args := range cases {
		if _, err := runStore(t, cfg, "", args...); err == nil {
			t.Errorf("%v should error", args)
		}
	}
	if got, _ := runStore(t, cfg, "", "foo"); got != "bar\n" {
		t.Errorf("rejected commands changed foo to %q", got)
	}
}

func TestStoreSecretMaskedInListButReadable(t *testing.T) {
	cfg := testCfg(t)
	runStore(t, cfg, "", "plain", "hi")
	if out, err := runStore(t, cfg, "", "--secret", "token", "hunter2"); err != nil || out != "set token (string, secret)\n" {
		t.Fatalf("secret set: out=%q err=%v", out, err)
	}

	if out, _ := runStore(t, cfg, ""); out != "plain\t\"hi\"\ntoken\tSECRET\n" {
		t.Errorf("list = %q", out)
	}
	// Queries never run on secrets: on "hunter2", .[] would fail with an error
	// quoting the value.
	runStore(t, cfg, "", "-d", "plain")
	if out, err := runStore(t, cfg, "", "--query", ".[]"); err != nil || out != "token\tSECRET\n" {
		t.Errorf("list --query = %q err=%v", out, err)
	}
	// An explicit get returns the real value.
	if out, _ := runStore(t, cfg, "", "token"); out != "hunter2\n" {
		t.Errorf("get token = %q", out)
	}
}

func TestStoreSecretMaskedInPromptAndConfirmation(t *testing.T) {
	cfg := testCfg(t)
	fakeTTY(t, true)
	runStore(t, cfg, "", "--secret", "token", "hunter2")

	out, err := runStore(t, cfg, "y\n", "token", "swordfish")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "hunter2") || strings.Contains(out, "swordfish") {
		t.Errorf("secret leaked in output:\n%s", out)
	}
	for _, want := range []string{"token already exists: SECRET", "update it to SECRET? [y/N]", "set token (string, secret); previous value: SECRET"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	// Updating without --secret keeps the key secret.
	if list, _ := runStore(t, cfg, ""); list != "token\tSECRET\n" {
		t.Errorf("update should keep the secret mark, list = %q", list)
	}
}

func TestStoreSecretMarkAndUnmark(t *testing.T) {
	cfg := testCfg(t)
	runStore(t, cfg, "", "token", "hunter2")

	steps := []struct {
		args []string
		want string
	}{
		{[]string{"--secret", "token"}, "token is now secret\n"},
		{[]string{"--secret", "token"}, "token is already secret\n"},
		{[]string{"--secret=false", "token"}, "token is no longer secret\n"},
		{[]string{"--secret=false", "token"}, "token isn't secret\n"},
		{[]string{"--secret", "token", "hunter2"}, "token is now secret\n"}, // same value: just re-marks, no prompt
		{[]string{"-f", "--secret=false", "token", "new"}, "set token (string); previous value: SECRET\n"},
	}
	for _, s := range steps {
		if out, err := runStore(t, cfg, "", s.args...); err != nil || out != s.want {
			t.Errorf("%v: out=%q err=%v, want %q", s.args, out, err, s.want)
		}
	}
	if list, _ := runStore(t, cfg, ""); list != "token\t\"new\"\n" {
		t.Errorf("list = %q", list)
	}
	if _, err := runStore(t, cfg, "", "--secret", "ghost"); err == nil {
		t.Error("marking a missing key should error")
	}
}

func TestStoreSearch(t *testing.T) {
	cfg := testCfg(t)
	runStore(t, cfg, "", "deploy.host", "example.com")
	runStore(t, cfg, "", "deploy", "yes")
	runStore(t, cfg, "", "hosts", `["api","worker"]`)
	runStore(t, cfg, "", "--secret", "api.token", "hunter2")
	runStore(t, cfg, "", "unrelated", "1")

	want := "api.token\tSECRET\nhosts\t[\"api\",\"worker\"]\n"
	if out, err := runStore(t, cfg, "", "-s", "api"); err != nil || out != want {
		t.Errorf("-s api: out=%q err=%v, want %q", out, err, want)
	}
	if out, _ := runStore(t, cfg, "", "--search", "dep"); out != "deploy\t\"yes\"\ndeploy.host\t\"example.com\"\n" {
		t.Errorf("--search dep = %q", out)
	}
	if out, _ := runStore(t, cfg, "", "-s", "host"); out != "hosts\t[\"api\",\"worker\"]\ndeploy.host\t\"example.com\"\n" {
		t.Errorf("-s host (prefix before word start) = %q", out)
	}
	if out, _ := runStore(t, cfg, "", "-s", "hunter"); out != "" {
		t.Errorf("secret values must not be searched: %q", out)
	}
	if out, _ := runStore(t, cfg, "", "--query", ".[0]", "-s", "hosts"); out != "hosts\t\"api\"\n" {
		t.Errorf("-s --query = %q", out)
	}
	for _, args := range [][]string{{"-s"}, {"-s", "a", "b"}, {"-s", "-d", "api"}, {"-s", "-r", "api"}, {"-s", "--secret", "api"}} {
		if _, err := runStore(t, cfg, "", args...); err == nil {
			t.Errorf("%v should error", args)
		}
	}
}
