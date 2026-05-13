package router

import (
	"context"
	"time"

	"github.com/500tpig/sourcemux-go/internal/capability"
)

type Router struct {
	providers         map[capability.Kind][]capability.Provider
	CapabilityTimeout time.Duration
}

func New(providers ...capability.Provider) *Router {
	r := &Router{providers: make(map[capability.Kind][]capability.Provider)}
	for _, p := range providers {
		r.Add(p)
	}
	return r
}

func (r *Router) Add(p capability.Provider) {
	if p == nil {
		return
	}
	if r.providers == nil {
		r.providers = make(map[capability.Kind][]capability.Provider)
	}
	r.providers[p.Kind()] = append(r.providers[p.Kind()], p)
}

func (r *Router) Providers(kind capability.Kind) []capability.Provider {
	if r == nil {
		return nil
	}
	return append([]capability.Provider(nil), r.providers[kind]...)
}

func (r *Router) Run(ctx context.Context, kind capability.Kind, req capability.Request) (capability.Result, RouteTrace) {
	var trace RouteTrace
	var finalResult capability.Result
	if r == nil {
		return capability.Result{}, trace
	}
	providers := r.providers[kind]
	for i, p := range providers {
		if m, ok := p.(capability.Matcher); ok {
			allowed, reason := m.CanHandle(req)
			if !allowed {
				trace.Decisions = append(trace.Decisions, RouteDecision{
					Capability:     kind,
					Provider:       p.Name(),
					Attempt:        i + 1,
					Status:         "skipped",
					FallbackReason: capability.ReasonNotApplicable,
					FallbackDetail: reason,
					SubAttempts:    subAttempts(p),
				})
				continue
			}
		}

		attemptCtx := ctx
		cancel := func() {}
		if r.CapabilityTimeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, r.CapabilityTimeout)
		}
		start := time.Now()
		res, err := p.Try(attemptCtx, req)
		cancel()

		outcome, reason, detail := classify(p, res, err)
		decision := RouteDecision{
			Capability:     kind,
			Provider:       p.Name(),
			Attempt:        i + 1,
			Status:         routeStatus(outcome, reason),
			LatencyMS:      time.Since(start).Milliseconds(),
			FallbackReason: reason,
			FallbackDetail: detail,
			SubAttempts:    subAttempts(p),
		}
		trace.Decisions = append(trace.Decisions, decision)
		if outcome == capability.OK {
			trace.FinalProvider = p.Name()
			finalResult = res
			break
		}
		if !outcome.ShouldFallback() {
			break
		}
	}
	trace.AttemptsCount = len(trace.Decisions)
	trace.FallbackTriggered = fallbackTriggered(trace.Decisions, trace.FinalProvider)
	return finalResult, trace
}

func subAttempts(p capability.Provider) int {
	if counter, ok := p.(capability.AttemptCounter); ok {
		return counter.AttemptCount()
	}
	return 0
}

func fallbackTriggered(decisions []RouteDecision, finalProvider string) bool {
	if finalProvider == "" {
		return false
	}
	for _, d := range decisions {
		if d.Provider != finalProvider && d.Status != "skipped" {
			return true
		}
	}
	return false
}

func routeStatus(outcome capability.Outcome, reason capability.FallbackReason) string {
	switch outcome {
	case capability.OK:
		return "ok"
	case capability.Empty:
		return "empty"
	case capability.Canceled:
		return "timeout"
	case capability.Transient:
		switch reason {
		case capability.ReasonRateLimited:
			return "rate_limited"
		case capability.ReasonTimeout:
			return "timeout"
		default:
			return "error"
		}
	default:
		return "error"
	}
}
