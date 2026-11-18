package config

import (
	"strings"
	"testing"
)

func TestLoadJEVV1AndV2(t *testing.T) {
	t.Setenv("TYPESAFE_API_KEY", "ignored-env-key")
	for _, prefix := range []string{"", `"version":2,"capabilities":{},`} {
		cfg, err := LoadFile(writeConfig(t, `{`+prefix+`"jev":{"apiKey":" test-key ","apiURL":"https://judge.example/v1/","model":" custom ","timeoutSec":20}}`))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.JEV.APIKey != "test-key" || cfg.JEV.APIURL != "https://judge.example/v1" || cfg.JEV.Model != "custom" || cfg.JEV.TimeoutSec != 20 {
			t.Fatal("JEV configuration not normalized")
		}
		cfg, err = LoadFile(writeConfig(t, `{`+prefix+`"jev":{"answerCheck":true}}`))
		if err != nil || !cfg.JEV.AnswerCheck {
			t.Fatalf("answerCheck configuration failed: %+v %v", cfg, err)
		}
		cfg, err = LoadFile(writeConfig(t, `{`+prefix+`"jev":{}}`))
		if err != nil || cfg.JEV.APIKey != "" || cfg.JEV.APIURL != "https://api.typesafe.ai/v1" || cfg.JEV.Model != "jev-latest" || cfg.JEV.TimeoutSec != 15 || cfg.JEV.AnswerCheck {
			t.Fatalf("default configuration or environment isolation failed: %v", err)
		}
	}
}

func TestLoadRejectsInvalidJEVConfig(t *testing.T) {
	for _, value := range []string{
		`{"timeoutSec":-1}`, `{"timeoutSec":301}`, `{"timeoutSec":1.5}`,
		`{"apiURL":"ftp://judge.example"}`, `{"apiURL":"https://"}`,
		`{"apiURL":"https://user:private-key@judge.example"}`,
		`{"apiURL":"https://judge.example?key=private-key"}`,
		`{"apiURL":"https://judge.example?"}`, `{"apiURL":"https://judge.example#private-key"}`,
	} {
		_, err := LoadFile(writeConfig(t, `{"jev":`+value+`}`))
		if err == nil || strings.Contains(err.Error(), "private-key") {
			t.Fatalf("invalid configuration accepted or leaked: %v", err)
		}
	}
}
