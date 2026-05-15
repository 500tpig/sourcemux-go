package router

import (
	"context"
	"errors"
	"testing"

	"github.com/500tpig/sourcemux-go/internal/capability"
)

type fakeProvider struct {
	name    string
	kind    capability.Kind
	content string
	err     error
}

func (p fakeProvider) Name() string          { return p.name }
func (p fakeProvider) Kind() capability.Kind { return p.kind }
func (p fakeProvider) Try(context.Context, capability.Request) (capability.Result, error) {
	if p.err != nil {
		return capability.Result{}, p.err
	}
	return capability.Result{Content: p.content}, nil
}

type matchingProvider struct {
	fakeProvider
	allowed bool
}

func (p matchingProvider) CanHandle(capability.Request) (bool, string) {
	if p.allowed {
		return true, ""
	}
	return false, "not a docs query"
}

func TestRouterFallbackStopsOnFirstOK(t *testing.T) {
	r := New(
		fakeProvider{name: "empty", kind: capability.MainSearch},
		fakeProvider{name: "ok", kind: capability.MainSearch, content: "answer"},
		fakeProvider{name: "unused", kind: capability.MainSearch, content: "unused"},
	)

	res, trace := r.Run(context.Background(), capability.MainSearch, capability.Request{Query: "q"})
	if res.Content != "answer" {
		t.Fatalf("content = %q, want answer", res.Content)
	}
	if trace.FinalProvider != "ok" || !trace.FallbackTriggered || trace.AttemptsCount != 2 {
		t.Fatalf("trace = %+v", trace)
	}
	if trace.Decisions[0].Status != "empty" || trace.Decisions[1].Status != "ok" {
		t.Fatalf("decisions = %+v", trace.Decisions)
	}
}

func TestRouterPermanentErrorStopsFallback(t *testing.T) {
	r := New(
		fakeProvider{name: "bad-auth", kind: capability.MainSearch, err: errors.New("401 unauthorized")},
		fakeProvider{name: "unused", kind: capability.MainSearch, content: "answer"},
	)

	res, trace := r.Run(context.Background(), capability.MainSearch, capability.Request{Query: "q"})
	if res.Content != "" {
		t.Fatalf("content = %q, want empty", res.Content)
	}
	if trace.AttemptsCount != 1 || trace.Decisions[0].Status != "error" {
		t.Fatalf("trace = %+v", trace)
	}
}

func TestRouterStatusDistinguishesRateLimitAndTimeout(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{name: "rate-limit", err: errors.New("429 too many requests"), want: "rate_limited"},
		{name: "timeout", err: context.DeadlineExceeded, want: "timeout"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := New(
				fakeProvider{name: "first", kind: capability.MainSearch, err: tc.err},
				fakeProvider{name: "second", kind: capability.MainSearch, content: "answer"},
			)
			_, trace := r.Run(context.Background(), capability.MainSearch, capability.Request{Query: "q"})
			if len(trace.Decisions) == 0 || trace.Decisions[0].Status != tc.want {
				t.Fatalf("trace = %+v, want first status %q", trace, tc.want)
			}
		})
	}
}

func TestRouterMatcherSkipsWithoutCallingProvider(t *testing.T) {
	r := New(
		matchingProvider{fakeProvider: fakeProvider{name: "docs-provider", kind: capability.DocsSearch, content: "should not run"}, allowed: false},
		fakeProvider{name: "exa", kind: capability.DocsSearch, content: "docs"},
	)

	res, trace := r.Run(context.Background(), capability.DocsSearch, capability.Request{Query: "general docs"})
	if res.Content != "docs" {
		t.Fatalf("content = %q, want docs", res.Content)
	}
	if trace.AttemptsCount != 2 || trace.Decisions[0].Status != "skipped" || trace.Decisions[0].FallbackReason != capability.ReasonNotApplicable {
		t.Fatalf("trace = %+v", trace)
	}
}
