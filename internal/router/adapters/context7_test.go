package adapters

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/500tpig/grok-search-go/internal/capability"
	"github.com/500tpig/grok-search-go/internal/engine"
)

func TestContext7DocsProvider_CanHandleRequiresExplicitLibrary(t *testing.T) {
	p := NewContext7Docs(nil)
	if ok, _ := p.CanHandle(capability.Request{Query: "react hooks"}); ok {
		t.Fatal("expected generic query to be skipped")
	}
	if ok, _ := p.CanHandle(capability.Request{Query: "hooks", Options: map[string]any{"library_id": "/facebook/react"}}); !ok {
		t.Fatal("expected library_id request to be handled")
	}
}

func TestContext7DocsProvider_ClassifyHTTPStatuses(t *testing.T) {
	p := NewContext7Docs(nil)
	for _, code := range []int{http.StatusBadRequest, http.StatusUnauthorized} {
		outcome, _, _ := p.Classify(capability.Result{}, engine.HTTPStatusError{Provider: "context7", StatusCode: code})
		if outcome != capability.Permanent {
			t.Fatalf("status %d outcome = %s", code, outcome)
		}
	}
	for _, code := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity, http.StatusTooManyRequests, http.StatusInternalServerError} {
		outcome, _, _ := p.Classify(capability.Result{}, engine.HTTPStatusError{Provider: "context7", StatusCode: code})
		if outcome != capability.Transient {
			t.Fatalf("status %d outcome = %s", code, outcome)
		}
	}
	outcome, _, _ := p.Classify(capability.Result{}, errors.New("timeout"))
	if outcome != capability.Transient {
		t.Fatalf("timeout outcome = %s", outcome)
	}
}

func TestContext7DocsProvider_SelectsByProviderAndScope(t *testing.T) {
	p := NewContext7Docs([]*engine.Context7Client{
		engine.NewContext7Client(engine.Context7Endpoint{Name: "react", APIKey: "a", LibraryScopes: []string{"/facebook/*"}}),
		engine.NewContext7Client(engine.Context7Endpoint{Name: "next", APIKey: "b", LibraryScopes: []string{"/vercel/*"}}),
	})
	client, err := p.selectClient(capability.Request{Options: map[string]any{"library_id": "/vercel/next.js"}})
	if err != nil {
		t.Fatalf("selectClient failed: %v", err)
	}
	if client.Name() != "next" {
		t.Fatalf("client = %s", client.Name())
	}
	client, err = p.selectClient(capability.Request{Options: map[string]any{"provider": "react", "library_id": "/vercel/next.js"}})
	if err != nil {
		t.Fatalf("explicit provider should override scope: %v", err)
	}
	if client.Name() != "react" {
		t.Fatalf("client = %s", client.Name())
	}
}

func TestContext7DocsProvider_NoNetworkWhenNotApplicable(t *testing.T) {
	called := false
	p := NewContext7Docs([]*engine.Context7Client{{
		Endpoint: engine.Context7Endpoint{Name: "ctx", APIKey: "a"},
		BaseURL:  "http://127.0.0.1:1",
	}})
	if ok, _ := p.CanHandle(capability.Request{Query: "general docs"}); ok {
		called = true
		_, _ = p.Try(context.Background(), capability.Request{Query: "general docs"})
	}
	if called {
		t.Fatal("provider should be skipped before Try")
	}
}
