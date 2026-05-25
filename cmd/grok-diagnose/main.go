package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/config"
	"github.com/500tpig/sourcemux-go/internal/engine"
)

type modelCheck struct {
	Model        string
	OK           bool
	Duration     time.Duration
	SourcesCount int
	ContentBytes int
	Err          string
}

func main() {
	configPath := flag.String("config", config.DefaultConfigPath(), "single JSON config file")
	mode := flag.String("mode", "candidates", "models to test: current, candidates, all")
	timeout := flag.Duration("timeout", 25*time.Second, "per-model request timeout")
	listTimeout := flag.Duration("list-timeout", 15*time.Second, "per-endpoint /models timeout")
	maxPerEndpoint := flag.Int("max-per-endpoint", 0, "maximum models tested per endpoint; 0 means unlimited")
	query := flag.String("query", "请用一句中文回答今天是哪一天，并尽量附带一个来源URL。", "test query")
	flag.Parse()

	cfg, err := config.LoadFile(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Grok diagnose: endpoints=%d mode=%s timeout=%s\n", len(cfg.GrokEndpoints), *mode, timeout.String())
	for i, ep := range cfg.GrokEndpoints {
		fmt.Printf("\n[%d] %s\n", i+1, endpointName(ep, i))
		if !ep.IsEnabled() {
			fmt.Println("    disabled: true (skipped)")
			continue
		}
		fmt.Printf("    baseURL: %s\n", ep.BaseURL)
		fmt.Printf("    configured model: %s\n", ep.Model)
		fmt.Printf("    profile: %s\n", ep.EffectiveProfile())
		fmt.Printf("    send search:true: %v\n", ep.SendSearchFlag)

		models, listErr := listModels(ep, *listTimeout)
		if listErr != nil {
			fmt.Printf("    /models: FAILED: %v\n", listErr)
		} else {
			fmt.Printf("    /models: OK (%d models)\n", len(models))
		}

		targets := selectModels(ep.Model, models, *mode)
		if *maxPerEndpoint > 0 && len(targets) > *maxPerEndpoint {
			targets = targets[:*maxPerEndpoint]
		}
		if len(targets) == 0 {
			fmt.Println("    no models selected for chat test")
			continue
		}

		results := make([]modelCheck, 0, len(targets))
		for _, model := range targets {
			results = append(results, checkModel(ep, model, *query, *timeout))
		}
		printResults(results)
	}
}

func endpointName(ep engine.GrokEndpoint, idx int) string {
	if ep.Name != "" {
		return ep.Name
	}
	return fmt.Sprintf("endpoint-%d", idx)
}

func newClient(ep engine.GrokEndpoint, model string, timeout time.Duration) *engine.GrokClient {
	c := engine.NewGrokClient(ep.BaseURL, ep.APIKey, model)
	c.Name = ep.Name
	c.SendSearchFlag = ep.SendSearchFlag
	c.HTTPClient = &http.Client{Timeout: timeout}
	c.RetryConfig.MaxAttempts = 1
	return c
}

func listModels(ep engine.GrokEndpoint, timeout time.Duration) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c := newClient(ep, ep.Model, timeout)
	return c.ListModels(ctx)
}

func selectModels(configured string, models []string, mode string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(models)+1)
	add := func(model string) {
		model = strings.TrimSpace(model)
		if model == "" {
			return
		}
		if _, ok := seen[model]; ok {
			return
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}

	add(configured)
	switch mode {
	case "current":
		return out
	case "all":
		for _, model := range models {
			add(model)
		}
	default:
		for _, model := range models {
			lower := strings.ToLower(model)
			if strings.Contains(lower, "grok") || strings.Contains(lower, "search") {
				add(model)
			}
		}
	}
	return out
}

func checkModel(ep engine.GrokEndpoint, model, query string, timeout time.Duration) modelCheck {
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c := newClient(ep, model, timeout)
	res, err := c.Search(ctx, query)
	check := modelCheck{
		Model:    model,
		Duration: time.Since(start),
	}
	if err != nil {
		check.Err = compactErr(err)
		return check
	}
	if res == nil || strings.TrimSpace(res.Content) == "" {
		check.Err = "empty content"
		return check
	}
	check.OK = true
	check.SourcesCount = res.SourcesCount
	check.ContentBytes = len(res.Content)
	return check
}

func compactErr(err error) string {
	msg := strings.ReplaceAll(err.Error(), "\n", " ")
	msg = strings.Join(strings.Fields(msg), " ")
	const maxLen = 220
	if len(msg) > maxLen {
		return msg[:maxLen] + "..."
	}
	return msg
}

func printResults(results []modelCheck) {
	okCount := 0
	for _, r := range results {
		if r.OK {
			okCount++
		}
	}
	fmt.Printf("    chat test: %d/%d OK\n", okCount, len(results))

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].OK != results[j].OK {
			return results[i].OK
		}
		return results[i].Duration < results[j].Duration
	})

	for _, r := range results {
		status := "FAIL"
		detail := r.Err
		if r.OK {
			status = "OK"
			detail = fmt.Sprintf("sources=%d bytes=%d", r.SourcesCount, r.ContentBytes)
		}
		fmt.Printf("      %-4s %-35s %6dms  %s\n", status, r.Model, r.Duration.Milliseconds(), detail)
	}
}
