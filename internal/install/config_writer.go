package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const sourceMuxConfigBackupReason = "Backup will be created before modifying this existing MCP client config so the previous config can be restored."

func configRewriteWarning(target string) string {
	switch target {
	case "codex":
		return "Codex TOML may be reserialized/reformatted; comments and original formatting may not be preserved."
	case "opencode":
		return "OpenCode JSONC may be reserialized as JSON/reformatted; comments and original formatting may not be preserved."
	case "gemini":
		return "Gemini JSON may be reserialized/reformatted; JSON formatting may change."
	default:
		return "Config may be reserialized/reformatted; comments and original formatting may not be preserved."
	}
}

func configChangeMessage(target, message string) string {
	return message + " " + sourceMuxConfigBackupReason + " " + configRewriteWarning(target)
}

func planConfigWriteAction(target, scope, binary, configPath string) (PlanAction, bool, error) {
	path, ok, err := mcpConfigPath(target, scope)
	if err != nil || !ok {
		return PlanAction{}, ok, err
	}
	status, err := readMCPConfigStatus(target, path, binary, configPath)
	if err != nil {
		return PlanAction{}, true, err
	}
	action := PlanAction{
		Type:    "merge_config",
		Target:  target,
		Path:    path,
		Status:  "create",
		Message: "Create supported MCP client config with only the sourcemux entry.",
	}
	if status.Exists {
		if status.Matches {
			action.Status = "unchanged"
			action.Message = "SourceMux MCP config already matches the desired command and config path."
			return action, true, nil
		}
		action.Status = "update"
		action.Backup = plannedBackupPath(path)
		if status.EntryPresent {
			action.Message = configChangeMessage(target, "Existing sourcemux MCP config entry is drifted and will be updated.")
		} else {
			action.Message = configChangeMessage(target, "Existing MCP client config will be merged with a new sourcemux entry.")
		}
	}
	return action, true, nil
}

func planConfigRemoveAction(target, scope string) (PlanAction, bool, error) {
	path, ok, err := mcpConfigPath(target, scope)
	if err != nil || !ok {
		return PlanAction{}, ok, err
	}
	status, err := readMCPConfigStatus(target, path, "", "")
	if err != nil {
		return PlanAction{}, true, err
	}
	action := PlanAction{
		Type:    "remove_config",
		Target:  target,
		Path:    path,
		Status:  "unchanged",
		Message: "No sourcemux MCP config entry is present; no config file will be deleted.",
	}
	if status.EntryPresent {
		action.Status = "remove"
		action.Backup = plannedBackupPath(path)
		action.Message = configChangeMessage(target, "Remove only the sourcemux MCP config entry; unrelated keys and the config file will be preserved.")
	}
	return action, true, nil
}

func configStatusFor(target, scope, binary, configPath string) ConfigStatus {
	path, ok, err := mcpConfigPath(target, scope)
	if err != nil {
		return ConfigStatus{Supported: ok, Status: "error", Error: err.Error()}
	}
	if !ok {
		return ConfigStatus{Supported: false, Status: "unsupported", Message: "No verified safe MCP config writer exists for this target yet."}
	}
	status, err := readMCPConfigStatus(target, path, binary, configPath)
	if err != nil {
		return ConfigStatus{Supported: true, Path: path, Status: "parse-error", Error: err.Error()}
	}
	return status
}

func readMCPConfigStatus(target, path, binary, configPath string) (ConfigStatus, error) {
	entry, exists, present, err := readMCPConfigEntry(target, path)
	if err != nil {
		return ConfigStatus{}, err
	}
	status := ConfigStatus{
		Supported:    true,
		Path:         path,
		Exists:       exists,
		EntryPresent: present,
		Status:       "missing",
	}
	if !exists {
		status.Message = "Config file does not exist."
		return status, nil
	}
	if !present {
		status.Message = "Config file exists, but sourcemux entry is not present."
		return status, nil
	}
	if binary == "" && configPath == "" {
		status.Status = "present"
		status.Message = "SourceMux MCP config entry is present."
		return status, nil
	}
	desired := desiredMCPConfigEntry(target, binary, configPath)
	status.Matches = configEntriesEqual(entry, desired)
	status.Drifted = !status.Matches
	if status.Matches {
		status.Status = "matching"
		status.Message = "SourceMux MCP config entry matches the desired command and config path."
	} else {
		status.Status = "drifted"
		status.Message = "SourceMux MCP config entry differs from the desired command or config path."
	}
	return status, nil
}

func writeMCPConfig(target, path, binary, configPath, plannedBackup string) (string, string, error) {
	status, err := readMCPConfigStatus(target, path, binary, configPath)
	if err != nil {
		return "", "", err
	}
	if status.Exists && status.Matches {
		return "unchanged", "", nil
	}
	updated, err := renderMCPConfigWithEntry(target, path, desiredMCPConfigEntry(target, binary, configPath))
	if err != nil {
		return "", "", err
	}
	backup, mode, err := backupExistingConfig(path, plannedBackup)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, updated, mode); err != nil {
		return "", "", err
	}
	if status.Exists {
		return "updated", backup, nil
	}
	return "created", "", nil
}

func removeMCPConfig(target, path, plannedBackup string) (string, string, error) {
	status, err := readMCPConfigStatus(target, path, "", "")
	if err != nil {
		return "", "", err
	}
	if !status.EntryPresent {
		return "unchanged", "", nil
	}
	updated, err := renderMCPConfigWithoutEntry(target, path)
	if err != nil {
		return "", "", err
	}
	backup, mode, err := backupExistingConfig(path, plannedBackup)
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(path, updated, mode); err != nil {
		return "", "", err
	}
	return "removed", backup, nil
}

func mcpConfigPath(target, scope string) (string, bool, error) {
	var path string
	switch target {
	case "codex":
		path = "~/.codex/config.toml"
		if scope == "project" {
			path = ".codex/config.toml"
		}
	case "gemini":
		path = "~/.gemini/settings.json"
		if scope == "project" {
			path = ".gemini/settings.json"
		}
	case "opencode":
		path = "~/.config/opencode/opencode.json"
		if scope == "project" {
			path = "opencode.json"
		}
	default:
		return "", false, nil
	}
	expanded, err := expandPath(path)
	if err != nil {
		return "", true, err
	}
	return expanded, true, nil
}

func desiredMCPConfigEntry(target, binary, configPath string) map[string]any {
	switch target {
	case "opencode":
		return map[string]any{
			"type":    "local",
			"command": []string{binary, "--config", configPath},
			"enabled": true,
		}
	default:
		return map[string]any{
			"command": binary,
			"args":    []string{"--config", configPath},
		}
	}
}

func readMCPConfigEntry(target, path string) (map[string]any, bool, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, false, nil
		}
		return nil, false, false, err
	}
	root, err := parseMCPConfig(target, data)
	if err != nil {
		return nil, true, false, fmt.Errorf("parse %s: %w", path, err)
	}
	parentKey := mcpConfigParentKey(target)
	parentValue, present := root[parentKey]
	if !present {
		return nil, true, false, nil
	}
	parent, ok := parentValue.(map[string]any)
	if !ok {
		return nil, true, false, fmt.Errorf("parse %s: %s must be an object/table to preserve existing config", path, parentKey)
	}
	childValue, present := parent["sourcemux"]
	if !present {
		return nil, true, false, nil
	}
	entry, ok := childValue.(map[string]any)
	if !ok {
		return map[string]any{"__invalid_entry_type": fmt.Sprintf("%T", childValue)}, true, true, nil
	}
	return entry, true, true, nil
}

func renderMCPConfigWithEntry(target, path string, entry map[string]any) ([]byte, error) {
	root, err := readMCPConfigRoot(target, path)
	if err != nil {
		return nil, err
	}
	parent, err := ensureNestedMap(root, mcpConfigParentKey(target))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	parent["sourcemux"] = entry
	return marshalMCPConfig(target, root)
}

func renderMCPConfigWithoutEntry(target, path string) ([]byte, error) {
	root, err := readMCPConfigRoot(target, path)
	if err != nil {
		return nil, err
	}
	parent, ok := root[mcpConfigParentKey(target)].(map[string]any)
	if ok {
		delete(parent, "sourcemux")
	}
	return marshalMCPConfig(target, root)
}

func readMCPConfigRoot(target, path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]any{}, nil
		}
		return nil, err
	}
	root, err := parseMCPConfig(target, data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return root, nil
}

func parseMCPConfig(target string, data []byte) (map[string]any, error) {
	root := map[string]any{}
	if len(bytes.TrimSpace(data)) == 0 {
		return root, nil
	}
	switch target {
	case "codex":
		if err := toml.Unmarshal(data, &root); err != nil {
			return nil, err
		}
	case "opencode":
		cleaned, err := jsoncToJSON(data)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(cleaned, &root); err != nil {
			return nil, err
		}
	default:
		if err := json.Unmarshal(data, &root); err != nil {
			return nil, err
		}
	}
	return root, nil
}

func marshalMCPConfig(target string, root map[string]any) ([]byte, error) {
	var (
		data []byte
		err  error
	)
	if target == "codex" {
		data, err = toml.Marshal(root)
	} else {
		data, err = json.MarshalIndent(root, "", "  ")
	}
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func mcpConfigParentKey(target string) string {
	if target == "codex" {
		return "mcp_servers"
	}
	if target == "opencode" {
		return "mcp"
	}
	return "mcpServers"
}

func ensureNestedMap(root map[string]any, key string) (map[string]any, error) {
	if existing, ok := root[key].(map[string]any); ok {
		return existing, nil
	}
	if _, exists := root[key]; exists {
		return nil, fmt.Errorf("%s must be an object/table to preserve existing config", key)
	}
	created := map[string]any{}
	root[key] = created
	return created, nil
}

func configEntriesEqual(a, b map[string]any) bool {
	return reflect.DeepEqual(normalizeConfigValue(a), normalizeConfigValue(b))
}

func normalizeConfigValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, item := range typed {
			out[key] = normalizeConfigValue(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, normalizeConfigValue(item))
		}
		return out
	case []string:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return typed
	}
}

func backupExistingConfig(path, plannedBackup string) (string, fs.FileMode, error) {
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", 0o600, nil
		}
		return "", 0, err
	}
	backup := plannedBackup
	if backup == "" {
		backup = plannedBackupPath(path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	if err := os.WriteFile(backup, data, info.Mode().Perm()); err != nil {
		return "", 0, fmt.Errorf("write backup %s: %w", backup, err)
	}
	return backup, info.Mode().Perm(), nil
}

func plannedBackupPath(path string) string {
	return fmt.Sprintf("%s.bak.%s", path, time.Now().UTC().Format("20060102T150405.000000000Z"))
}

func jsoncToJSON(data []byte) ([]byte, error) {
	withoutComments, err := stripJSONCComments(data)
	if err != nil {
		return nil, err
	}
	return stripTrailingJSONCommas(withoutComments), nil
}

func stripJSONCComments(data []byte) ([]byte, error) {
	var out bytes.Buffer
	inString := false
	escaped := false
	for i := 0; i < len(data); i++ {
		ch := data[i]
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out.WriteByte(ch)
			continue
		}
		if ch == '/' && i+1 < len(data) {
			next := data[i+1]
			if next == '/' {
				for i < len(data) && data[i] != '\n' {
					i++
				}
				if i < len(data) {
					out.WriteByte('\n')
				}
				continue
			}
			if next == '*' {
				i += 2
				closed := false
				for i < len(data)-1 {
					if data[i] == '\n' {
						out.WriteByte('\n')
					}
					if data[i] == '*' && data[i+1] == '/' {
						closed = true
						i++
						break
					}
					i++
				}
				if !closed {
					return nil, fmt.Errorf("unterminated JSONC block comment")
				}
				continue
			}
		}
		out.WriteByte(ch)
	}
	if inString {
		return nil, fmt.Errorf("unterminated JSON string")
	}
	return out.Bytes(), nil
}

func stripTrailingJSONCommas(data []byte) []byte {
	var out bytes.Buffer
	inString := false
	escaped := false
	for i := 0; i < len(data); i++ {
		ch := data[i]
		if inString {
			out.WriteByte(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			out.WriteByte(ch)
			continue
		}
		if ch == ',' {
			j := i + 1
			for j < len(data) && strings.ContainsRune(" \t\r\n", rune(data[j])) {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				continue
			}
		}
		out.WriteByte(ch)
	}
	return out.Bytes()
}
