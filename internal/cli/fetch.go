package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/500tpig/sourcemux-go/internal/router"
	"github.com/500tpig/sourcemux-go/internal/tools"
)

type fetchOutput struct {
	Source        string                 `json:"source"`
	URL           string                 `json:"url"`
	Content       string                 `json:"content"`
	Policy        tools.FetchPolicy      `json:"policy,omitempty"`
	RouteDecision []router.RouteDecision `json:"route_decision,omitempty"`
	Error         string                 `json:"error,omitempty"`
}

func runFetch(args []string) int {
	fs := flag.NewFlagSet("fetch", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	timeout := fs.Duration("timeout", 0, "Per-call timeout; defaults to 300s for quality and 60s otherwise")
	profile := fs.String("profile", tools.FetchProfileAuto, "Fetch profile: auto, quality, cheap, github, or compare")
	jsonOut := fs.Bool("json", false, "Emit JSON")
	positional, err := parsePositional(fs, args)
	if err != nil {
		return 2
	}
	timeoutProvided := flagWasProvided(fs, "timeout")
	if len(positional) == 0 {
		fmt.Fprintln(os.Stderr, "fetch: url is required")
		fs.Usage()
		return 2
	}
	url := positional[0]

	cfg, err := loadConfig()
	if err != nil {
		return reportFetchErr(*jsonOut, url, fmt.Sprintf("config: %v", err))
	}
	if msg := minimumProfileError(cfg); msg != "" {
		return reportFetchErrCode(*jsonOut, url, msg, 3)
	}

	effectiveTimeout := *timeout
	if !timeoutProvided {
		effectiveTimeout = tools.DefaultCallerTimeoutForFetchProfile(*profile)
	}
	ctx, cancel := context.WithTimeout(context.Background(), effectiveTimeout)
	defer cancel()

	clients := buildWebFetchClients(cfg)
	clients.Profile = *profile
	res, err := tools.RunWebFetch(ctx, clients, url)
	if err != nil {
		return reportFetchErr(*jsonOut, url, err.Error())
	}
	return emitFetch(*jsonOut, fetchOutput{
		Source:        res.Source,
		URL:           res.URL,
		Content:       res.Content,
		Policy:        res.Policy,
		RouteDecision: res.RouteTrace.Decisions,
	})
}

func emitFetch(asJSON bool, out fetchOutput) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	fmt.Printf("Source: %s\nURL: %s\n\n%s\n", out.Source, out.URL, out.Content)
	if out.Policy.EffectiveProfile != "" {
		fmt.Printf("\nprofile: %s intent: %s\n", out.Policy.EffectiveProfile, out.Policy.Intent)
	}
	if len(out.RouteDecision) > 0 {
		fmt.Printf("\nroute_attempts: %d\n", len(out.RouteDecision))
	}
	return 0
}

func reportFetchErr(asJSON bool, url, msg string) int {
	return reportFetchErrCode(asJSON, url, msg, 1)
}

func reportFetchErrCode(asJSON bool, url, msg string, code int) int {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(fetchOutput{URL: url, Error: msg})
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return code
}
