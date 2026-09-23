package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"leo/internal/store"
)

func TestResolveClipArg(t *testing.T) {
	dir := t.TempDir()
	realFile := filepath.Join(dir, "logo.png")
	if err := os.WriteFile(realFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(filepath.Join(dir, "store.json"))
	if err != nil {
		t.Fatal(err)
	}
	st.Set("greeting", json.RawMessage(`"hello there"`))
	st.Set("limits", json.RawMessage(`{"cpu":2}`))
	st.Set("logo", json.RawMessage(`"`+realFile+`"`))

	cases := []struct {
		name         string
		arg          string
		wantText     string
		wantIsFile   bool
		wantResolved bool
	}{
		{"string key", "greeting", "hello there", false, true},
		{"json key", "limits", `{"cpu":2}`, false, true},
		{"key naming a file", "logo", realFile, true, true},
		{"literal existing file", realFile, realFile, true, true},
		{"unresolved", "nosuchkey", "nosuchkey", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, isFile, resolved := resolveClipArg(st, c.arg, false)
			if text != c.wantText || isFile != c.wantIsFile || resolved != c.wantResolved {
				t.Errorf("resolveClipArg(%q) = (%q,%v,%v), want (%q,%v,%v)",
					c.arg, text, isFile, resolved, c.wantText, c.wantIsFile, c.wantResolved)
			}
		})
	}
}

func TestNotInStoreMessage(t *testing.T) {
	one := notInStore([]string{"foo"}).Error()
	if !strings.Contains(one, `"foo"`) || !strings.Contains(one, "nothing to copy") {
		t.Errorf("single-missing message = %q", one)
	}
	many := notInStore([]string{"foo", "bar"}).Error()
	if !strings.Contains(many, `"foo"`) || !strings.Contains(many, `"bar"`) {
		t.Errorf("multi-missing message = %q", many)
	}
}

// TestClipMissingKeyCopiesNothing exercises the full clip command on the miss
// path, which returns before any clipboard call. Guarded to macOS because clip
// is macOS-only (elsewhere it returns ErrUnsupported first).
func TestClipMissingKeyCopiesNothing(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("clip is macOS-only")
	}
	cfg := testCfg(t)
	// Seed the store so it exists but lacks the requested key.
	runStore(t, cfg, "", "set", "present", "1")

	out, err := runCmd(t, newClipCmd(cfg), "", "absent")
	if err == nil {
		t.Fatal("clip on a missing key should return an error")
	}
	if !strings.Contains(err.Error(), `"absent"`) {
		t.Errorf("error should name the missing key: %v", err)
	}
	if strings.Contains(out, "copied") {
		t.Errorf("nothing should have been copied, got output: %q", out)
	}
}

func TestClipMutuallyExclusiveFlags(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("clip is macOS-only")
	}
	cfg := testCfg(t)
	if _, err := runCmd(t, newClipCmd(cfg), "", "x", "--file", "--text"); err == nil {
		t.Error("--file and --text together should error")
	}
}
