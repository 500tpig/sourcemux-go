package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bettas/grok-search-go/internal/engine"
)

type probeEndpoint struct {
	Name           string   `json:"name"`
	BaseURL        string   `json:"base_url"`
	Model          string   `json:"model"`
	SendSearchFlag bool     `json:"send_search_flag"`
	OK             bool     `json:"ok"`
	DurationMS     int64    `json:"duration_ms"`
	ModelsCount    int      `json:"models_count"`
	Models         []string `json:"models,omitempty"`
	Error          string   `json:"error,omitempty"`
}

type probeOutput struct {
	TavilyEnabled bool            `json:"tavily_enabled"`
	TavilyAPIURL  string          `json:"tavily_api_url"`
	TavilyKey     string          `json:"tavily_key_status"`
	ExaEnabled    bool            `json:"exa_enabled"`
	ExaAPIURL     string          `json:"exa_api_url"`
	ExaKey        string          `json:"exa_key_status"`
	JinaAPIURL    string          `json:"jina_api_url"`
	JinaKey       string          `json:"jina_key_status"`
	Debug         bool            `json:"debug"`
	Endpoints     []probeEndpoint `json:"endpoints"`
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
		TavilyEnabled: cfg.TavilyEnabled,
		TavilyAPIURL:  cfg.TavilyAPIURL,
		TavilyKey:     keyStatus(cfg.TavilyAPIKey),
		ExaEnabled:    cfg.ExaEnabled,
		ExaAPIURL:     cfg.ExaAPIURL,
		ExaKey:        keyStatus(cfg.ExaAPIKey),
		JinaAPIURL:    cfg.JinaAPIURL,
		JinaKey:       keyStatus(cfg.JinaAPIKey),
		Debug:         cfg.Debug,
	}

	pool := engine.NewGrokPool(cfg.GrokEndpoints)
	for _, c := range pool.Clients() {
		ep := probeEndpoint{
			Name:           c.Name,
			BaseURL:        c.BaseURL,
			Model:          c.Model,
			SendSearchFlag: c.SendSearchFlag,
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

	fmt.Println("=== Grok Search Config ===")
	fmt.Printf("Tavily Enabled: %v\n", out.TavilyEnabled)
	fmt.Printf("Tavily API URL: %s\n", out.TavilyAPIURL)
	fmt.Printf("Tavily API Key: %s\n", out.TavilyKey)
	fmt.Printf("Exa Enabled: %v\n", out.ExaEnabled)
	fmt.Printf("Exa API URL: %s\n", out.ExaAPIURL)
	fmt.Printf("Exa API Key: %s\n", out.ExaKey)
	fmt.Printf("Jina Reader URL: %s\n", out.JinaAPIURL)
	fmt.Printf("Jina API Key: %s\n", out.JinaKey)
	fmt.Printf("Debug: %v\n", out.Debug)
	fmt.Printf("\n=== Grok Endpoint Pool (%d configured) ===\n", len(out.Endpoints))
	for i, ep := range out.Endpoints {
		fmt.Printf("\n[%d] %s\n  Base URL: %s\n  Model:    %s\n  Send `search:true`: %v\n",
			i+1, ep.Name, ep.BaseURL, ep.Model, ep.SendSearchFlag)
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
