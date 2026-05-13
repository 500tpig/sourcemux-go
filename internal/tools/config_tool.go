package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RegisterConfig registers the get_config_info diagnostic tool.
// It lists every endpoint in the Grok pool and probes each via /models.
func RegisterConfig(s *mcpserver.MCPServer, cfg *config.Config, pool *engine.GrokPool) {
	tool := mcp.NewTool("get_config_info",
		mcp.WithDescription("Show current configuration and probe each configured Grok endpoint."),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var sb strings.Builder

		sb.WriteString("=== SourceMux Config ===")
		sb.WriteString(fmt.Sprintf("\nTavily Enabled: %v", cfg.TavilyEnabled))
		sb.WriteString(fmt.Sprintf("\nTavily API URL: %s", cfg.TavilyAPIURL))
		sb.WriteString(fmt.Sprintf("\nTavily API Key: %s", optionalKeyStatus(cfg.TavilyAPIKey)))
		sb.WriteString(fmt.Sprintf("\nExa Enabled: %v", cfg.ExaEnabled))
		sb.WriteString(fmt.Sprintf("\nExa API URL: %s", cfg.ExaAPIURL))
		sb.WriteString(fmt.Sprintf("\nExa API Key: %s", optionalKeyStatus(cfg.ExaAPIKey)))
		sb.WriteString(fmt.Sprintf("\nJina Reader URL: %s", cfg.JinaAPIURL))
		sb.WriteString(fmt.Sprintf("\nJina API Key: %s", optionalKeyStatus(cfg.JinaAPIKey)))
		sb.WriteString(fmt.Sprintf("\nTinyFish Enabled: %v", cfg.TinyFishEnabled))
		sb.WriteString(fmt.Sprintf("\nTinyFish Search URL: %s", cfg.TinyFishSearchURL))
		sb.WriteString(fmt.Sprintf("\nTinyFish Fetch URL: %s", cfg.TinyFishFetchURL))
		sb.WriteString(fmt.Sprintf("\nTinyFish Keys: %d configured", len(cfg.TinyFishKeys)))
		for i, key := range cfg.TinyFishKeys {
			sb.WriteString(fmt.Sprintf("\n    [%d] %s: %s", i+1, key.Name, maskKey(key.APIKey)))
		}
		sb.WriteString(fmt.Sprintf("\nReasoning Endpoints: %d configured", len(cfg.ReasoningEndpoints)))
		for i, ep := range cfg.ReasoningEndpoints {
			sb.WriteString(fmt.Sprintf("\n    [%d] %s", i+1, ep.Name))
			sb.WriteString(fmt.Sprintf("\n        Base URL: %s", ep.BaseURL))
			sb.WriteString(fmt.Sprintf("\n        API Key:  %s", maskKey(ep.APIKey)))
			sb.WriteString(fmt.Sprintf("\n        Model:    %s", ep.Model))
		}
		sb.WriteString(fmt.Sprintf("\nDebug: %v", cfg.Debug))

		sb.WriteString(fmt.Sprintf("\n\n=== Grok Endpoint Pool (%d configured, in priority order) ===", pool.Len()))
		for i, c := range pool.Clients() {
			sb.WriteString(fmt.Sprintf("\n\n[%d] %s", i+1, c.Name))
			sb.WriteString(fmt.Sprintf("\n    Base URL: %s", c.BaseURL))
			sb.WriteString(fmt.Sprintf("\n    API Key:  %s", maskKey(c.APIKey)))
			sb.WriteString(fmt.Sprintf("\n    Model:    %s", c.Model))
			apiType := c.APIType
			if apiType == "" {
				apiType = "chat"
			}
			sb.WriteString(fmt.Sprintf("\n    API type: %s", apiType))
			sb.WriteString(fmt.Sprintf("\n    Send search flag/tools: %v", c.SendSearchFlag))
			if c.APIType == "responses" && c.SendSearchFlag {
				sb.WriteString(fmt.Sprintf("\n    Response tools: %s", strings.Join(engine.EffectiveResponseTools(c.ResponseTools), ", ")))
			}

			start := time.Now()
			models, err := c.ListModels(ctx)
			duration := time.Since(start)
			if err != nil {
				sb.WriteString(fmt.Sprintf("\n    Probe:    FAILED (%v)", err))
				continue
			}
			sb.WriteString(fmt.Sprintf("\n    Probe:    OK (%dms, %d models)", duration.Milliseconds(), len(models)))
			if len(models) > 0 {
				preview := models
				if len(preview) > 8 {
					preview = preview[:8]
				}
				sb.WriteString(fmt.Sprintf("\n    Models:   %s", strings.Join(preview, ", ")))
				if len(models) > 8 {
					sb.WriteString(fmt.Sprintf(" \u2026 (+%d more)", len(models)-8))
				}
			}
		}

		return mcp.NewToolResultText(sb.String()), nil
	})
}

func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "..." + key[len(key)-4:]
}

func optionalKeyStatus(key string) string {
	if key == "" {
		return "(not set)"
	}
	return maskKey(key)
}
