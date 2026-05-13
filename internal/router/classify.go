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
	if c, ok := p.(capability.Classifier); ok {
		return c.Classify(res, err)
	}
	return DefaultClassify(res, err)
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
		if strings.Contains(msg, "401") || strings.Contains(msg, "403") || strings.Contains(msg, "400") {
			return capability.Permanent, capability.ReasonUpstreamError, err.Error()
		}
		return capability.Transient, capability.ReasonUpstreamError, err.Error()
	}
	if strings.TrimSpace(res.Content) == "" {
		return capability.Empty, capability.ReasonNoContent, "empty content"
	}
	return capability.OK, capability.ReasonNone, ""
}
