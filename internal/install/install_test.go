package install

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunListAgentsJSON(t *testing.T) {
	out := captureStdout(t, func() {
		if got := RunInstall([]string{"list-agents", "--json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(list-agents --json) = %d, want 0", got)
		}
	})
	var parsed []Target
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("decode list-agents: %v\n%s", err, out)
	}
	if len(parsed) == 0 {
		t.Fatal("list-agents returned no targets")
	}
	foundCodex := false
	for _, target := range parsed {
		if target.Name == "codex" {
			foundCodex = true
			if target.Support != SupportFull || !target.Skill {
				t.Fatalf("codex target = %+v", target)
			}
		}
	}
	if !foundCodex {
		t.Fatal("codex target missing")
	}
}

func TestInstallCodexDryRunJSONDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	out := captureStdout(t, func() {
		if got := RunInstall([]string{"codex", "--dry-run", "--json", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(codex dry-run) = %d, want 0", got)
		}
	})
	var plan Plan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, out)
	}
	if !plan.DryRun || plan.Mode != "install" {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.Actions) < 2 {
		t.Fatalf("actions = %+v, want skill and Codex MCP guidance", plan.Actions)
	}
	cmd := findAction(plan.Actions, "codex", "shell_command")
	if cmd == nil || strings.Join(cmd.Command, " ") != "codex mcp add sourcemux -- "+plan.Binary+" --config "+plan.ConfigFile {
		t.Fatalf("codex command action = %+v", cmd)
	}
	snippet := findAction(plan.Actions, "codex", "config_snippet")
	if snippet == nil || snippet.Path != ".codex/config.toml" || !strings.Contains(snippet.Snippet, "[mcp_servers.sourcemux]") {
		t.Fatalf("codex config snippet = %+v", snippet)
	}
	path := filepath.Join(dir, ".agents", "skills", skillName, "SKILL.md")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote %s: %v", path, err)
	}
	if strings.Contains(out, "sk-") || strings.Contains(out, "grok-search") {
		t.Fatalf("dry-run output leaked old names or secret-looking data: %s", out)
	}
}

func TestInstallCodexWriteConfigDryRunJSONDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	out := captureStdout(t, func() {
		if got := RunInstall([]string{"codex", "--write-config", "--dry-run", "--json", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(codex --write-config dry-run) = %d, want 0", got)
		}
	})
	var plan Plan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, out)
	}
	action := findAction(plan.Actions, "codex", "merge_config")
	if action == nil {
		t.Fatalf("missing codex merge_config action: %+v", plan.Actions)
	}
	if !strings.HasSuffix(action.Path, filepath.Join(".codex", "config.toml")) || action.Status != "create" {
		t.Fatalf("codex merge_config action = %+v", action)
	}
	if _, err := os.Stat(filepath.Join(dir, ".codex", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote Codex config: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents", "skills", skillName, "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote skill: %v", err)
	}
}

func TestInstallCodexProjectWritesPortableSkill(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	if got := RunInstall([]string{"codex", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
		t.Fatalf("RunInstall(codex) = %d, want 0", got)
	}
	path := filepath.Join(dir, ".agents", "skills", skillName, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated skill: %v", err)
	}
	manifest, err := readManifest(manifestPath(path))
	if err != nil {
		t.Fatalf("read generated manifest: %v", err)
	}
	if manifest.Target != "codex" || manifest.ContentSHA256 != contentSHA256(data) {
		t.Fatalf("manifest = %+v, content hash = %s", manifest, contentSHA256(data))
	}
	text := string(data)
	for _, want := range []string{"name: sourcemux-routing", "SourceMux routing", "custom.sourcemux.json"} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated skill missing %q:\n%s", want, text)
		}
	}
	for _, bad := range []string{"grok-search-routing", "/Users/johnsmith/Project/Study/grok-search-go"} {
		if strings.Contains(text, bad) {
			t.Fatalf("generated skill contains non-portable %q:\n%s", bad, text)
		}
	}
}

func TestInstallConflictRequiresForce(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, ".agents", "skills", skillName, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("custom user content"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := RunInstall([]string{"codex"}, "sourcemux.json"); got != 1 {
		t.Fatalf("RunInstall(conflict) = %d, want 1", got)
	}
	if got := RunInstall([]string{"codex", "--force"}, "sourcemux.json"); got != 0 {
		t.Fatalf("RunInstall(conflict --force) = %d, want 0", got)
	}
	matches, err := filepath.Glob(path + ".bak.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("backup matches = %+v, want one backup", matches)
	}
}

func TestInstallFirstTierTargetsExposeOfficialMCPGuidance(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	out := captureStdout(t, func() {
		if got := RunInstall([]string{"claude-code", "gemini", "opencode", "--dry-run", "--json", "--scope", "project"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(first-tier dry-run) = %d, want 0", got)
		}
	})
	var plan Plan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, out)
	}
	claude := findAction(plan.Actions, "claude-code", "shell_command")
	if claude == nil || strings.Join(claude.Command[:5], " ") != "claude mcp add --transport stdio" {
		t.Fatalf("claude command action = %+v", claude)
	}
	if !containsAll(claude.Command, []string{"--scope", "project", "sourcemux", "--config"}) {
		t.Fatalf("claude command missing scope/name/config: %+v", claude.Command)
	}

	gemini := findAction(plan.Actions, "gemini", "shell_command")
	if gemini == nil || strings.Join(gemini.Command[:4], " ") != "gemini mcp add --scope" {
		t.Fatalf("gemini command action = %+v", gemini)
	}
	if !containsAll(gemini.Command, []string{"project", "sourcemux", "--", "--config"}) {
		t.Fatalf("gemini command missing scope/name/arg separator/config: %+v", gemini.Command)
	}
	geminiSnippet := findAction(plan.Actions, "gemini", "config_snippet")
	if geminiSnippet == nil || geminiSnippet.Path != ".gemini/settings.json" || !strings.Contains(geminiSnippet.Snippet, `"mcpServers"`) {
		t.Fatalf("gemini snippet action = %+v", geminiSnippet)
	}

	opencode := findAction(plan.Actions, "opencode", "config_snippet")
	if opencode == nil || opencode.Path != "opencode.json" || !strings.Contains(opencode.Snippet, `"type": "local"`) {
		t.Fatalf("opencode snippet action = %+v", opencode)
	}
}

func TestInstallGeminiWriteConfigPreservesKeysAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := `{
  "theme": "dark",
  "mcpServers": {
    "other": {"command": "other-tool"}
  }
}`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if got := RunInstall([]string{"gemini", "--write-config", "--json", "--binary", "/opt/sourcemux", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(gemini --write-config) = %d, want 0", got)
		}
	})
	var plan Plan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, out)
	}
	action := findAction(plan.Actions, "gemini", "merge_config")
	if action == nil || action.Status != "updated" || action.Backup == "" || !strings.Contains(action.Message, "Backup will be created") {
		t.Fatalf("gemini merge_config action = %+v", action)
	}
	backupData, err := os.ReadFile(action.Backup)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backupData) != original {
		t.Fatalf("backup = %q, want original %q", backupData, original)
	}
	cfg := readJSONMap(t, path)
	if cfg["theme"] != "dark" {
		t.Fatalf("unrelated key not preserved: %+v", cfg)
	}
	mcp := cfg["mcpServers"].(map[string]any)
	if _, ok := mcp["other"]; !ok {
		t.Fatalf("other MCP server not preserved: %+v", mcp)
	}
	sourcemux := mcp["sourcemux"].(map[string]any)
	if sourcemux["command"] != "/opt/sourcemux" {
		t.Fatalf("sourcemux command = %+v", sourcemux)
	}
	args := sourcemux["args"].([]any)
	if len(args) != 2 || args[0] != "--config" || !strings.HasSuffix(args[1].(string), "custom.sourcemux.json") {
		t.Fatalf("sourcemux args = %+v", args)
	}
}

func TestWriteConfigDryRunShowsBackupIntentWithoutBackup(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"other":{"command":"other"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if got := RunInstall([]string{"gemini", "--write-config", "--dry-run", "--json", "--binary", "/opt/sourcemux", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(gemini dry-run --write-config) = %d, want 0", got)
		}
	})
	var plan Plan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, out)
	}
	action := findAction(plan.Actions, "gemini", "merge_config")
	if action == nil || action.Status != "update" || action.Backup == "" || !strings.Contains(action.Message, "Backup will be created") {
		t.Fatalf("dry-run merge_config action = %+v", action)
	}
	if _, err := os.Stat(action.Backup); !os.IsNotExist(err) {
		t.Fatalf("dry-run created backup %s: %v", action.Backup, err)
	}
	cfg := readJSONMap(t, path)
	if _, present := cfg["mcpServers"].(map[string]any)["sourcemux"]; present {
		t.Fatalf("dry-run wrote sourcemux entry: %+v", cfg)
	}
}

func TestWriteConfigDryRunHumanShowsBackupIntent(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"other":{"command":"other"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if got := RunInstall([]string{"gemini", "--write-config", "--dry-run", "--binary", "/opt/sourcemux", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(gemini human dry-run --write-config) = %d, want 0", got)
		}
	})
	if !strings.Contains(out, "backup:") || !strings.Contains(out, "Backup will be created") || !strings.Contains(out, "Gemini JSON may be reserialized/reformatted") {
		t.Fatalf("human dry-run did not show backup intent:\n%s", out)
	}
	matches, err := filepath.Glob(path + ".bak.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("human dry-run created backups: %+v", matches)
	}
}

func TestWriteConfigNonDryRunJSONPrintsBackupNoticeToStderr(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"other":{"command":"other"}}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var out string
	errOut := captureStderr(t, func() {
		out = captureStdout(t, func() {
			if got := RunInstall([]string{"gemini", "--write-config", "--json", "--binary", "/opt/sourcemux", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
				t.Fatalf("RunInstall(gemini --write-config --json) = %d, want 0", got)
			}
		})
	})
	if !strings.Contains(errOut, "will modify") || !strings.Contains(errOut, "create backup") || !strings.Contains(errOut, "Gemini JSON may be reserialized/reformatted") {
		t.Fatalf("stderr pre-apply notice missing backup warning:\n%s", errOut)
	}
	var plan Plan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("stdout is not JSON plan: %v\nstdout=%s\nstderr=%s", err, out, errOut)
	}
	action := findAction(plan.Actions, "gemini", "merge_config")
	if action == nil || action.Status != "updated" || action.Backup == "" {
		t.Fatalf("merge_config action = %+v", action)
	}
	if !strings.Contains(action.Message, "JSON formatting may change") {
		t.Fatalf("merge_config action missing formatting warning: %+v", action)
	}
}

func TestInstallOpenCodeWriteConfigUpdatesJSONCAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, "opencode.json")
	original := `{
  // OpenCode allows JSONC, but SourceMux rewrites valid JSON.
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "sourcemux": {"type": "local", "command": ["/old/sourcemux"], "enabled": true},
    "other": {"type": "local", "command": ["other"], "enabled": true},
  },
}`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		if got := RunInstall([]string{"opencode", "--write-config", "--json", "--binary", "/opt/sourcemux", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(opencode --write-config) = %d, want 0", got)
		}
	})
	var plan Plan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, out)
	}
	action := findAction(plan.Actions, "opencode", "merge_config")
	if action == nil || action.Status != "updated" || action.Backup == "" || !strings.Contains(action.Message, "drifted") || !strings.Contains(action.Message, "OpenCode JSONC may be reserialized as JSON/reformatted") || !strings.Contains(action.Message, "comments and original formatting may not be preserved") {
		t.Fatalf("opencode merge_config action = %+v", action)
	}
	backupData, err := os.ReadFile(action.Backup)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}
	if string(backupData) != original {
		t.Fatalf("backup = %q, want original JSONC", backupData)
	}
	cfg := readJSONMap(t, path)
	mcp := cfg["mcp"].(map[string]any)
	if _, ok := mcp["other"]; !ok {
		t.Fatalf("other MCP server not preserved: %+v", mcp)
	}
	sourcemux := mcp["sourcemux"].(map[string]any)
	if sourcemux["type"] != "local" || sourcemux["enabled"] != true {
		t.Fatalf("sourcemux opencode entry = %+v", sourcemux)
	}
	command := sourcemux["command"].([]any)
	if len(command) != 3 || command[0] != "/opt/sourcemux" || command[1] != "--config" {
		t.Fatalf("sourcemux command = %+v", command)
	}
}

func TestShellJoinQuotesSpaces(t *testing.T) {
	got := shellJoin([]string{"codex", "mcp", "add", "sourcemux", "--", "/tmp/Source Mux/sourcemux", "--config", "/tmp/cfg file.json"})
	want := "codex mcp add sourcemux -- '/tmp/Source Mux/sourcemux' --config '/tmp/cfg file.json'"
	if got != want {
		t.Fatalf("shellJoin = %q, want %q", got, want)
	}
}

func TestInstallBinaryOverride(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	out := captureStdout(t, func() {
		if got := RunInstall([]string{"stdio", "--binary", "/opt/sourcemux/bin/sourcemux", "--dry-run", "--json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(stdio --binary) = %d, want 0", got)
		}
	})
	var plan Plan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, out)
	}
	if plan.Binary != "/opt/sourcemux/bin/sourcemux" {
		t.Fatalf("binary = %q, want override", plan.Binary)
	}
	if len(plan.Warnings) != 0 {
		t.Fatalf("warnings = %+v, want none for stable override", plan.Warnings)
	}
	stdio := findAction(plan.Actions, "stdio", "stdio")
	if stdio == nil || strings.Join(stdio.Command, " ") != "/opt/sourcemux/bin/sourcemux --config "+plan.ConfigFile {
		t.Fatalf("stdio action = %+v", stdio)
	}
}

func TestUninstallRemovesGeneratedSkill(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if got := RunInstall([]string{"codex"}, "sourcemux.json"); got != 0 {
		t.Fatalf("RunInstall(codex) = %d, want 0", got)
	}
	if got := RunUninstall([]string{"codex"}, "sourcemux.json"); got != 0 {
		t.Fatalf("RunUninstall(codex) = %d, want 0", got)
	}
	path := filepath.Join(dir, ".agents", "skills", skillName, "SKILL.md")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("skill still exists after uninstall: %v", err)
	}
	if _, err := os.Stat(manifestPath(path)); !os.IsNotExist(err) {
		t.Fatalf("manifest still exists after uninstall: %v", err)
	}
}

func TestUninstallCodexWriteConfigRemovesOnlySourceMuxEntry(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := `profile = "default"

[mcp_servers.other]
command = "other"
args = ["--flag"]

[mcp_servers.sourcemux]
command = "/opt/sourcemux"
args = ["--config", "/tmp/sourcemux.json"]
`
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := RunInstall([]string{"codex", "--binary", "/opt/sourcemux", "--config", "/tmp/sourcemux.json"}, "sourcemux.json"); got != 0 {
		t.Fatalf("RunInstall(codex) = %d, want 0", got)
	}
	out := captureStdout(t, func() {
		if got := RunUninstall([]string{"codex", "--write-config", "--json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunUninstall(codex --write-config) = %d, want 0", got)
		}
	})
	var plan Plan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, out)
	}
	action := findAction(plan.Actions, "codex", "remove_config")
	if action == nil || action.Status != "removed" || action.Backup == "" || !strings.Contains(action.Message, "Remove only the sourcemux") || !strings.Contains(action.Message, "Codex TOML may be reserialized/reformatted") || !strings.Contains(action.Message, "comments and original formatting may not be preserved") {
		t.Fatalf("codex remove_config action = %+v", action)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("config file was deleted: %v", err)
	}
	text := string(updated)
	if strings.Contains(text, "sourcemux") {
		t.Fatalf("sourcemux entry still present:\n%s", text)
	}
	if !strings.Contains(text, "profile = 'default'") && !strings.Contains(text, `profile = "default"`) {
		t.Fatalf("unrelated top-level key missing:\n%s", text)
	}
	if !strings.Contains(text, "mcp_servers.other") {
		t.Fatalf("unrelated MCP server missing:\n%s", text)
	}
}

func TestUninstallJSONWriteConfigRemovesOnlySourceMuxEntry(t *testing.T) {
	tests := []struct {
		name      string
		target    string
		path      string
		parentKey string
		original  string
	}{
		{
			name:      "gemini",
			target:    "gemini",
			path:      filepath.Join(".gemini", "settings.json"),
			parentKey: "mcpServers",
			original:  `{"theme":"dark","mcpServers":{"other":{"command":"other"},"sourcemux":{"command":"/opt/sourcemux","args":["--config","/tmp/sourcemux.json"]}}}`,
		},
		{
			name:      "opencode",
			target:    "opencode",
			path:      "opencode.json",
			parentKey: "mcp",
			original:  `{"$schema":"https://opencode.ai/config.json","mcp":{"other":{"type":"local","command":["other"],"enabled":true},"sourcemux":{"type":"local","command":["/opt/sourcemux","--config","/tmp/sourcemux.json"],"enabled":true}}}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			chdir(t, dir)
			path := filepath.Join(dir, tc.path)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.original), 0o644); err != nil {
				t.Fatal(err)
			}

			out := captureStdout(t, func() {
				if got := RunUninstall([]string{tc.target, "--write-config", "--json"}, "sourcemux.json"); got != 0 {
					t.Fatalf("RunUninstall(%s --write-config) = %d, want 0", tc.target, got)
				}
			})
			var plan Plan
			if err := json.Unmarshal([]byte(out), &plan); err != nil {
				t.Fatalf("decode plan: %v\n%s", err, out)
			}
			action := findAction(plan.Actions, tc.target, "remove_config")
			if action == nil || action.Status != "removed" || action.Backup == "" {
				t.Fatalf("%s remove_config action = %+v", tc.target, action)
			}
			cfg := readJSONMap(t, path)
			parent := cfg[tc.parentKey].(map[string]any)
			if _, ok := parent["other"]; !ok {
				t.Fatalf("%s other MCP server not preserved: %+v", tc.target, parent)
			}
			if _, ok := parent["sourcemux"]; ok {
				t.Fatalf("%s sourcemux entry still present: %+v", tc.target, parent)
			}
		})
	}
}

func TestUninstallRefusesSkillWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, ".agents", "skills", skillName, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("user-authored skill"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := RunUninstall([]string{"codex"}, "sourcemux.json"); got != 1 {
		t.Fatalf("RunUninstall(codex unmanaged) = %d, want 1", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("unmanaged skill was removed: %v", err)
	}
	if string(data) != "user-authored skill" {
		t.Fatalf("unmanaged skill changed to %q", data)
	}
}

func TestUninstallRefusesModifiedGeneratedSkill(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if got := RunInstall([]string{"codex"}, "sourcemux.json"); got != 0 {
		t.Fatalf("RunInstall(codex) = %d, want 0", got)
	}
	path := filepath.Join(dir, ".agents", "skills", skillName, "SKILL.md")
	if err := os.WriteFile(path, []byte("locally modified generated skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := RunUninstall([]string{"codex"}, "sourcemux.json"); got != 1 {
		t.Fatalf("RunUninstall(codex modified) = %d, want 1", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("modified skill should remain: %v", err)
	}
	if _, err := os.Stat(manifestPath(path)); err != nil {
		t.Fatalf("manifest should remain: %v", err)
	}
}

func TestStatusReportsManagedAndModifiedSkill(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if got := RunInstall([]string{"codex"}, "sourcemux.json"); got != 0 {
		t.Fatalf("RunInstall(codex) = %d, want 0", got)
	}
	out := captureStdout(t, func() {
		if got := RunInstall([]string{"status", "codex", "--json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(status codex --json) = %d, want 0", got)
		}
	})
	statuses := decodeStatuses(t, out)
	if len(statuses) != 1 || !statuses[0].Installed || !statuses[0].Managed || statuses[0].Modified {
		t.Fatalf("status after install = %+v", statuses)
	}

	path := filepath.Join(dir, ".agents", "skills", skillName, "SKILL.md")
	if err := os.WriteFile(path, []byte("locally modified generated skill"), 0o644); err != nil {
		t.Fatal(err)
	}
	out = captureStdout(t, func() {
		if got := RunInstall([]string{"status", "codex", "--json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(status codex --json modified) = %d, want 0", got)
		}
	})
	statuses = decodeStatuses(t, out)
	if len(statuses) != 1 || !statuses[0].Modified {
		t.Fatalf("status after modification = %+v", statuses)
	}
}

func TestInstallStatusConfigStatusReportsMatchAndDrift(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if got := RunInstall([]string{"gemini", "--write-config", "--binary", "/opt/sourcemux", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
		t.Fatalf("RunInstall(gemini --write-config) = %d, want 0", got)
	}
	out := captureStdout(t, func() {
		if got := RunInstall([]string{"status", "gemini", "--config-status", "--json", "--binary", "/opt/sourcemux", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(status --config-status) = %d, want 0", got)
		}
	})
	statuses := decodeStatuses(t, out)
	if len(statuses) != 1 || statuses[0].ConfigStatus == nil || statuses[0].ConfigStatus.Status != "matching" || !statuses[0].ConfigStatus.Matches {
		t.Fatalf("matching config status = %+v", statuses)
	}
	path := filepath.Join(dir, ".gemini", "settings.json")
	cfg := readJSONMap(t, path)
	cfg["mcpServers"].(map[string]any)["sourcemux"].(map[string]any)["command"] = "/different/sourcemux"
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	out = captureStdout(t, func() {
		if got := RunInstall([]string{"status", "gemini", "--config-status", "--json", "--binary", "/opt/sourcemux", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(status drift --config-status) = %d, want 0", got)
		}
	})
	statuses = decodeStatuses(t, out)
	if len(statuses) != 1 || statuses[0].ConfigStatus == nil || statuses[0].ConfigStatus.Status != "drifted" || !statuses[0].ConfigStatus.Drifted {
		t.Fatalf("drifted config status = %+v", statuses)
	}
}

func TestMalformedGeminiConfigFailsWithoutChangingFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"mcpServers": `)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := RunInstall([]string{"gemini", "--write-config", "--json"}, "sourcemux.json"); got != 2 {
		t.Fatalf("RunInstall(malformed gemini --write-config) = %d, want 2", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("malformed config changed to %q", data)
	}
	matches, err := filepath.Glob(path + ".bak.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("malformed config created backups: %+v", matches)
	}
}

func TestMalformedCodexConfigFailsWithoutChangingFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, ".codex", "config.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte("[mcp_servers.sourcemux\ncommand = \"/opt/sourcemux\"\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := RunInstall([]string{"codex", "--write-config", "--json"}, "sourcemux.json"); got != 2 {
		t.Fatalf("RunInstall(malformed codex --write-config) = %d, want 2", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("malformed TOML changed to %q", data)
	}
	matches, err := filepath.Glob(path + ".bak.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("malformed TOML created backups: %+v", matches)
	}
}

func TestWriteConfigRefusesNonObjectParentWithoutChangingFile(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	path := filepath.Join(dir, ".gemini", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	original := []byte(`{"theme":"dark","mcpServers":"manual note"}`)
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := RunInstall([]string{"gemini", "--write-config", "--json"}, "sourcemux.json"); got != 2 {
		t.Fatalf("RunInstall(gemini non-object parent --write-config) = %d, want 2", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("non-object parent config changed to %q", data)
	}
	matches, err := filepath.Glob(path + ".bak.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("non-object parent created backups: %+v", matches)
	}
}

func TestWriteConfigIdempotentReportsUnchanged(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	if got := RunInstall([]string{"opencode", "--write-config", "--binary", "/opt/sourcemux", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
		t.Fatalf("RunInstall(opencode --write-config) = %d, want 0", got)
	}
	out := captureStdout(t, func() {
		if got := RunInstall([]string{"opencode", "--write-config", "--dry-run", "--json", "--binary", "/opt/sourcemux", "--config", "custom.sourcemux.json"}, "sourcemux.json"); got != 0 {
			t.Fatalf("RunInstall(opencode --write-config idempotent dry-run) = %d, want 0", got)
		}
	})
	var plan Plan
	if err := json.Unmarshal([]byte(out), &plan); err != nil {
		t.Fatalf("decode plan: %v\n%s", err, out)
	}
	action := findAction(plan.Actions, "opencode", "merge_config")
	if action == nil || action.Status != "unchanged" || action.Backup != "" {
		t.Fatalf("idempotent merge_config action = %+v", action)
	}
}

func findAction(actions []PlanAction, target, actionType string) *PlanAction {
	for i := range actions {
		if actions[i].Target == target && actions[i].Type == actionType {
			return &actions[i]
		}
	}
	return nil
}

func containsAll(values, want []string) bool {
	set := map[string]bool{}
	for _, value := range values {
		set[value] = true
	}
	for _, value := range want {
		if !set[value] {
			return false
		}
	}
	return true
}

func decodeStatuses(t *testing.T, out string) []TargetStatus {
	t.Helper()
	var statuses []TargetStatus
	if err := json.Unmarshal([]byte(out), &statuses); err != nil {
		t.Fatalf("decode statuses: %v\n%s", err, out)
	}
	return statuses
}

func readJSONMap(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("decode %s: %v\n%s", path, err, data)
	}
	return parsed
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

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()
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
