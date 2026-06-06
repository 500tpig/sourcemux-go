package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/500tpig/sourcemux-go/internal/capability"
	"github.com/500tpig/sourcemux-go/internal/engine"
	"github.com/500tpig/sourcemux-go/internal/router"
	"github.com/500tpig/sourcemux-go/internal/router/adapters"
	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

const mcpFetchExcerptRunes = 1800

// WebFetchClients groups the production fetch providers in fallback order.
type WebFetchClients struct {
	Jina        *engine.JinaClient
	TinyFish    *engine.TinyFishPool
	Firecrawl   *engine.FirecrawlPool
	Exa         *engine.ExaClient
	Tavily      *engine.TavilyClient
	Order       []string
	StrictOrder bool
	Profile     string
}

// WebFetchResult is the shared fetch envelope used by MCP, CLI, and research.
type WebFetchResult struct {
	Source     string            `json:"source"`
	URL        string            `json:"url"`
	Content    string            `json:"content"`
	Policy     FetchPolicy       `json:"policy,omitempty"`
	RouteTrace router.RouteTrace `json:"route_trace,omitempty"`
}

// RegisterFetch registers the web_fetch tool.
func RegisterFetch(s *mcpserver.MCPServer, clients WebFetchClients) {
	tool := mcp.NewTool("web_fetch",
		mcp.WithDescription("Fetch and extract web page content as Markdown through SourceMux policy-first provider routing."),
		mcp.WithString("url", mcp.Required(), mcp.Description("Target URL to fetch")),
		mcp.WithBoolean("include_trace", mcp.Description("Return full route trace in _meta.route_trace")),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		url, _ := req.Params.Arguments["url"].(string)
		includeTrace := boolArgOr(req.Params.Arguments, "include_trace", false)
		result, err := RunWebFetch(ctx, clients, url)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		out := mcp.NewToolResultText(FormatWebFetchResult(result))
		if includeTrace {
			out.Meta = map[string]any{"route_trace": result.RouteTrace}
		} else {
			out.Meta = map[string]any{"route_trace": result.RouteTrace.Compact()}
		}
		return out, nil
	})
}

// RunWebFetch executes the production web_fetch fallback chain.
func RunWebFetch(ctx context.Context, clients WebFetchClients, url string) (*WebFetchResult, error) {
	if url == "" {
		return nil, fmt.Errorf("url is required")
	}
	policy, err := ResolveFetchPolicy(clients.Profile, url, clients.Order, clients.StrictOrder)
	if err != nil {
		return nil, err
	}
	clients.Order = policy.ProviderOrder
	clients.StrictOrder = true

	r := router.New(fetchProviders(clients)...)
	res, trace := r.Run(ctx, capability.WebFetch, capability.Request{URL: url})
	if strings.TrimSpace(res.Content) == "" {
		if detail := firstFailureDetail(trace); detail != "" {
			return nil, fmt.Errorf("web_fetch failed: %s", detail)
		}
		return nil, fmt.Errorf("all policy-selected web_fetch providers failed or are not configured")
	}
	resultURL := metadataString(res.Metadata, "url", url)
	if resultURL == "" && len(res.Sources) > 0 {
		resultURL = res.Sources[0].URL
	}
	return &WebFetchResult{
		Source:     metadataString(res.Metadata, "engine", trace.FinalProvider),
		URL:        resultURL,
		Content:    res.Content,
		Policy:     policy,
		RouteTrace: trace,
	}, nil
}

func FormatWebFetchResult(result *WebFetchResult) string {
	if result == nil {
		return ""
	}
	content := strings.TrimSpace(result.Content)
	var sb strings.Builder
	fmt.Fprintf(&sb, "Source: %s\nURL: %s\n", result.Source, result.URL)
	if result.RouteTrace.AttemptsCount > 0 {
		fmt.Fprintf(&sb, "route: final_provider=%s fallback_triggered=%v attempts_count=%d profile=%s intent=%s\n",
			result.RouteTrace.FinalProvider, result.RouteTrace.FallbackTriggered, result.RouteTrace.AttemptsCount, result.Policy.EffectiveProfile, result.Policy.Intent)
	}
	if content == "" {
		return strings.TrimSpace(sb.String())
	}
	fmt.Fprintf(&sb, "content_chars: %d\n\nexcerpt:\n%s",
		len([]rune(content)),
		indentContinuation(clipRunes(content, mcpFetchExcerptRunes), "  "),
	)
	return sb.String()
}

func fetchProviders(clients WebFetchClients) []capability.Provider {
	var providers []capability.Provider
	order := clients.Order
	if len(order) == 0 && !clients.StrictOrder {
		order = []string{"jina", "tinyfish", "exa", "tavily"}
	}
	for _, name := range order {
		switch strings.TrimSpace(name) {
		case "github":
			providers = append(providers, NewGitHubFetchProvider(nil))
		case "jina":
			if clients.Jina != nil {
				providers = append(providers, adapters.NewJinaFetch(clients.Jina))
			}
		case "tinyfish":
			if clients.TinyFish != nil && clients.TinyFish.Len() > 0 {
				providers = append(providers, adapters.NewTinyFishFetch(clients.TinyFish))
			}
		case "firecrawl":
			if clients.Firecrawl != nil && clients.Firecrawl.Len() > 0 {
				providers = append(providers, adapters.NewFirecrawlFetch(clients.Firecrawl))
			}
		case "exa":
			if clients.Exa != nil {
				providers = append(providers, adapters.NewExaContents(clients.Exa))
			}
		case "tavily":
			if clients.Tavily != nil {
				providers = append(providers, adapters.NewTavilyExtract(clients.Tavily))
			}
		}
	}
	return providers
}

type GitHubFetchProvider struct {
	HTTPClient *http.Client
	APIURL     string
	RawBaseURL string
}

func NewGitHubFetchProvider(client *http.Client) *GitHubFetchProvider {
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	return &GitHubFetchProvider{HTTPClient: client, APIURL: "https://api.github.com", RawBaseURL: "https://raw.githubusercontent.com"}
}

func (p *GitHubFetchProvider) Name() string { return "github-provider" }
func (p *GitHubFetchProvider) Kind() capability.Kind {
	return capability.WebFetch
}

func (p *GitHubFetchProvider) Try(ctx context.Context, req capability.Request) (capability.Result, error) {
	ref, ok := parseGitHubURL(req.URL)
	if !ok {
		return capability.Result{}, fmt.Errorf("not a supported GitHub repository URL")
	}
	content, err := p.fetchGitHubContent(ctx, ref)
	if err != nil {
		return capability.Result{}, err
	}
	return githubFetchResult("GitHub Provider", req.URL, content), nil
}

func (p *GitHubFetchProvider) fetchGitHubContent(ctx context.Context, ref githubRef) (string, error) {
	repo, err := p.getRepo(ctx, ref.Owner, ref.Repo)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s/%s\n\n", ref.Owner, ref.Repo)
	if strings.TrimSpace(repo.Description) != "" {
		fmt.Fprintf(&sb, "%s\n\n", strings.TrimSpace(repo.Description))
	}
	fmt.Fprintf(&sb, "- stars: %d\n- forks: %d\n- language: %s\n- license: %s\n- default_branch: %s\n- open_issues: %d\n",
		repo.StargazersCount,
		repo.ForksCount,
		stringOrFallback(repo.Language, "unknown"),
		repo.LicenseName(),
		stringOrFallback(repo.DefaultBranch, "unknown"),
		repo.OpenIssuesCount,
	)
	if languages, err := p.getLanguages(ctx, ref.Owner, ref.Repo); err == nil && len(languages) > 0 {
		fmt.Fprintf(&sb, "- languages: %s\n", formatGitHubLanguages(languages))
	}
	if ref.Kind != "" {
		fmt.Fprintf(&sb, "- requested_kind: %s\n", ref.Kind)
	}
	if ref.Target != "" {
		fmt.Fprintf(&sb, "- requested_target: %s\n", ref.Target)
	}
	if release, err := p.getLatestRelease(ctx, ref.Owner, ref.Repo); err == nil && release.TagName != "" {
		fmt.Fprintf(&sb, "- latest_release: %s", release.TagName)
		if release.Name != "" && release.Name != release.TagName {
			fmt.Fprintf(&sb, " (%s)", release.Name)
		}
		sb.WriteString("\n")
	}
	if readme, err := p.getReadme(ctx, ref.Owner, ref.Repo); err == nil && strings.TrimSpace(readme) != "" {
		fmt.Fprintf(&sb, "\n## README\n\n%s\n", clipRunes(readme, 8000))
	}
	if ref.Kind == "blob" && ref.Target != "" {
		if fileContent, err := p.getRawBlob(ctx, ref); err == nil && strings.TrimSpace(fileContent) != "" {
			fmt.Fprintf(&sb, "\n## Target File\n\npath: %s\n\n%s\n", ref.Target, clipRunes(fileContent, 8000))
		}
	}
	return strings.TrimSpace(sb.String()), nil
}

func (p *GitHubFetchProvider) getRepo(ctx context.Context, owner, repo string) (githubRepoResponse, error) {
	var out githubRepoResponse
	if err := p.getJSON(ctx, fmt.Sprintf("/repos/%s/%s", url.PathEscape(owner), url.PathEscape(repo)), &out); err != nil {
		return githubRepoResponse{}, err
	}
	return out, nil
}

func (p *GitHubFetchProvider) getReadme(ctx context.Context, owner, repo string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.APIURL, "/")+fmt.Sprintf("/repos/%s/%s/readme", url.PathEscape(owner), url.PathEscape(repo)), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.raw")
	req.Header.Set("User-Agent", "sourcemux")
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github readme HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1_000_000))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (p *GitHubFetchProvider) getLanguages(ctx context.Context, owner, repo string) (map[string]int, error) {
	out := map[string]int{}
	if err := p.getJSON(ctx, fmt.Sprintf("/repos/%s/%s/languages", url.PathEscape(owner), url.PathEscape(repo)), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *GitHubFetchProvider) getLatestRelease(ctx context.Context, owner, repo string) (githubReleaseResponse, error) {
	var out githubReleaseResponse
	if err := p.getJSON(ctx, fmt.Sprintf("/repos/%s/%s/releases/latest", url.PathEscape(owner), url.PathEscape(repo)), &out); err != nil {
		return githubReleaseResponse{}, err
	}
	return out, nil
}

func (p *GitHubFetchProvider) getRawBlob(ctx context.Context, ref githubRef) (string, error) {
	targetParts := strings.Split(ref.Target, "/")
	if len(targetParts) < 2 {
		return "", fmt.Errorf("github blob URL missing branch or path")
	}
	branch := targetParts[0]
	path := strings.Join(targetParts[1:], "/")
	rawBase := strings.TrimRight(p.RawBaseURL, "/")
	if rawBase == "" {
		rawBase = "https://raw.githubusercontent.com"
	}
	rawURL := rawBase + "/" + url.PathEscape(ref.Owner) + "/" + url.PathEscape(ref.Repo) + "/" + url.PathEscape(branch) + "/" + escapeGitHubPath(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "sourcemux")
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github raw HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1_000_000))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (p *GitHubFetchProvider) getJSON(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.APIURL, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sourcemux")
	resp, err := p.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1000))
		return fmt.Errorf("github API HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (p *GitHubFetchProvider) httpClient() *http.Client {
	if p != nil && p.HTTPClient != nil {
		return p.HTTPClient
	}
	return http.DefaultClient
}

type githubRef struct {
	Owner  string
	Repo   string
	Kind   string
	Target string
}

func parseGitHubURL(raw string) (githubRef, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return githubRef{}, false
	}
	host := strings.ToLower(strings.TrimPrefix(u.Hostname(), "www."))
	if host != "github.com" {
		return githubRef{}, false
	}
	parts := strings.Split(strings.Trim(u.EscapedPath(), "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return githubRef{}, false
	}
	ref := githubRef{Owner: parts[0], Repo: parts[1]}
	if len(parts) > 2 {
		ref.Kind = parts[2]
		ref.Target = strings.Join(parts[3:], "/")
	}
	return ref, true
}

type githubRepoResponse struct {
	Description     string `json:"description"`
	StargazersCount int    `json:"stargazers_count"`
	ForksCount      int    `json:"forks_count"`
	Language        string `json:"language"`
	DefaultBranch   string `json:"default_branch"`
	OpenIssuesCount int    `json:"open_issues_count"`
	License         *struct {
		Name string `json:"name"`
		SPDX string `json:"spdx_id"`
	} `json:"license"`
}

type githubReleaseResponse struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
}

func (r githubRepoResponse) LicenseName() string {
	if r.License == nil {
		return "unknown"
	}
	if strings.TrimSpace(r.License.SPDX) != "" {
		return strings.TrimSpace(r.License.SPDX)
	}
	return stringOrFallback(r.License.Name, "unknown")
}

func stringOrFallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func formatGitHubLanguages(languages map[string]int) string {
	parts := make([]string, 0, len(languages))
	for language, bytes := range languages {
		if strings.TrimSpace(language) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s:%d", language, bytes))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func escapeGitHubPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

func githubFetchResult(source, rawURL, content string) capability.Result {
	return capability.Result{
		Content: content,
		Sources: []capability.Source{
			{URL: rawURL},
		},
		Metadata: map[string]any{
			"engine": source,
			"url":    rawURL,
		},
	}
}
