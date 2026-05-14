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
