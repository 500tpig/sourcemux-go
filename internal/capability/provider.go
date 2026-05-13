package capability

import "context"

// Kind identifies a high-level provider capability. Keep the string values
// stable because they are emitted in route_decision JSON.
type Kind string

const (
	MainSearch Kind = "main_search"
	DocsSearch Kind = "docs_search"
	WebFetch   Kind = "web_fetch"
	WebEnhance Kind = "web_enhance"
)

type Source struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

type Request struct {
	Query   string
	URL     string
	Options map[string]any
	Hints   map[string]string
}

type Result struct {
	Content  string
	Sources  []Source
	Metadata map[string]any
}

type Provider interface {
	Name() string
	Kind() Kind
	Try(ctx context.Context, req Request) (Result, error)
}

// Classifier lets providers override generic result/error classification.
type Classifier interface {
	Classify(result Result, err error) (Outcome, FallbackReason, string)
}

// Matcher lets providers skip request shapes they should not handle. Returning
// false must not consume upstream quota; the router records skipped/not_applicable.
type Matcher interface {
	CanHandle(req Request) (bool, string)
}

// AttemptCounter lets pool-style providers expose folded sub-attempts without
// expanding them into many route_decision entries.
type AttemptCounter interface {
	AttemptCount() int
}
