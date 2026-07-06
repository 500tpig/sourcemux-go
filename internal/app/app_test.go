package app

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitGlobalConfigArgRejectsBlankPath(t *testing.T) {
	cases := [][]string{
		{"--config", ""},
		{"--config="},
		{"-c", "  "},
	}
	for _, args := range cases {
		if _, _, err := SplitGlobalConfigArg(args); err == nil {
			t.Fatalf("SplitGlobalConfigArg(%v) error = nil, want error", args)
		}
	}
}

func TestSplitGlobalConfigArgAcceptsExplicitPath(t *testing.T) {
	path, args, err := SplitGlobalConfigArg([]string{"--config", "custom.json", "cli", "config", "path"})
	if err != nil {
		t.Fatalf("SplitGlobalConfigArg failed: %v", err)
	}
	if path != "custom.json" {
		t.Fatalf("path = %q, want custom.json", path)
	}
	if len(args) != 3 || args[0] != "cli" || args[1] != "config" || args[2] != "path" {
		t.Fatalf("args = %#v, want [cli config path]", args)
	}
}

func TestRunVersionJSON(t *testing.T) {
	SetVersionInfo("v1.2.3", "abc123", "2026-05-13T00:00:00Z")
	out := captureStdout(t, func() {
		if got := Run([]string{"version", "--json"}); got != 0 {
			t.Fatalf("Run(version --json) = %d, want 0", got)
		}
	})
	var decoded VersionInfo
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode version: %v\n%s", err, out)
	}
	if decoded.Version != "v1.2.3" || decoded.Commit != "abc123" {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestRunVersionFlagDoesNotLoadConfig(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	SetVersionInfo("v9.9.9", "def456", "2026-06-08T00:00:00Z")
	out := captureStdout(t, func() {
		if got := Run([]string{"--version", "--json"}); got != 0 {
			t.Fatalf("Run(--version --json) = %d, want 0", got)
		}
	})
	var decoded VersionInfo
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode version: %v\n%s", err, out)
	}
	if decoded.Version != "v9.9.9" || decoded.Commit != "def456" {
		t.Fatalf("decoded = %+v", decoded)
	}
}

func TestRunTopLevelHelpDoesNotLoadConfig(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	out := captureStdout(t, func() {
		if got := Run([]string{"--help"}); got != 0 {
			t.Fatalf("Run(--help) = %d, want 0", got)
		}
	})
	if !strings.Contains(out, "Usage: sourcemux") || !strings.Contains(out, "cli <command>") {
		t.Fatalf("unexpected help output: %s", out)
	}
}

func TestRunTopLevelInstallDryRun(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	out := captureStdout(t, func() {
		if got := Run([]string{"install", "codex", "--dry-run", "--json"}); got != 0 {
			t.Fatalf("Run(install codex --dry-run --json) = %d, want 0", got)
		}
	})
	if !json.Valid([]byte(out)) {
		t.Fatalf("install output is not JSON: %s", out)
	}
	if _, err := os.Stat(".agents/skills/sourcemux-routing/SKILL.md"); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote skill file: %v", err)
	}
}

func TestRunTopLevelSetupHelpDoesNotLoadConfig(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if got := Run([]string{"setup", "--help"}); got != 0 {
		t.Fatalf("Run(setup --help) = %d, want 0", got)
	}
}

func TestRunTopLevelConfigPathUsesGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, "custom.sourcemux.json")
	out := captureStdout(t, func() {
		if got := Run([]string{"--config", path, "config", "path", "--json"}); got != 0 {
			t.Fatalf("Run(--config path config path --json) = %d, want 0", got)
		}
	})
	if !strings.Contains(out, path) {
		t.Fatalf("config path output missing %q in %s", path, out)
	}
}

func TestRunTopLevelSearchRoutesToCLI(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	out := captureStdout(t, func() {
		if got := Run([]string{"search", "hello", "--json"}); got != 1 {
			t.Fatalf("Run(search hello --json) = %d, want 1 for missing config", got)
		}
	})
	if !strings.Contains(out, "config file not found") {
		t.Fatalf("search output did not come from CLI config handling: %s", out)
	}
}

func TestRunTopLevelResearchEvalRoutesToCLI(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	casesPath := filepath.Join(dir, "research-eval-cases.json")
	if err := os.WriteFile(casesPath, []byte(`{
	  "cases": [
	    {
	      "name": "top-level",
	      "query": "SourceMux release status",
	      "max_fetches": 1,
	      "search_results": [
	        {"urls": ["https://github.com/500tpig/sourcemux-go/releases"]}
	      ],
	      "fetch_pages": [
	        {
	          "url": "https://github.com/500tpig/sourcemux-go/releases",
	          "content": "SourceMux release status appears in GitHub releases."
	        }
	      ],
	      "expect": {
	        "selected_source_urls_include": ["https://github.com/500tpig/sourcemux-go/releases"],
	        "min_fetched_pages": 1,
	        "min_confirmed_facts": 1
	      }
	    }
	  ]
	}`), 0o600); err != nil {
		t.Fatalf("write cases: %v", err)
	}

	out := captureStdout(t, func() {
		if got := Run([]string{"eval-research", "--cases", casesPath, "--json"}); got != 0 {
			t.Fatalf("Run(eval-research --json) = %d, want 0", got)
		}
	})
	if !json.Valid([]byte(out)) || !strings.Contains(out, `"ok": true`) {
		t.Fatalf("eval-research output is not successful JSON: %s", out)
	}
}

func TestRunTopLevelBootstrapDryRun(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	out := captureStdout(t, func() {
		if got := Run([]string{"bootstrap", "codex", "--dry-run", "--json"}); got != 0 {
			t.Fatalf("Run(bootstrap codex --dry-run --json) = %d, want 0", got)
		}
	})
	if !json.Valid([]byte(out)) {
		t.Fatalf("bootstrap output is not JSON: %s", out)
	}
	if !strings.Contains(out, `"target": "codex"`) {
		t.Fatalf("bootstrap output missing codex action: %s", out)
	}
	if _, err := os.Stat(".agents/skills/sourcemux-routing/SKILL.md"); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote skill file: %v", err)
	}
}

func TestRunTopLevelBootstrapUserScopeUsesUserConfigDefaultUnlessGlobalConfigExplicit(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	t.Setenv("HOME", dir)

	out := captureStdout(t, func() {
		if got := Run([]string{"bootstrap", "codex", "--scope", "user", "--dry-run", "--json", "--binary", "/usr/local/bin/sourcemux"}); got != 0 {
			t.Fatalf("Run(bootstrap codex user dry-run) = %d, want 0", got)
		}
	})
	userConfig := filepath.Join(dir, ".config", "sourcemux", "sourcemux.json")
	if !strings.Contains(out, userConfig) {
		t.Fatalf("bootstrap user output missing default user config %q in %s", userConfig, out)
	}

	explicitConfig := filepath.Join(dir, "explicit.sourcemux.json")
	out = captureStdout(t, func() {
		if got := Run([]string{"--config", explicitConfig, "bootstrap", "codex", "--scope", "user", "--dry-run", "--json", "--binary", "/usr/local/bin/sourcemux"}); got != 0 {
			t.Fatalf("Run(--config bootstrap codex user dry-run) = %d, want 0", got)
		}
	})
	if !strings.Contains(out, explicitConfig) || strings.Contains(out, userConfig) {
		t.Fatalf("explicit global config should win over user default; output: %s", out)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	fn()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
