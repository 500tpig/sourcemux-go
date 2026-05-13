package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cfgpkg "github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
)

const configUsage = `Usage: sourcemux cli config <command> [flags]

Commands:
  path               Show the active config file path.
  files              Show the one config file sourcemux will read.
  list               Show the effective loaded config with secrets masked.
  migrate            Rewrite a legacy v1 config to v2 capabilities format.

Flags:
  --json             Emit machine-readable JSON.
  --help, -h         Show this usage.

Examples:
  sourcemux cli config path
  sourcemux cli --config ./prod.sourcemux.json config path --json
  sourcemux cli config files --json
  sourcemux cli config list --json
`

type configPathOutput struct {
	ConfigFile    string `json:"config_file"`
	AbsConfigFile string `json:"abs_config_file"`
	Exists        bool   `json:"exists"`
	Size          int64  `json:"size,omitempty"`
	Mode          string `json:"mode,omitempty"`
}

type configFileStatus struct {
	Path    string `json:"path"`
	AbsPath string `json:"abs_path"`
	Exists  bool   `json:"exists"`
	Size    int64  `json:"size,omitempty"`
	Mode    string `json:"mode,omitempty"`
	Role    string `json:"role"`
}

type configFilesOutput struct {
	ConfigFile configFileStatus `json:"config_file"`
	Notes      []string         `json:"notes"`
}

type configEndpointOutput struct {
	Name           string   `json:"name"`
	BaseURL        string   `json:"base_url"`
	Model          string   `json:"model"`
	APIType        string   `json:"api_type,omitempty"`
	SendSearchFlag bool     `json:"send_search_flag"`
	ResponseTools  []string `json:"response_tools,omitempty"`
	KeyStatus      string   `json:"key_status"`
}

type configNamedKeyOutput struct {
	Name      string `json:"name"`
	KeyStatus string `json:"key_status"`
}

type configContext7Output struct {
	Name            string   `json:"name"`
	APIURL          string   `json:"api_url"`
	KeyStatus       string   `json:"key_status"`
	Priority        int      `json:"priority,omitempty"`
	LibraryScopes   []string `json:"library_scopes,omitempty"`
	MonthlyBudget   int      `json:"monthly_budget,omitempty"`
	CooldownSeconds int      `json:"cooldown_on_rate_limit_seconds,omitempty"`
}

type configListOutput struct {
	Paths          configPathOutput `json:"paths"`
	Version        int              `json:"version"`
	MinimumProfile string           `json:"minimum_profile"`

	GrokEndpoints      []configEndpointOutput `json:"grok_endpoints"`
	ReasoningEndpoints []configEndpointOutput `json:"reasoning_endpoints"`

	TavilyEnabled bool   `json:"tavily_enabled"`
	TavilyAPIURL  string `json:"tavily_api_url"`
	TavilyKey     string `json:"tavily_key_status"`

	ExaEnabled bool   `json:"exa_enabled"`
	ExaAPIURL  string `json:"exa_api_url"`
	ExaKey     string `json:"exa_key_status"`

	Context7Endpoints []configContext7Output `json:"context7_endpoints"`

	JinaAPIURL string `json:"jina_api_url"`
	JinaKey    string `json:"jina_key_status"`

	TinyFishEnabled   bool                   `json:"tinyfish_enabled"`
	TinyFishSearchURL string                 `json:"tinyfish_search_url"`
	TinyFishFetchURL  string                 `json:"tinyfish_fetch_url"`
	TinyFishKeys      []configNamedKeyOutput `json:"tinyfish_keys"`

	Debug              bool   `json:"debug"`
	LogLevel           string `json:"log_level"`
	GrokPoolTimeoutSec int64  `json:"grok_pool_timeout_sec"`
}

func runConfig(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(os.Stdout, configUsage)
		return 0
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "path":
		return runConfigPath(rest)
	case "files":
		return runConfigFiles(rest)
	case "list":
		return runConfigList(rest)
	case "migrate":
		return runConfigMigrate(rest)
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand %q\n\n%s", cmd, configUsage)
		return 2
	}
}

type configMigrateOutput struct {
	ConfigFile string `json:"config_file"`
	BackupFile string `json:"backup_file,omitempty"`
	Changed    bool   `json:"changed"`
	Message    string `json:"message"`
}

func runConfigMigrate(args []string) int {
	fs := flag.NewFlagSet("config migrate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "Emit JSON")
	backupPath := fs.String("backup", "", "Backup path (default: <config>.bak)")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	path := currentConfigPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return reportConfigErr(*jsonOut, fmt.Sprintf("read config file %s: %v", path, err))
	}
	var marker map[string]json.RawMessage
	if err := json.Unmarshal(raw, &marker); err != nil {
		return reportConfigErr(*jsonOut, fmt.Sprintf("parse config file %s: %v", path, err))
	}
	if _, ok := marker["capabilities"]; ok {
		return emitConfigMigrate(*jsonOut, configMigrateOutput{
			ConfigFile: path,
			Changed:    false,
			Message:    "config already uses v2 capabilities",
		})
	}

	cfg, err := loadConfig()
	if err != nil {
		return reportConfigErr(*jsonOut, fmt.Sprintf("config: %v", err))
	}
	backup := strings.TrimSpace(*backupPath)
	if backup == "" {
		backup = path + ".bak"
	}
	if err := os.WriteFile(backup, raw, 0o600); err != nil {
		return reportConfigErr(*jsonOut, fmt.Sprintf("write backup %s: %v", backup, err))
	}
	_ = os.Chmod(backup, 0o600)

	data, err := json.MarshalIndent(buildV2ConfigMap(cfg), "", "  ")
	if err != nil {
		return reportConfigErr(*jsonOut, fmt.Sprintf("encode migrated config: %v", err))
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return reportConfigErr(*jsonOut, fmt.Sprintf("write migrated config %s: %v", path, err))
	}
	_ = os.Chmod(path, 0o600)

	return emitConfigMigrate(*jsonOut, configMigrateOutput{
		ConfigFile: path,
		BackupFile: backup,
		Changed:    true,
		Message:    "config migrated to v2 capabilities",
	})
}

func buildV2ConfigMap(cfg *cfgpkg.Config) map[string]any {
	mainProviders := make([]map[string]any, 0, 4)
	if len(cfg.GrokEndpoints) > 0 {
		mainProviders = append(mainProviders, map[string]any{
			"type":      "grok-pool",
			"name":      "grok-pool",
			"endpoints": cfg.GrokEndpoints,
		})
	}
	if cfg.TinyFishEnabled && len(cfg.TinyFishKeys) > 0 {
		mainProviders = append(mainProviders, tinyFishProviderMap(cfg))
	}
	if cfg.ExaEnabled && cfg.ExaAPIKey != "" {
		mainProviders = append(mainProviders, exaProviderMap(cfg))
	}
	if cfg.TavilyEnabled && cfg.TavilyAPIKey != "" {
		mainProviders = append(mainProviders, tavilyProviderMap(cfg))
	}

	docsProviders := []map[string]any{}
	if cfg.ExaEnabled && cfg.ExaAPIKey != "" {
		docsProviders = append(docsProviders, exaProviderMap(cfg))
	}
	for _, ep := range cfg.Context7Endpoints {
		docsProviders = append(docsProviders, context7ProviderMap(ep))
	}

	fetchProviders := []map[string]any{
		{
			"type":   "jina",
			"name":   "jina-reader",
			"apiURL": cfg.JinaAPIURL,
			"apiKey": cfg.JinaAPIKey,
		},
	}
	if cfg.TinyFishEnabled && len(cfg.TinyFishKeys) > 0 {
		fetchProviders = append(fetchProviders, tinyFishProviderMap(cfg))
	}
	if cfg.ExaEnabled && cfg.ExaAPIKey != "" {
		fetchProviders = append(fetchProviders, exaProviderMap(cfg))
	}
	if cfg.TavilyEnabled && cfg.TavilyAPIKey != "" {
		fetchProviders = append(fetchProviders, tavilyProviderMap(cfg))
	}

	return map[string]any{
		"version":         2,
		"minimum_profile": "off",
		"capabilities": map[string]any{
			"main_search": map[string]any{"providers": mainProviders},
			"docs_search": map[string]any{"providers": docsProviders},
			"web_fetch":   map[string]any{"providers": fetchProviders},
			"web_enhance": map[string]any{"providers": []map[string]any{}},
		},
		"reasoning_endpoints": cfg.ReasoningEndpoints,
		"grokPoolTimeoutSec":  int64(cfg.GrokPoolTimeout.Seconds()),
		"logLevel":            cfg.LogLevel,
		"debug":               cfg.Debug,
	}
}

func exaProviderMap(cfg *cfgpkg.Config) map[string]any {
	return map[string]any{
		"type":    "exa",
		"name":    "exa-main",
		"apiURL":  cfg.ExaAPIURL,
		"apiKey":  cfg.ExaAPIKey,
		"enabled": cfg.ExaEnabled,
	}
}

func tavilyProviderMap(cfg *cfgpkg.Config) map[string]any {
	return map[string]any{
		"type":    "tavily",
		"name":    "tavily-main",
		"apiURL":  cfg.TavilyAPIURL,
		"apiKey":  cfg.TavilyAPIKey,
		"enabled": cfg.TavilyEnabled,
	}
}

func tinyFishProviderMap(cfg *cfgpkg.Config) map[string]any {
	return map[string]any{
		"type":      "tinyfish",
		"name":      "tinyfish-pool",
		"keys":      cfg.TinyFishKeys,
		"searchURL": cfg.TinyFishSearchURL,
		"fetchURL":  cfg.TinyFishFetchURL,
		"enabled":   cfg.TinyFishEnabled,
	}
}

func context7ProviderMap(ep engine.Context7Endpoint) map[string]any {
	return map[string]any{
		"type":                           "context7",
		"name":                           ep.Name,
		"apiURL":                         ep.APIURL,
		"apiKey":                         ep.APIKey,
		"priority":                       ep.Priority,
		"library_scopes":                 ep.LibraryScopes,
		"monthly_budget":                 ep.MonthlyBudget,
		"cooldown_on_rate_limit_seconds": ep.CooldownSeconds,
	}
}

func emitConfigMigrate(asJSON bool, out configMigrateOutput) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	fmt.Println(out.Message)
	fmt.Printf("Config file: %s\n", out.ConfigFile)
	if out.BackupFile != "" {
		fmt.Printf("Backup file: %s\n", out.BackupFile)
	}
	return 0
}

func runConfigPath(args []string) int {
	fs := flag.NewFlagSet("config path", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "Emit JSON")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	out := buildConfigPathOutput()
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	status := "missing"
	if out.Exists {
		status = fmt.Sprintf("exists size=%d mode=%s", out.Size, out.Mode)
	}
	fmt.Printf("Config file: %s\n", out.ConfigFile)
	fmt.Printf("Absolute:    %s\n", out.AbsConfigFile)
	fmt.Printf("Status:      %s\n", status)
	return 0
}

func runConfigFiles(args []string) int {
	fs := flag.NewFlagSet("config files", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "Emit JSON")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	out := buildConfigFilesOutput()
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	status := "missing"
	if out.ConfigFile.Exists {
		status = fmt.Sprintf("exists size=%d mode=%s", out.ConfigFile.Size, out.ConfigFile.Mode)
	}
	fmt.Printf("Config file: %s\n", out.ConfigFile.Path)
	fmt.Printf("Absolute:    %s\n", out.ConfigFile.AbsPath)
	fmt.Printf("Status:      %s\n", status)
	fmt.Printf("Role:        %s\n", out.ConfigFile.Role)
	if len(out.Notes) > 0 {
		fmt.Println("\nNotes:")
		for _, note := range out.Notes {
			fmt.Printf("  - %s\n", note)
		}
	}
	return 0
}

func runConfigList(args []string) int {
	fs := flag.NewFlagSet("config list", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jsonOut := fs.Bool("json", false, "Emit JSON")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		return reportConfigErr(*jsonOut, fmt.Sprintf("config: %v", err))
	}

	out := buildConfigListOutput(cfg)
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	fmt.Println("=== SourceMux Effective Config ===")
	fmt.Printf("Config file: %s\n", out.Paths.ConfigFile)
	fmt.Printf("Absolute:    %s\n", out.Paths.AbsConfigFile)
	fmt.Printf("Version:     %d\n", out.Version)
	fmt.Printf("Minimum profile: %s\n", out.MinimumProfile)
	fmt.Printf("\nGrok endpoints: %d\n", len(out.GrokEndpoints))
	for i, ep := range out.GrokEndpoints {
		apiType := ep.APIType
		if apiType == "" {
			apiType = "chat"
		}
		fmt.Printf("  [%d] %s\n", i+1, ep.Name)
		fmt.Printf("      Base URL: %s\n", ep.BaseURL)
		fmt.Printf("      Model: %s\n", ep.Model)
		fmt.Printf("      API type: %s\n", apiType)
		fmt.Printf("      Send search flag/tools: %v\n", ep.SendSearchFlag)
		if len(ep.ResponseTools) > 0 {
			fmt.Printf("      Response tools: %s\n", strings.Join(ep.ResponseTools, ", "))
		}
		fmt.Printf("      API key: %s\n", ep.KeyStatus)
	}
	fmt.Printf("\nReasoning endpoints: %d\n", len(out.ReasoningEndpoints))
	for i, ep := range out.ReasoningEndpoints {
		fmt.Printf("  [%d] %s\n", i+1, ep.Name)
		fmt.Printf("      Base URL: %s\n", ep.BaseURL)
		fmt.Printf("      Model: %s\n", ep.Model)
		fmt.Printf("      API key: %s\n", ep.KeyStatus)
	}
	fmt.Printf("\nTavily: enabled=%v url=%s key=%s\n", out.TavilyEnabled, out.TavilyAPIURL, out.TavilyKey)
	fmt.Printf("Exa:    enabled=%v url=%s key=%s\n", out.ExaEnabled, out.ExaAPIURL, out.ExaKey)
	fmt.Printf("Context7 endpoints: %d\n", len(out.Context7Endpoints))
	for i, ep := range out.Context7Endpoints {
		fmt.Printf("  [%d] %s url=%s key=%s\n", i+1, ep.Name, ep.APIURL, ep.KeyStatus)
	}
	fmt.Printf("Jina:   url=%s key=%s\n", out.JinaAPIURL, out.JinaKey)
	fmt.Printf("TinyFish: enabled=%v search=%s fetch=%s keys=%s\n",
		out.TinyFishEnabled, out.TinyFishSearchURL, out.TinyFishFetchURL, formatNamedKeyStatuses(out.TinyFishKeys))
	fmt.Printf("Debug: %v\n", out.Debug)
	fmt.Printf("Log level: %s\n", out.LogLevel)
	fmt.Printf("Grok pool timeout: %ds\n", out.GrokPoolTimeoutSec)
	return 0
}

func buildConfigPathOutput() configPathOutput {
	path := currentConfigPath()
	out := configPathOutput{
		ConfigFile:    path,
		AbsConfigFile: absPath(path),
	}
	if info, err := os.Stat(path); err == nil {
		out.Exists = true
		out.Size = info.Size()
		out.Mode = info.Mode().Perm().String()
	}
	return out
}

func buildConfigFilesOutput() configFilesOutput {
	path := currentConfigPath()
	setupCmd := scopedConfigCLICommand(path, "setup")
	status := configFileStatus{
		Path:    path,
		AbsPath: absPath(path),
		Role:    "the only config file loaded by sourcemux",
	}
	if info, err := os.Stat(path); err == nil {
		status.Exists = true
		status.Size = info.Size()
		status.Mode = info.Mode().Perm().String()
	}
	return configFilesOutput{
		ConfigFile: status,
		Notes: []string{
			"Only this file is loaded. There is no environment-variable config chain.",
			"Hidden files under ~/.config/sourcemux and legacy endpoints.json are ignored.",
			"Use the global --config flag to select a different single JSON file.",
			fmt.Sprintf("Run `%s` to create the file without hand-writing JSON.", setupCmd),
		},
	}
}

func buildConfigListOutput(cfg *cfgpkg.Config) configListOutput {
	out := configListOutput{
		Paths:              buildConfigPathOutput(),
		Version:            cfg.Version,
		MinimumProfile:     cfg.MinimumProfile,
		TavilyEnabled:      cfg.TavilyEnabled,
		TavilyAPIURL:       cfg.TavilyAPIURL,
		TavilyKey:          keyStatus(cfg.TavilyAPIKey),
		ExaEnabled:         cfg.ExaEnabled,
		ExaAPIURL:          cfg.ExaAPIURL,
		ExaKey:             keyStatus(cfg.ExaAPIKey),
		JinaAPIURL:         cfg.JinaAPIURL,
		JinaKey:            keyStatus(cfg.JinaAPIKey),
		TinyFishEnabled:    cfg.TinyFishEnabled,
		TinyFishSearchURL:  cfg.TinyFishSearchURL,
		TinyFishFetchURL:   cfg.TinyFishFetchURL,
		TinyFishKeys:       []configNamedKeyOutput{},
		Debug:              cfg.Debug,
		LogLevel:           cfg.LogLevel,
		GrokPoolTimeoutSec: int64(cfg.GrokPoolTimeout.Seconds()),
	}

	for _, ep := range cfg.GrokEndpoints {
		responseTools := ep.ResponseTools
		if ep.APIType == "responses" && ep.SendSearchFlag {
			responseTools = engine.EffectiveResponseTools(ep.ResponseTools)
		}
		out.GrokEndpoints = append(out.GrokEndpoints, configEndpointOutput{
			Name:           ep.Name,
			BaseURL:        ep.BaseURL,
			Model:          ep.Model,
			APIType:        ep.APIType,
			SendSearchFlag: ep.SendSearchFlag,
			ResponseTools:  responseTools,
			KeyStatus:      keyStatus(ep.APIKey),
		})
	}
	for _, ep := range cfg.ReasoningEndpoints {
		out.ReasoningEndpoints = append(out.ReasoningEndpoints, configEndpointOutput{
			Name:      ep.Name,
			BaseURL:   ep.BaseURL,
			Model:     ep.Model,
			KeyStatus: keyStatus(ep.APIKey),
		})
	}
	for _, key := range cfg.TinyFishKeys {
		out.TinyFishKeys = append(out.TinyFishKeys, configNamedKeyOutput{
			Name:      key.Name,
			KeyStatus: keyStatus(key.APIKey),
		})
	}
	for _, ep := range cfg.Context7Endpoints {
		out.Context7Endpoints = append(out.Context7Endpoints, configContext7Output{
			Name:            ep.Name,
			APIURL:          ep.APIURL,
			KeyStatus:       keyStatus(ep.APIKey),
			Priority:        ep.Priority,
			LibraryScopes:   ep.LibraryScopes,
			MonthlyBudget:   ep.MonthlyBudget,
			CooldownSeconds: ep.CooldownSeconds,
		})
	}
	return out
}

func reportConfigErr(jsonOut bool, message string) int {
	path := currentConfigPath()
	setupCmd := scopedConfigCLICommand(path, "setup")
	pathCmd := scopedConfigCLICommand(path, "config path")
	nextSteps := []string{
		fmt.Sprintf("Run `%s` to create the active config file without hand-writing JSON.", setupCmd),
		"Or create one JSON file at the active path and pass the same path with `--config` when it is not ./sourcemux.json.",
		fmt.Sprintf("Run `%s` to see the active target file.", pathCmd),
	}
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"error": message, "next_steps": nextSteps})
		return 1
	}
	fmt.Fprintln(os.Stderr, message)
	for _, step := range nextSteps {
		fmt.Fprintf(os.Stderr, "- %s\n", step)
	}
	return 1
}

func scopedConfigCLICommand(path, command string) string {
	if strings.TrimSpace(path) == "" || path == cfgpkg.DefaultConfigPath() {
		return "sourcemux cli " + command
	}
	return "sourcemux cli --config " + shellQuote(path) + " " + command
}

func formatNamedKeyStatuses(keys []configNamedKeyOutput) string {
	if len(keys) == 0 {
		return "(none)"
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s:%s", key.Name, key.KeyStatus))
	}
	return strings.Join(parts, ", ")
}

func absPath(path string) string {
	if strings.TrimSpace(path) == "" {
		path = cfgpkg.DefaultConfigPath()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return abs
}
