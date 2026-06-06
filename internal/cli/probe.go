package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/engine"
)

type probeEndpoint struct {
	Name           string   `json:"name"`
	BaseURL        string   `json:"base_url"`
	Model          string   `json:"model"`
	APIType        string   `json:"api_type,omitempty"`
	Profile        string   `json:"profile,omitempty"`
	SendSearchFlag bool     `json:"send_search_flag"`
	ResponseTools  []string `json:"response_tools,omitempty"`
	OK             bool     `json:"ok"`
	DurationMS     int64    `json:"duration_ms"`
	ModelsCount    int      `json:"models_count"`
	Models         []string `json:"models,omitempty"`
	Error          string   `json:"error,omitempty"`
}

type probeOutput struct {
	TavilyEnabled    bool                   `json:"tavily_enabled"`
	TavilyAPIURL     string                 `json:"tavily_api_url"`
	TavilyKey        string                 `json:"tavily_key_status"`
	FirecrawlEnabled bool                   `json:"firecrawl_enabled"`
	FirecrawlAPIURL  string                 `json:"firecrawl_api_url"`
	FirecrawlKey     string                 `json:"firecrawl_key_status"`
	FirecrawlKeys    []configNamedKeyOutput `json:"firecrawl_keys"`
	WebFetchOrder    []string               `json:"web_fetch_order,omitempty"`
	ExaEnabled       bool                   `json:"exa_enabled"`
	ExaAPIURL        string                 `json:"exa_api_url"`
	ExaKey           string                 `json:"exa_key_status"`
	JinaAPIURL       string                 `json:"jina_api_url"`
	JinaKey          string                 `json:"jina_key_status"`
	Debug            bool                   `json:"debug"`
	Endpoints        []probeEndpoint        `json:"endpoints"`
}

func runProbe(args []string) int {
	return runProbeNamed("probe", args)
}

func runProbeNamed(name string, args []string) int {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	timeout := fs.Duration("list-timeout", 15*time.Second, "Per-endpoint /models timeout")
	previewMax := fs.Int("preview", 8, "Max models listed per endpoint")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	cfg, err := loadConfig()
	if err != nil {
		if *jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": fmt.Sprintf("config: %v", err)})
		} else {
			fmt.Fprintf(os.Stderr, "config: %v\n", err)
		}
		return 1
	}

	out := probeOutput{
		TavilyEnabled:    cfg.TavilyEnabled,
		TavilyAPIURL:     cfg.TavilyAPIURL,
		TavilyKey:        keyStatus(cfg.TavilyAPIKey),
		FirecrawlEnabled: cfg.FirecrawlEnabled,
		FirecrawlAPIURL:  cfg.FirecrawlAPIURL,
		FirecrawlKey:     keyStatus(cfg.FirecrawlAPIKey),
		FirecrawlKeys:    []configNamedKeyOutput{},
		WebFetchOrder:    append([]string(nil), cfg.WebFetchOrder...),
		ExaEnabled:       cfg.ExaEnabled,
		ExaAPIURL:        cfg.ExaAPIURL,
		ExaKey:           keyStatus(cfg.ExaAPIKey),
		JinaAPIURL:       cfg.JinaAPIURL,
		JinaKey:          keyStatus(cfg.JinaAPIKey),
		Debug:            cfg.Debug,
	}
	for _, key := range cfg.FirecrawlKeys {
		out.FirecrawlKeys = append(out.FirecrawlKeys, configNamedKeyOutput{
			Name:      key.Name,
			KeyStatus: keyStatus(key.APIKey),
		})
	}

	pool := engine.NewGrokPool(cfg.GrokEndpoints)
	for _, c := range pool.Clients() {
		ep := probeEndpoint{
			Name:           c.Name,
			BaseURL:        c.BaseURL,
			Model:          c.Model,
			APIType:        c.APIType,
			Profile:        c.EffectiveProfile(),
			SendSearchFlag: c.SendSearchFlag,
		}
		if c.APIType == "responses" && c.SendSearchFlag {
			ep.ResponseTools = engine.EffectiveResponseTools(c.ResponseTools)
		}
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		start := time.Now()
		models, err := c.ListModels(ctx)
		cancel()
		ep.DurationMS = time.Since(start).Milliseconds()
		if err != nil {
			ep.Error = err.Error()
		} else {
			ep.OK = true
			ep.ModelsCount = len(models)
			if *previewMax > 0 && len(models) > *previewMax {
				ep.Models = models[:*previewMax]
			} else {
				ep.Models = models
			}
		}
		out.Endpoints = append(out.Endpoints, ep)
	}

	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}

	fmt.Println("=== SourceMux Config ===")
	fmt.Printf("Tavily Enabled: %v\n", out.TavilyEnabled)
	fmt.Printf("Tavily API URL: %s\n", out.TavilyAPIURL)
	fmt.Printf("Tavily API Key: %s\n", out.TavilyKey)
	fmt.Printf("Firecrawl Enabled: %v\n", out.FirecrawlEnabled)
	fmt.Printf("Firecrawl API URL: %s\n", out.FirecrawlAPIURL)
	fmt.Printf("Firecrawl API Key: %s\n", out.FirecrawlKey)
	fmt.Printf("Firecrawl Keys: %s\n", formatNamedKeyStatuses(out.FirecrawlKeys))
	if len(out.WebFetchOrder) > 0 {
		fmt.Printf("Web Fetch Order: %s\n", strings.Join(out.WebFetchOrder, " -> "))
	}
	fmt.Printf("Exa Enabled: %v\n", out.ExaEnabled)
	fmt.Printf("Exa API URL: %s\n", out.ExaAPIURL)
	fmt.Printf("Exa API Key: %s\n", out.ExaKey)
	fmt.Printf("Jina Reader URL: %s\n", out.JinaAPIURL)
	fmt.Printf("Jina API Key: %s\n", out.JinaKey)
	fmt.Printf("Debug: %v\n", out.Debug)
	fmt.Printf("\n=== Grok Endpoint Pool (%d configured) ===\n", len(out.Endpoints))
	for i, ep := range out.Endpoints {
		fmt.Printf("\n[%d] %s\n  Base URL: %s\n  Model:    %s\n",
			i+1, ep.Name, ep.BaseURL, ep.Model)
		fmt.Printf("  Profile:  %s\n", ep.Profile)
		if ep.APIType != "" {
			fmt.Printf("  API type: %s\n", ep.APIType)
		}
		fmt.Printf("  Send search flag/tools: %v\n", ep.SendSearchFlag)
		if len(ep.ResponseTools) > 0 {
			fmt.Printf("  Response tools: %s\n", strings.Join(ep.ResponseTools, ", "))
		}
		if ep.OK {
			fmt.Printf("  Probe:    OK (%dms, %d models)\n", ep.DurationMS, ep.ModelsCount)
			if len(ep.Models) > 0 {
				fmt.Printf("  Models:   %s", strings.Join(ep.Models, ", "))
				if ep.ModelsCount > len(ep.Models) {
					fmt.Printf(" \u2026 (+%d more)", ep.ModelsCount-len(ep.Models))
				}
				fmt.Println()
			}
		} else {
			fmt.Printf("  Probe:    FAILED (%s)\n", ep.Error)
		}
	}
	return 0
}

// keyStatus returns a masked, human-friendly description of an API key.
func keyStatus(key string) string {
	if key == "" {
		return "(not set)"
	}
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
