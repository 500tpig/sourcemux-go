package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/500tpig/grok-search-go/internal/engine"
)

const defaultTinyFishCasesPath = "docs/tinyfish-benchmark-cases.sample.json"

type tinyfishKeyConfig struct {
	Name   string `json:"name"`
	APIKey string `json:"apiKey"`
}

type tinyfishKeysFile struct {
	Keys []tinyfishKeyConfig `json:"keys"`
}

type tinyfishBenchmarkCases struct {
	Search []tinyfishSearchCase `json:"search"`
	Fetch  []tinyfishFetchCase  `json:"fetch"`
	Agent  []tinyfishAgentCase  `json:"agent"`
}

type tinyfishSearchCase struct {
	Name     string `json:"name"`
	Query    string `json:"query"`
	Location string `json:"location,omitempty"`
	Language string `json:"language,omitempty"`
	Page     *int   `json:"page,omitempty"`
}

type tinyfishFetchCase struct {
	Name       string   `json:"name"`
	URLs       []string `json:"urls"`
	Format     string   `json:"format,omitempty"`
	Links      bool     `json:"links,omitempty"`
	ImageLinks bool     `json:"image_links,omitempty"`
}

type tinyfishAgentCase struct {
	Name           string         `json:"name"`
	URL            string         `json:"url"`
	Goal           string         `json:"goal"`
	BrowserProfile string         `json:"browser_profile,omitempty"`
	MaxSteps       int            `json:"max_steps,omitempty"`
	OutputSchema   map[string]any `json:"output_schema,omitempty"`
}

type tinyfishBenchmarkOutput struct {
	GeneratedAt string                    `json:"generated_at"`
	CasesFile   string                    `json:"cases_file"`
	KeyCount    int                       `json:"key_count"`
	DurationMS  int64                     `json:"duration_ms"`
	Results     []tinyfishKeyBenchmarkRun `json:"results"`
}

type tinyfishKeyBenchmarkRun struct {
	Name      string                      `json:"name"`
	KeyStatus string                      `json:"key_status"`
	Search    []tinyfishSearchMeasurement `json:"search,omitempty"`
	Fetch     []tinyfishFetchMeasurement  `json:"fetch,omitempty"`
	Agent     []tinyfishAgentMeasurement  `json:"agent,omitempty"`
}

type tinyfishSearchMeasurement struct {
	Case         string   `json:"case"`
	Query        string   `json:"query"`
	OK           bool     `json:"ok"`
	HTTPStatus   int      `json:"http_status,omitempty"`
	DurationMS   int64    `json:"duration_ms"`
	Error        string   `json:"error,omitempty"`
	ResultCount  int      `json:"result_count"`
	TotalResults int      `json:"total_results"`
	SourceURLs   []string `json:"source_urls,omitempty"`
}

type tinyfishFetchMeasurement struct {
	Case       string                        `json:"case"`
	URLs       []string                      `json:"urls"`
	OK         bool                          `json:"ok"`
	HTTPStatus int                           `json:"http_status,omitempty"`
	DurationMS int64                         `json:"duration_ms"`
	Error      string                        `json:"error,omitempty"`
	Results    []tinyfishFetchResultSummary  `json:"results,omitempty"`
	Errors     []engine.TinyFishFetchFailure `json:"per_url_errors,omitempty"`
}

type tinyfishFetchResultSummary struct {
	URL        string `json:"url"`
	FinalURL   string `json:"final_url,omitempty"`
	Title      string `json:"title,omitempty"`
	TextLength int    `json:"text_length"`
	LatencyMS  *int64 `json:"latency_ms,omitempty"`
}

type tinyfishAgentMeasurement struct {
	Case       string          `json:"case"`
	URL        string          `json:"url"`
	Goal       string          `json:"goal"`
	OK         bool            `json:"ok"`
	HTTPStatus int             `json:"http_status,omitempty"`
	DurationMS int64           `json:"duration_ms"`
	Error      string          `json:"error,omitempty"`
	RunID      string          `json:"run_id,omitempty"`
	Status     string          `json:"status,omitempty"`
	NumOfSteps *int            `json:"num_of_steps,omitempty"`
	Steps      json.RawMessage `json:"steps,omitempty"`
	Result     json.RawMessage `json:"result,omitempty"`
}

func runTinyFishBench(args []string) int {
	fs := flag.NewFlagSet("tinyfish-bench", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	casesPath := fs.String("cases", defaultTinyFishCasesPath, "Benchmark cases JSON file")
	keysFile := fs.String("keys-file", "", "Local JSON file containing TinyFish API keys")
	timeout := fs.Duration("timeout", 150*time.Second, "Per-request timeout")
	jsonOut := fs.Bool("json", false, "Emit machine-readable JSON")
	searchURL := fs.String("search-url", engine.DefaultTinyFishSearchURL, "TinyFish Search endpoint")
	fetchURL := fs.String("fetch-url", engine.DefaultTinyFishFetchURL, "TinyFish Fetch endpoint")
	agentURL := fs.String("agent-url", engine.DefaultTinyFishAgentURL, "TinyFish sync Agent endpoint")
	surfaces := fs.String("surfaces", "search,fetch,agent", "Comma-separated surfaces to run: search,fetch,agent")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	keys, err := loadTinyFishKeys(*keysFile)
	if err != nil {
		return reportTinyFishBenchErr(*jsonOut, fmt.Sprintf("keys: %v", err))
	}
	cases, err := loadTinyFishBenchmarkCases(*casesPath)
	if err != nil {
		return reportTinyFishBenchErr(*jsonOut, fmt.Sprintf("cases: %v", err))
	}
	selected, err := parseTinyFishSurfaces(*surfaces)
	if err != nil {
		return reportTinyFishBenchErr(*jsonOut, err.Error())
	}

	start := time.Now()
	out := tinyfishBenchmarkOutput{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		CasesFile:   *casesPath,
		KeyCount:    len(keys),
	}

	for _, key := range keys {
		client := engine.NewTinyFishClient(key.APIKey)
		client.SearchURL = *searchURL
		client.FetchURL = *fetchURL
		client.AgentURL = *agentURL

		run := tinyfishKeyBenchmarkRun{Name: key.Name, KeyStatus: keyStatus(key.APIKey)}
		if selected["search"] {
			for _, c := range cases.Search {
				run.Search = append(run.Search, benchmarkTinyFishSearch(client, c, *timeout))
			}
		}
		if selected["fetch"] {
			for _, c := range cases.Fetch {
				run.Fetch = append(run.Fetch, benchmarkTinyFishFetch(client, c, *timeout))
			}
		}
		if selected["agent"] {
			for _, c := range cases.Agent {
				run.Agent = append(run.Agent, benchmarkTinyFishAgent(client, c, *timeout))
			}
		}
		out.Results = append(out.Results, run)
	}
	out.DurationMS = time.Since(start).Milliseconds()
	return emitTinyFishBench(*jsonOut, out)
}

func benchmarkTinyFishSearch(client *engine.TinyFishClient, c tinyfishSearchCase, timeout time.Duration) tinyfishSearchMeasurement {
	m := tinyfishSearchMeasurement{Case: c.Name, Query: c.Query}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	res, err := client.Search(ctx, engine.TinyFishSearchRequest{
		Query:    c.Query,
		Location: c.Location,
		Language: c.Language,
		Page:     c.Page,
	})
	m.DurationMS = time.Since(start).Milliseconds()
	if res != nil {
		m.HTTPStatus = res.HTTPStatus
		m.ResultCount = len(res.Results)
		m.TotalResults = res.TotalResults
		for _, r := range res.Results {
			if r.URL != "" {
				m.SourceURLs = append(m.SourceURLs, r.URL)
			}
		}
	}
	if err != nil {
		m.Error = err.Error()
		return m
	}
	m.OK = true
	return m
}

func benchmarkTinyFishFetch(client *engine.TinyFishClient, c tinyfishFetchCase, timeout time.Duration) tinyfishFetchMeasurement {
	m := tinyfishFetchMeasurement{Case: c.Name, URLs: c.URLs}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	res, err := client.Fetch(ctx, engine.TinyFishFetchRequest{
		URLs:       c.URLs,
		Format:     c.Format,
		Links:      c.Links,
		ImageLinks: c.ImageLinks,
	})
	m.DurationMS = time.Since(start).Milliseconds()
	if res != nil {
		m.HTTPStatus = res.HTTPStatus
		m.Errors = res.Errors
		for _, r := range res.Results {
			m.Results = append(m.Results, tinyfishFetchResultSummary{
				URL:        r.URL,
				FinalURL:   r.FinalURL,
				Title:      r.Title,
				TextLength: engine.TinyFishTextLength(r.Text),
				LatencyMS:  r.LatencyMS,
			})
		}
	}
	if err != nil {
		m.Error = err.Error()
		return m
	}
	m.OK = len(m.Errors) == 0
	return m
}

func benchmarkTinyFishAgent(client *engine.TinyFishClient, c tinyfishAgentCase, timeout time.Duration) tinyfishAgentMeasurement {
	m := tinyfishAgentMeasurement{Case: c.Name, URL: c.URL, Goal: c.Goal}
	req := engine.TinyFishAgentRequest{
		URL:            c.URL,
		Goal:           c.Goal,
		BrowserProfile: c.BrowserProfile,
		OutputSchema:   c.OutputSchema,
	}
	if c.MaxSteps > 0 {
		req.AgentConfig = map[string]any{"max_steps": c.MaxSteps}
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	start := time.Now()
	res, err := client.RunAgent(ctx, req)
	m.DurationMS = time.Since(start).Milliseconds()
	if res != nil {
		m.HTTPStatus = res.HTTPStatus
		m.RunID = res.RunID
		m.Status = res.Status
		m.NumOfSteps = res.NumOfSteps
		m.Steps = res.Steps
		m.Result = res.Result
		if rawErr := rawJSONSummary(res.Error); rawErr != "" {
			m.Error = rawErr
		}
	}
	if err != nil {
		m.Error = err.Error()
		return m
	}
	m.OK = strings.EqualFold(m.Status, "COMPLETED") && m.Error == ""
	return m
}

func loadTinyFishKeys(path string) ([]tinyfishKeyConfig, error) {
	if path == "" {
		path = os.Getenv("TINYFISH_KEYS_FILE")
	}
	if path != "" {
		return loadTinyFishKeysFile(path)
	}
	raw := os.Getenv("TINYFISH_API_KEYS")
	if raw == "" {
		raw = os.Getenv("TINYFISH_API_KEY")
	}
	if raw == "" {
		return nil, errors.New("set TINYFISH_API_KEYS, TINYFISH_API_KEY, TINYFISH_KEYS_FILE, or --keys-file")
	}
	keyParts := splitTinyFishList(raw)
	nameParts := splitTinyFishList(os.Getenv("TINYFISH_KEY_NAMES"))
	keys := make([]tinyfishKeyConfig, 0, len(keyParts))
	for i, key := range keyParts {
		name := fmt.Sprintf("key-%d", i+1)
		if i < len(nameParts) && nameParts[i] != "" {
			name = nameParts[i]
		}
		keys = append(keys, tinyfishKeyConfig{Name: name, APIKey: key})
	}
	return normalizeTinyFishKeys(keys)
}

func loadTinyFishKeysFile(path string) ([]tinyfishKeyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wrapped tinyfishKeysFile
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Keys) > 0 {
		return normalizeTinyFishKeys(wrapped.Keys)
	}
	var direct []tinyfishKeyConfig
	if err := json.Unmarshal(data, &direct); err != nil {
		return nil, err
	}
	return normalizeTinyFishKeys(direct)
}

func normalizeTinyFishKeys(keys []tinyfishKeyConfig) ([]tinyfishKeyConfig, error) {
	out := make([]tinyfishKeyConfig, 0, len(keys))
	for i, key := range keys {
		key.Name = strings.TrimSpace(key.Name)
		key.APIKey = strings.TrimSpace(key.APIKey)
		if key.APIKey == "" {
			return nil, fmt.Errorf("key #%d is empty", i+1)
		}
		if key.Name == "" {
			key.Name = fmt.Sprintf("key-%d", i+1)
		}
		out = append(out, key)
	}
	if len(out) == 0 {
		return nil, errors.New("no TinyFish keys configured")
	}
	return out, nil
}

func loadTinyFishBenchmarkCases(path string) (*tinyfishBenchmarkCases, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases tinyfishBenchmarkCases
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, err
	}
	return &cases, validateTinyFishBenchmarkCases(cases)
}

func validateTinyFishBenchmarkCases(cases tinyfishBenchmarkCases) error {
	if len(cases.Search)+len(cases.Fetch)+len(cases.Agent) == 0 {
		return errors.New("no benchmark cases configured")
	}
	for i, c := range cases.Search {
		if strings.TrimSpace(c.Query) == "" {
			return fmt.Errorf("search case #%d has empty query", i+1)
		}
	}
	for i, c := range cases.Fetch {
		if len(c.URLs) == 0 {
			return fmt.Errorf("fetch case #%d has no URLs", i+1)
		}
	}
	for i, c := range cases.Agent {
		if strings.TrimSpace(c.URL) == "" || strings.TrimSpace(c.Goal) == "" {
			return fmt.Errorf("agent case #%d needs url and goal", i+1)
		}
	}
	return nil
}

func parseTinyFishSurfaces(raw string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		s := strings.ToLower(strings.TrimSpace(part))
		if s == "" {
			continue
		}
		switch s {
		case "search", "fetch", "agent":
			out[s] = true
		default:
			return nil, fmt.Errorf("unknown surface %q", s)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("at least one TinyFish surface is required")
	}
	return out, nil
}

func splitTinyFishList(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if s := strings.TrimSpace(part); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func emitTinyFishBench(asJSON bool, out tinyfishBenchmarkOutput) int {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(out)
		return 0
	}
	fmt.Printf("TinyFish benchmark: %d key(s), %dms\n", out.KeyCount, out.DurationMS)
	for _, run := range out.Results {
		fmt.Printf("\n[%s] key=%s\n", run.Name, run.KeyStatus)
		for _, m := range run.Search {
			fmt.Printf("  search %-24s ok=%v status=%d results=%d duration=%dms", m.Case, m.OK, m.HTTPStatus, m.ResultCount, m.DurationMS)
			if m.Error != "" {
				fmt.Printf(" error=%s", m.Error)
			}
			fmt.Println()
		}
		for _, m := range run.Fetch {
			fmt.Printf("  fetch  %-24s ok=%v status=%d results=%d url_errors=%d duration=%dms", m.Case, m.OK, m.HTTPStatus, len(m.Results), len(m.Errors), m.DurationMS)
			if m.Error != "" {
				fmt.Printf(" error=%s", m.Error)
			}
			fmt.Println()
		}
		for _, m := range run.Agent {
			fmt.Printf("  agent  %-24s ok=%v status=%s steps=", m.Case, m.OK, m.Status)
			if m.NumOfSteps == nil {
				fmt.Print("null")
			} else {
				fmt.Print(*m.NumOfSteps)
			}
			fmt.Printf(" duration=%dms", m.DurationMS)
			if m.Error != "" {
				fmt.Printf(" error=%s", m.Error)
			}
			fmt.Println()
		}
	}
	return 0
}

func reportTinyFishBenchErr(asJSON bool, msg string) int {
	if asJSON {
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": msg})
	} else {
		fmt.Fprintln(os.Stderr, msg)
	}
	return 1
}

func rawJSONSummary(raw json.RawMessage) string {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	if len(raw) > 500 {
		return string(raw[:500]) + "..."
	}
	return string(raw)
}
