package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// SourceGetter is implemented by server.App to retrieve cached sources.
type SourceGetter interface {
	GetSources(sessionID string) ([]string, bool)
}

// RegisterSources registers the get_sources tool.
func RegisterSources(s *mcpserver.MCPServer, getter SourceGetter) {
	tool := mcp.NewTool("get_sources",
		mcp.WithDescription("Retrieve source URLs from a previous web_search by session_id."),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("Session ID from web_search")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		sessionID, _ := req.Params.Arguments["session_id"].(string)
		if sessionID == "" {
			return mcp.NewToolResultError("session_id is required"), nil
		}

		urls, ok := getter.GetSources(sessionID)
		if !ok {
			return mcp.NewToolResultError(fmt.Sprintf("no sources found for session %s", sessionID)), nil
		}

		output := fmt.Sprintf("session_id: %s\nsources_count: %d\n\n%s",
			sessionID, len(urls), strings.Join(urls, "\n"))
		return mcp.NewToolResultText(output), nil
	})
}
