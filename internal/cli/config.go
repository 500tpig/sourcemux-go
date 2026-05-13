package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	cfgpkg "github.com/500tpig/grok-search-go/internal/config"
	"github.com/500tpig/grok-search-go/internal/engine"
)

const configUsage = `Usage: grok-search cli config <command> [flags]

Commands:
  path               Show the active config file path.
  files              Show the one config file grok-search will read.
  list               Show the effective loaded config with secrets masked.

Flags:
  --json             Emit machine-readable JSON.
  --help, -h         Show this usage.

Examples:
  grok-search cli config path
  grok-search cli --config ./prod.grok-search.json config path --json
  grok-search cli config files --json
  grok-search cli config list --json
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

type configListOutput struct {
	Paths configPathOutput `json:"paths"`

	GrokEndpoints      []configEndpointOutput `json:"grok_endpoints"`
	ReasoningEndpoints []configEndpointOutput `json:"reasoning_endpoints"`

	TavilyEnabled bool   `json:"tavily_enabled"`
	TavilyAPIURL  string `json:"tavily_api_url"`
	TavilyKey     string `json:"tavily_key_status"`

	ExaEnabled bool   `json:"exa_enabled"`
	ExaAPIURL  string `json:"exa_api_url"`
	ExaKey     string `json:"exa_key_status"`

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
	default:
		fmt.Fprintf(os.Stderr, "unknown config subcommand %q\n\n%s", cmd, configUsage)
		return 2
	}
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

	fmt.Println("=== Grok Search Effective Config ===")
	fmt.Printf("Config file: %s\n", out.Paths.ConfigFile)
	fmt.Printf("Absolute:    %s\n", out.Paths.AbsConfigFile)
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
		Role:    "the only config file loaded by grok-search",
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
			"Hidden files under ~/.config/grok-search and legacy endpoints.json are ignored.",
			"Use the global --config flag to select a different single JSON file.",
			fmt.Sprintf("Run `%s` to create the file without hand-writing JSON.", setupCmd),
		},
	}
}

func buildConfigListOutput(cfg *cfgpkg.Config) configListOutput {
	out := configListOutput{
		Paths:              buildConfigPathOutput(),
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
	return out
}

func reportConfigErr(jsonOut bool, message string) int {
	path := currentConfigPath()
	setupCmd := scopedConfigCLICommand(path, "setup")
	pathCmd := scopedConfigCLICommand(path, "config path")
	nextSteps := []string{
		fmt.Sprintf("Run `%s` to create the active config file without hand-writing JSON.", setupCmd),
		"Or create one JSON file at the active path and pass the same path with `--config` when it is not ./grok-search.json.",
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
		return "grok-search cli " + command
	}
	return "grok-search cli --config " + shellQuote(path) + " " + command
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
