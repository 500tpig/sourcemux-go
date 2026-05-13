package adapters

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path"
	"strings"

	"github.com/500tpig/grok-search-go/internal/capability"
	"github.com/500tpig/grok-search-go/internal/engine"
)

const (
	optionLibraryName = "library_name"
	optionLibraryID   = "library_id"
	optionProvider    = "provider"
	optionFast        = "fast"
)

type Context7DocsProvider struct {
	Clients []*engine.Context7Client
}

func NewContext7Docs(clients []*engine.Context7Client) *Context7DocsProvider {
	return &Context7DocsProvider{Clients: clients}
}

func (p *Context7DocsProvider) Name() string { return "context7" }
func (p *Context7DocsProvider) Kind() capability.Kind {
	return capability.DocsSearch
}

func (p *Context7DocsProvider) CanHandle(req capability.Request) (bool, string) {
	if strings.TrimSpace(stringOption(req, optionLibraryID)) != "" {
		return true, ""
	}
	if strings.TrimSpace(stringOption(req, optionLibraryName)) != "" {
		return true, ""
	}
	return false, "context7 requires explicit library_id or library_name"
}

func (p *Context7DocsProvider) Try(ctx context.Context, req capability.Request) (capability.Result, error) {
	client, err := p.selectClient(req)
	if err != nil {
		return capability.Result{}, err
	}
	libraryID := strings.TrimSpace(stringOption(req, optionLibraryID))
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return capability.Result{}, fmt.Errorf("context7 docs query is required")
	}
	if libraryID == "" {
		search, err := client.SearchLibraries(ctx, engine.Context7LibrarySearchRequest{
			LibraryName: stringOption(req, optionLibraryName),
			Query:       query,
			Fast:        boolOption(req, optionFast),
		})
		if err != nil {
			return capability.Result{}, err
		}
		libraryID = firstContext7LibraryID(search)
		if libraryID == "" {
			return capability.Result{}, fmt.Errorf("context7 library search returned no library id")
		}
	}
	res, err := client.GetDocs(ctx, engine.Context7DocsRequest{
		LibraryID: libraryID,
		Query:     query,
		Type:      "json",
		Fast:      boolOption(req, optionFast),
	})
	if err != nil {
		return capability.Result{}, err
	}
	return capability.Result{
		Content: engine.FormatContext7DocsContent(res, 1200),
		Sources: sourcesFromURLs(engine.Context7DocsSourceURLs(res)),
		Metadata: map[string]any{
			metaEngine:       "Context7",
			metaEndpointName: client.Name(),
			metaFallback:     "context7",
		},
	}, nil
}

func (p *Context7DocsProvider) Classify(res capability.Result, err error) (capability.Outcome, capability.FallbackReason, string) {
	if err == nil {
		if strings.TrimSpace(res.Content) == "" {
			return capability.Empty, capability.ReasonNoContent, "empty content"
		}
		return capability.OK, capability.ReasonNone, ""
	}
	var statusErr engine.HTTPStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.StatusCode {
		case http.StatusBadRequest, http.StatusUnauthorized:
			return capability.Permanent, capability.ReasonUpstreamError, statusErr.Error()
		case http.StatusTooManyRequests:
			return capability.Transient, capability.ReasonRateLimited, statusErr.Error()
		case http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity:
			return capability.Transient, capability.ReasonUpstreamError, statusErr.Error()
		default:
			if statusErr.StatusCode >= 500 {
				return capability.Transient, capability.ReasonUpstreamError, statusErr.Error()
			}
		}
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "rate") || strings.Contains(msg, "429") {
		return capability.Transient, capability.ReasonRateLimited, err.Error()
	}
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline") {
		return capability.Transient, capability.ReasonTimeout, err.Error()
	}
	return capability.Transient, capability.ReasonUpstreamError, err.Error()
}

func (p *Context7DocsProvider) selectClient(req capability.Request) (*engine.Context7Client, error) {
	if p == nil || len(p.Clients) == 0 {
		return nil, fmt.Errorf("context7 is not configured")
	}
	provider := strings.TrimSpace(stringOption(req, optionProvider))
	libraryID := strings.TrimSpace(stringOption(req, optionLibraryID))
	if libraryID == "" {
		libraryID = strings.TrimSpace(stringOption(req, optionLibraryName))
	}
	for _, client := range p.Clients {
		if client == nil {
			continue
		}
		if provider != "" && provider != client.Name() {
			continue
		}
		if provider == "" && !matchesContext7Scopes(client.Endpoint.LibraryScopes, libraryID) {
			continue
		}
		return client, nil
	}
	if provider != "" {
		return nil, fmt.Errorf("context7 provider %q not found", provider)
	}
	return nil, fmt.Errorf("no context7 provider matches library %q", libraryID)
}

func matchesContext7Scopes(scopes []string, library string) bool {
	if len(scopes) == 0 || strings.TrimSpace(library) == "" {
		return true
	}
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		if ok, _ := path.Match(scope, library); ok {
			return true
		}
	}
	return false
}

func firstContext7LibraryID(res *engine.Context7LibrarySearchResult) string {
	if res == nil {
		return ""
	}
	for _, lib := range res.Results {
		if strings.TrimSpace(lib.ID) != "" {
			return strings.TrimSpace(lib.ID)
		}
	}
	return ""
}

func boolOption(req capability.Request, key string) bool {
	if req.Options == nil {
		return false
	}
	if v, ok := req.Options[key].(bool); ok {
		return v
	}
	return false
}
