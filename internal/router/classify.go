package router

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/500tpig/sourcemux-go/internal/capability"
	"github.com/500tpig/sourcemux-go/internal/engine"
)

func classify(p capability.Provider, res capability.Result, err error) (capability.Outcome, capability.FallbackReason, string) {
	var outcome capability.Outcome
	var reason capability.FallbackReason
	var detail string
	if c, ok := p.(capability.Classifier); ok {
		outcome, reason, detail = c.Classify(res, err)
	} else {
		outcome, reason, detail = DefaultClassify(res, err)
	}
	if outcome == capability.OK && searchKindRequiresSources(p.Kind()) && validSourceCount(res) == 0 {
		return capability.Empty, capability.ReasonNoValidSource, "search response contained no valid HTTP(S) sources"
	}
	return outcome, reason, detail
}

func DefaultClassify(res capability.Result, err error) (capability.Outcome, capability.FallbackReason, string) {
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return capability.Canceled, capability.ReasonTimeout, err.Error()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return capability.Transient, capability.ReasonTimeout, err.Error()
		}
		var statusErr engine.HTTPStatusError
		if errors.As(err, &statusErr) {
			switch {
			case statusErr.StatusCode == http.StatusTooManyRequests:
				return capability.Transient, capability.ReasonRateLimited, statusErr.Error()
			case statusErr.StatusCode == http.StatusUnauthorized || statusErr.StatusCode == http.StatusForbidden:
				return capability.Transient, capability.ReasonConfiguration, statusErr.Error()
			case statusErr.StatusCode >= 500:
				return capability.Transient, capability.ReasonUpstreamError, statusErr.Error()
			case statusErr.StatusCode >= 400:
				return capability.Permanent, capability.ReasonUpstreamError, statusErr.Error()
			}
		}
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "429") || strings.Contains(msg, "rate limit") || strings.Contains(msg, "too many requests") {
			return capability.Transient, capability.ReasonRateLimited, err.Error()
		}
		if strings.Contains(msg, "deadline") || strings.Contains(msg, "timeout") || strings.Contains(msg, "timed out") {
			return capability.Transient, capability.ReasonTimeout, err.Error()
		}
		if strings.Contains(msg, "401") || strings.Contains(msg, "403") {
			return capability.Transient, capability.ReasonConfiguration, err.Error()
		}
		if strings.Contains(msg, "400") {
			return capability.Permanent, capability.ReasonUpstreamError, err.Error()
		}
		return capability.Transient, capability.ReasonUpstreamError, err.Error()
	}
	if strings.TrimSpace(res.Content) == "" {
		return capability.Empty, capability.ReasonNoContent, "empty content"
	}
	return capability.OK, capability.ReasonNone, ""
}

func searchKindRequiresSources(kind capability.Kind) bool {
	return kind == capability.MainSearch || kind == capability.DocsSearch
}

func validSourceCount(res capability.Result) int {
	count := 0
	for _, source := range res.Sources {
		if _, ok := engine.NormalizeSourceURL(source.URL); ok {
			count++
		}
	}
	return count
}
