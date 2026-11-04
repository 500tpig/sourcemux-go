package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func TestConfigInfoJEVDoesNotExposeOrProbeKey(t *testing.T) {
	var calls atomic.Int32
	endpoint := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer endpoint.Close()
	for _, key := range []string{"", "jev-private-key"} {
		server := mcpserver.NewMCPServer("test", "1")
		RegisterConfig(server, &config.Config{JEV: engine.JEVConfig{APIURL: endpoint.URL, APIKey: key, Model: "custom-judge", TimeoutSec: 24}}, engine.NewGrokPool(nil))
		response := server.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_config_info","arguments":{}}}`))
		encoded, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		out := string(encoded)
		if strings.Contains(out, "jev-") || strings.Contains(out, "-key") || calls.Load() != 0 {
			t.Fatal("JEV diagnostics exposed or probed key")
		}
		status := "not set"
		if key != "" {
			status = "configured"
		}
		for _, want := range []string{"JEV API URL: " + endpoint.URL, "JEV Model: custom-judge", "JEV Timeout: 24s", "JEV API Key: " + status + " (not probed)"} {
			if !strings.Contains(out, want) {
				t.Fatalf("diagnostic missing %q", want)
			}
		}
	}
}
