package tools

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bettas/grok-search-go/internal/config"
	"github.com/bettas/grok-search-go/internal/engine"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// RegisterConfig registers the get_config_info diagnostic tool.
func RegisterConfig(s *mcpserver.MCPServer, cfg *config.Config, grok *engine.GrokClient) {
	tool := mcp.NewTool("get_config_info",
		mcp.WithDescription("Show current configuration and test API connectivity."),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var sb strings.Builder

		sb.WriteString("=== Grok Search Config ===")
		sb.WriteString(fmt.Sprintf("\nGrok API URL: %s", cfg.GrokAPIURL))
		sb.WriteString(fmt.Sprintf("\nGrok API Key: %s", maskKey(cfg.GrokAPIKey)))
		sb.WriteString(fmt.Sprintf("\nGrok Model: %s", cfg.GrokModel))
		sb.WriteString(fmt.Sprintf("\nTavily Enabled: %v", cfg.TavilyEnabled))
		sb.WriteString(fmt.Sprintf("\nTavily API URL: %s", cfg.TavilyAPIURL))
		sb.WriteString(fmt.Sprintf("\nJina Reader URL: %s", cfg.JinaAPIURL))
		sb.WriteString(fmt.Sprintf("\nJina API Key: %s", jinaKeyStatus(cfg.JinaAPIKey)))
		sb.WriteString(fmt.Sprintf("\nDebug: %v", cfg.Debug))

		// Test Grok connectivity
		sb.WriteString("\n\n--- Grok API Test ---")
		start := time.Now()
		models, err := grok.ListModels(ctx)
		duration := time.Since(start)

		if err != nil {
			sb.WriteString(fmt.Sprintf("\nConnection: FAILED (%v)", err))
		} else {
			sb.WriteString(fmt.Sprintf("\nConnection: OK (%dms)", duration.Milliseconds()))
			sb.WriteString(fmt.Sprintf("\nAvailable models: %s", strings.Join(models, ", ")))
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

func jinaKeyStatus(key string) string {
	if key == "" {
		return "(not set, using free tier)"
	}
	return maskKey(key)
}
