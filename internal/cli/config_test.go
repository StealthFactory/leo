package cli

import (
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
