package cli

import (
	"encoding/json"
	"strings"
	"testing"

	cfgpkg "github.com/500tpig/sourcemux-go/internal/config"
)

func TestConfigMigratePreservesJEV(t *testing.T) {
	path := writeCLIConfig(t, `{"jev":{"apiURL":"https://judge.example/v1","apiKey":"jev-private-key","model":"custom-judge","timeoutSec":24}}`)
	before, err := cfgpkg.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if code := Run([]string{"--config", path, "config", "migrate", "--json"}); code != 0 {
			t.Fatalf("migration exit = %d", code)
		}
	})
	after, err := cfgpkg.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != 2 || after.JEV != before.JEV {
		t.Fatal("migration lost JEV settings")
	}
	if strings.Contains(out, "jev-private-key") {
		t.Fatal("migration output leaked JEV key")
	}
}

func TestConfigListJEVOnlyShowsKeyPresence(t *testing.T) {
	for _, key := range []string{"", "jev-private-key"} {
		path := writeCLIConfig(t, `{"jev":{"apiURL":"https://judge.example/v1","apiKey":"`+key+`","model":"custom-judge","timeoutSec":24}}`)
		for _, asJSON := range []bool{false, true} {
			args := []string{"--config", path, "config", "list"}
			if asJSON {
				args = append(args, "--json")
			}
			out := captureStdout(t, func() {
				if code := Run(args); code != 0 {
					t.Fatalf("list exit = %d", code)
				}
			})
			if strings.Contains(out, "jev-") || strings.Contains(out, "-key") {
				t.Fatal("diagnostics leaked JEV key fragments")
			}
			status := "not set"
			if key != "" {
				status = "configured"
			}
			if asJSON {
				var result configListOutput
				if err := json.Unmarshal([]byte(out), &result); err != nil {
					t.Fatal(err)
				}
				if result.JEVAPIURL != "https://judge.example/v1" || result.JEVModel != "custom-judge" || result.JEVTimeoutSec != 24 || result.JEVKey != status {
					t.Fatal("JEV diagnostics missing")
				}
			} else if !strings.Contains(out, "JEV: url=https://judge.example/v1 model=custom-judge timeout=24s key="+status) {
				t.Fatal("text diagnostics missing")
			}
		}
	}
}
