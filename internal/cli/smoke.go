package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/500tpig/grok-search-go/internal/capability"
	"github.com/500tpig/grok-search-go/internal/router"
)

type smokeOutput struct {
	OK         bool              `json:"ok"`
	Mode       string            `json:"mode"`
	RouteTrace router.RouteTrace `json:"route_trace"`
	Content    string            `json:"content,omitempty"`
	Error      string            `json:"error,omitempty"`
}

func runSmoke(args []string) int {
	fs := flag.NewFlagSet("smoke", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	mock := fs.Bool("mock", false, "Run router smoke test with fake providers and zero external API calls")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if !*mock {
		msg := "smoke currently requires --mock to guarantee zero external API calls"
		if *jsonOut {
			_ = json.NewEncoder(os.Stdout).Encode(smokeOutput{OK: false, Error: msg})
		} else {
			fmt.Fprintln(os.Stderr, msg)
		}
		return 2
	}

	r := router.New(
		smokeProvider{name: "mock-empty", kind: capability.MainSearch},
		smokeProvider{name: "mock-ok", kind: capability.MainSearch, content: "mock route ok"},
	)
	res, trace := r.Run(context.Background(), capability.MainSearch, capability.Request{Query: "smoke"})
	out := smokeOutput{OK: res.Content != "", Mode: "mock", RouteTrace: trace, Content: res.Content}
	if *jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
	} else {
		fmt.Printf("smoke: ok=%v mode=%s final_provider=%s attempts=%d\n", out.OK, out.Mode, trace.FinalProvider, trace.AttemptsCount)
	}
	if out.OK {
		return 0
	}
	return 1
}

type smokeProvider struct {
	name    string
	kind    capability.Kind
	content string
}

func (p smokeProvider) Name() string          { return p.name }
func (p smokeProvider) Kind() capability.Kind { return p.kind }
func (p smokeProvider) Try(context.Context, capability.Request) (capability.Result, error) {
	return capability.Result{Content: p.content}, nil
}
