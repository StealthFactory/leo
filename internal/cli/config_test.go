package cli

import (
	"os"
	"strings"
	"testing"
)

func TestConfigPath(t *testing.T) {
	cfg := testCfg(t)
	out, err := runCmd(t, newConfigCmd(cfg), "", "path")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != cfg.ConfigPath {
		t.Errorf("config path = %q, want %q", strings.TrimSpace(out), cfg.ConfigPath)
	}
}

func TestConfigShow(t *testing.T) {
	cfg := testCfg(t)
	out, err := runCmd(t, newConfigCmd(cfg), "", "show")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, cfg.StorePath) {
		t.Errorf("show should list the store path:\n%s", out)
	}
	if !strings.Contains(out, "[default]") {
		t.Errorf("show should list the default set:\n%s", out)
	}
}

func TestConfigSetupWritesTypedValues(t *testing.T) {
	cfg := testCfg(t)
	stdin := "/custom/store.json\npython\n/custom/commands\n"
	out, err := runCmd(t, newConfigCmd(cfg), stdin, "setup")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "wrote "+cfg.ConfigPath) {
		t.Errorf("setup should report the written path:\n%s", out)
	}
	b, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	got := string(b)
	for _, want := range []string{
		`store_path = "/custom/store.json"`,
		`gen_lang = "python"`,
		`path = "/custom/commands"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("config missing %q:\n%s", want, got)
		}
	}
}

func TestConfigSetupKeepsDefaultsOnEnter(t *testing.T) {
	cfg := testCfg(t)
	// Three bare newlines: keep store_path, gen_lang, and the default set path.
	if _, err := runCmd(t, newConfigCmd(cfg), "\n\n\n", "setup"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	got := string(b)
	if !strings.Contains(got, `store_path = "`+cfg.StorePath+`"`) {
		t.Errorf("default store_path not kept:\n%s", got)
	}
	if !strings.Contains(got, `gen_lang = "bash"`) {
		t.Errorf("default gen_lang not kept:\n%s", got)
	}
}

func TestConfigSetupCanonicalizesLangAlias(t *testing.T) {
	cfg := testCfg(t)
	// "py" is an alias; it should be stored as the canonical "python".
	if _, err := runCmd(t, newConfigCmd(cfg), "\npy\n\n", "setup"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(cfg.ConfigPath)
	if !strings.Contains(string(b), `gen_lang = "python"`) {
		t.Errorf("alias not canonicalized:\n%s", b)
	}
}
