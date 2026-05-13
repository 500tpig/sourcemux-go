package cli

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/500tpig/grok-search-go/internal/engine"
)

type setupConfigFile struct {
	GrokEndpoints      []engine.GrokEndpoint `json:"grokEndpoints"`
	Tavily             *setupServiceConfig   `json:"tavily,omitempty"`
	Exa                *setupServiceConfig   `json:"exa,omitempty"`
	Context7           *setupServiceConfig   `json:"context7,omitempty"`
	Jina               *setupServiceConfig   `json:"jina,omitempty"`
	TinyFish           *setupTinyFishConfig  `json:"tinyfish,omitempty"`
	GrokPoolTimeoutSec int                   `json:"grokPoolTimeoutSec,omitempty"`
	LogLevel           string                `json:"logLevel,omitempty"`
}

type setupServiceConfig struct {
	APIURL  string `json:"apiURL,omitempty"`
	APIKey  string `json:"apiKey,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

type setupTinyFishConfig struct {
	Enabled   *bool                `json:"enabled,omitempty"`
	Keys      []engine.TinyFishKey `json:"keys,omitempty"`
	SearchURL string               `json:"searchURL,omitempty"`
	FetchURL  string               `json:"fetchURL,omitempty"`
}

type setupOutput struct {
	ConfigFile   string                 `json:"config_file"`
	Endpoint     configEndpointOutput   `json:"endpoint"`
	TavilyKey    string                 `json:"tavily_key_status,omitempty"`
	ExaKey       string                 `json:"exa_key_status,omitempty"`
	Context7Key  string                 `json:"context7_key_status,omitempty"`
	JinaKey      string                 `json:"jina_key_status,omitempty"`
	TinyFishKeys []configNamedKeyOutput `json:"tinyfish_keys,omitempty"`
	NextSteps    []string               `json:"next_steps"`
}

type setupOptions struct {
	NonInteractive bool
	Force          bool
	JSONOut        bool

	Name           string
	APIURL         string
	APIKey         string
	Model          string
	APIType        string
	SendSearchFlag bool
	ResponseTools  string

	TavilyKey     string
	ExaKey        string
	Context7Key   string
	JinaKey       string
	TinyFishKeys  string
	TinyFishNames string
}

func runSetup(args []string) int {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := setupOptions{}
	fs.BoolVar(&opts.NonInteractive, "non-interactive", false, "Do not prompt; require required flags")
	fs.BoolVar(&opts.Force, "force", false, "Overwrite existing grok-search.json")
	fs.BoolVar(&opts.JSONOut, "json", false, "Emit JSON")
	fs.StringVar(&opts.Name, "name", "primary", "Endpoint display name")
	fs.StringVar(&opts.APIURL, "api-url", "", "Grok-compatible base URL, e.g. https://api.x.ai/v1")
	fs.StringVar(&opts.APIKey, "api-key", "", "Grok-compatible API key")
	fs.StringVar(&opts.Model, "model", "grok-4.20-fast", "Default model")
	fs.StringVar(&opts.APIType, "api-type", "chat", `API protocol: "chat" or "responses"`)
	fs.BoolVar(&opts.SendSearchFlag, "send-search-flag", false, "Enable provider-specific web-search flag/tools")
	fs.StringVar(&opts.ResponseTools, "response-tools", "", "Comma-separated Responses API tools, e.g. web_search,x_search")
	fs.StringVar(&opts.TavilyKey, "tavily-key", "", "Optional Tavily API key")
	fs.StringVar(&opts.ExaKey, "exa-key", "", "Optional Exa API key")
	fs.StringVar(&opts.Context7Key, "context7-key", "", "Optional Context7 API key")
	fs.StringVar(&opts.JinaKey, "jina-key", "", "Optional Jina API key")
	fs.StringVar(&opts.TinyFishKeys, "tinyfish-keys", "", "Optional comma-separated TinyFish API keys")
	fs.StringVar(&opts.TinyFishNames, "tinyfish-key-names", "", "Optional comma-separated TinyFish key display names")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	out, err := runSetupWithIO(opts, os.Stdin, os.Stderr)
	if err != nil {
		return reportSetupErr(opts.JSONOut, err.Error())
	}
	if opts.JSONOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	fmt.Fprintf(os.Stdout, "Wrote config: %s\n", out.ConfigFile)
	fmt.Fprintf(os.Stdout, "Endpoint: %s (%s, %s, key=%s)\n",
		out.Endpoint.Name, out.Endpoint.BaseURL, out.Endpoint.Model, out.Endpoint.KeyStatus)
	if out.Context7Key != "" {
		fmt.Fprintf(os.Stdout, "Context7: key=%s\n", out.Context7Key)
	}
	for _, step := range out.NextSteps {
		fmt.Fprintf(os.Stdout, "- %s\n", step)
	}
	return 0
}

func runSetupWithIO(opts setupOptions, in io.Reader, promptOut io.Writer) (setupOutput, error) {
	if !opts.NonInteractive {
		if err := promptSetupOptions(&opts, in, promptOut); err != nil {
			return setupOutput{}, err
		}
	}
	if err := validateSetupOptions(opts); err != nil {
		return setupOutput{}, err
	}

	path := currentConfigPath()
	if path == "" {
		return setupOutput{}, errors.New("could not resolve config path")
	}
	if !opts.Force {
		if _, err := os.Stat(path); err == nil {
			return setupOutput{}, fmt.Errorf("config file already exists: %s (pass --force to overwrite)", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			return setupOutput{}, fmt.Errorf("stat config file %s: %w", path, err)
		}
	}

	cfg := buildSetupConfig(opts)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return setupOutput{}, fmt.Errorf("encode config: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return setupOutput{}, fmt.Errorf("create config directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return setupOutput{}, fmt.Errorf("write config file %s: %w", path, err)
	}
	_ = os.Chmod(path, 0o600)

	return buildSetupOutput(path, cfg), nil
}

func promptSetupOptions(opts *setupOptions, in io.Reader, out io.Writer) error {
	reader := bufio.NewReader(in)
	var err error
	opts.Name, err = promptString(reader, out, "Endpoint name", opts.Name, false)
	if err != nil {
		return err
	}
	opts.APIURL, err = promptString(reader, out, "Grok API base URL", opts.APIURL, true)
	if err != nil {
		return err
	}
	opts.APIKey, err = promptString(reader, out, "Grok API key", opts.APIKey, true)
	if err != nil {
		return err
	}
	opts.Model, err = promptString(reader, out, "Default model", opts.Model, false)
	if err != nil {
		return err
	}
	opts.APIType, err = promptString(reader, out, "API type (chat/responses)", opts.APIType, false)
	if err != nil {
		return err
	}
	opts.SendSearchFlag, err = promptBool(reader, out, "Send search flag/tools", opts.SendSearchFlag)
	if err != nil {
		return err
	}
	opts.ResponseTools, err = promptString(reader, out, "Responses API tools (comma-separated, blank for web_search default)", opts.ResponseTools, false)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "\nDocs provider keys (optional)")
	opts.ExaKey, err = promptString(reader, out, "Exa API key", opts.ExaKey, false)
	if err != nil {
		return err
	}
	opts.Context7Key, err = promptString(reader, out, "Context7 API key", opts.Context7Key, false)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "\nFetch/search fallback provider keys (optional)")
	opts.TavilyKey, err = promptString(reader, out, "Tavily API key", opts.TavilyKey, false)
	if err != nil {
		return err
	}
	opts.JinaKey, err = promptString(reader, out, "Jina API key", opts.JinaKey, false)
	if err != nil {
		return err
	}
	opts.TinyFishKeys, err = promptString(reader, out, "TinyFish API keys (comma-separated)", opts.TinyFishKeys, false)
	if err != nil {
		return err
	}
	opts.TinyFishNames, err = promptString(reader, out, "TinyFish key names (comma-separated)", opts.TinyFishNames, false)
	return err
}

func promptString(reader *bufio.Reader, out io.Writer, label, current string, required bool) (string, error) {
	for {
		if current != "" {
			fmt.Fprintf(out, "%s [%s]: ", label, current)
		} else {
			fmt.Fprintf(out, "%s: ", label)
		}
		raw, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			value = current
		}
		if value != "" || !required {
			return value, nil
		}
		fmt.Fprintln(out, "Value is required.")
		if errors.Is(err, io.EOF) {
			return "", io.ErrUnexpectedEOF
		}
	}
}

func promptBool(reader *bufio.Reader, out io.Writer, label string, current bool) (bool, error) {
	defaultText := "false"
	if current {
		defaultText = "true"
	}
	for {
		fmt.Fprintf(out, "%s [%s]: ", label, defaultText)
		raw, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		value := strings.TrimSpace(raw)
		if value == "" {
			return current, nil
		}
		parsed, parseErr := strconv.ParseBool(value)
		if parseErr == nil {
			return parsed, nil
		}
		fmt.Fprintln(out, "Use true or false.")
		if errors.Is(err, io.EOF) {
			return false, io.ErrUnexpectedEOF
		}
	}
}

func validateSetupOptions(opts setupOptions) error {
	if strings.TrimSpace(opts.APIURL) == "" {
		return errors.New("--api-url is required (or run without --non-interactive to prompt)")
	}
	if strings.TrimSpace(opts.APIKey) == "" {
		return errors.New("--api-key is required (or run without --non-interactive to prompt)")
	}
	switch strings.TrimSpace(opts.APIType) {
	case "", "chat", "responses":
	default:
		return fmt.Errorf("--api-type must be chat or responses, got %q", opts.APIType)
	}
	responseTools, err := engine.NormalizeResponseTools(setupSplitCSV(opts.ResponseTools))
	if err != nil {
		return fmt.Errorf("--response-tools: %w", err)
	}
	if len(responseTools) > 0 && strings.TrimSpace(opts.APIType) != "responses" {
		return errors.New("--response-tools requires --api-type responses")
	}
	return nil
}

func buildSetupConfig(opts setupOptions) setupConfigFile {
	responseTools, _ := engine.NormalizeResponseTools(setupSplitCSV(opts.ResponseTools))
	endpoint := engine.GrokEndpoint{
		Name:           strings.TrimSpace(opts.Name),
		BaseURL:        strings.TrimSpace(opts.APIURL),
		APIKey:         strings.TrimSpace(opts.APIKey),
		Model:          strings.TrimSpace(opts.Model),
		APIType:        strings.TrimSpace(opts.APIType),
		SendSearchFlag: opts.SendSearchFlag,
		ResponseTools:  responseTools,
	}
	if endpoint.Name == "" {
		endpoint.Name = "primary"
	}
	if endpoint.Model == "" {
		endpoint.Model = "grok-4.20-fast"
	}

	cfg := setupConfigFile{
		GrokEndpoints: []engine.GrokEndpoint{endpoint},
		LogLevel:      "INFO",
	}
	if key := strings.TrimSpace(opts.TavilyKey); key != "" {
		enabled := true
		cfg.Tavily = &setupServiceConfig{APIURL: "https://api.tavily.com", APIKey: key, Enabled: &enabled}
	}
	if key := strings.TrimSpace(opts.ExaKey); key != "" {
		enabled := true
		cfg.Exa = &setupServiceConfig{APIURL: "https://api.exa.ai", APIKey: key, Enabled: &enabled}
	}
	if key := strings.TrimSpace(opts.Context7Key); key != "" {
		enabled := true
		cfg.Context7 = &setupServiceConfig{APIURL: engine.DefaultContext7APIURL, APIKey: key, Enabled: &enabled}
	}
	if key := strings.TrimSpace(opts.JinaKey); key != "" {
		cfg.Jina = &setupServiceConfig{APIURL: "https://r.jina.ai", APIKey: key}
	}
	if keys := setupSplitCSV(opts.TinyFishKeys); len(keys) > 0 {
		enabled := true
		names := setupSplitCSV(opts.TinyFishNames)
		tinyfish := setupTinyFishConfig{
			Enabled:   &enabled,
			SearchURL: engine.DefaultTinyFishSearchURL,
			FetchURL:  engine.DefaultTinyFishFetchURL,
		}
		for i, key := range keys {
			item := engine.TinyFishKey{APIKey: key}
			if i < len(names) {
				item.Name = names[i]
			}
			if item.Name == "" {
				item.Name = fmt.Sprintf("key-%d", i)
			}
			tinyfish.Keys = append(tinyfish.Keys, item)
		}
		cfg.TinyFish = &tinyfish
	}
	return cfg
}

func setupSplitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func buildSetupOutput(path string, cfg setupConfigFile) setupOutput {
	configFlag := ""
	if strings.TrimSpace(path) != "" && path != "grok-search.json" {
		configFlag = " --config " + shellQuote(path)
	}
	out := setupOutput{
		ConfigFile: path,
		NextSteps: []string{
			fmt.Sprintf("Run `grok-search cli%s config list --json` to inspect masked effective config.", configFlag),
			fmt.Sprintf("Run `grok-search cli%s doctor --json` to verify local config structure without provider requests.", configFlag),
			fmt.Sprintf("Run `grok-search cli%s search \"test query\" --json` to test search.", configFlag),
		},
	}
	if len(cfg.GrokEndpoints) > 0 {
		ep := cfg.GrokEndpoints[0]
		out.Endpoint = configEndpointOutput{
			Name:           ep.Name,
			BaseURL:        ep.BaseURL,
			Model:          ep.Model,
			APIType:        ep.APIType,
			SendSearchFlag: ep.SendSearchFlag,
			ResponseTools:  ep.ResponseTools,
			KeyStatus:      keyStatus(ep.APIKey),
		}
	}
	if cfg.Tavily != nil {
		out.TavilyKey = keyStatus(cfg.Tavily.APIKey)
	}
	if cfg.Exa != nil {
		out.ExaKey = keyStatus(cfg.Exa.APIKey)
	}
	if cfg.Context7 != nil {
		out.Context7Key = keyStatus(cfg.Context7.APIKey)
	}
	if cfg.Jina != nil {
		out.JinaKey = keyStatus(cfg.Jina.APIKey)
	}
	if cfg.TinyFish != nil {
		for _, key := range cfg.TinyFish.Keys {
			out.TinyFishKeys = append(out.TinyFishKeys, configNamedKeyOutput{
				Name:      key.Name,
				KeyStatus: keyStatus(key.APIKey),
			})
		}
	}
	return out
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.ContainsAny(value, " \t\n'\"\\$`") {
		return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
	}
	return value
}

func reportSetupErr(jsonOut bool, message string) int {
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"error": message,
			"next_steps": []string{
				"Run `grok-search cli setup --help` for available flags.",
				"Run `grok-search cli config path` to inspect the target path.",
			},
		})
		return 1
	}
	fmt.Fprintln(os.Stderr, message)
	fmt.Fprintln(os.Stderr, "- Run `grok-search cli setup --help` for available flags.")
	fmt.Fprintln(os.Stderr, "- Run `grok-search cli config path` to inspect the target path.")
	return 1
}
