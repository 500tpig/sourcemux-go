package engine

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RetryConfig configures httpDoWithRetry. Use DefaultRetryConfig() for sane
// defaults; tests can override Jitter/BaseDelay (and the unexported sleep/now
// hooks) for determinism.
type RetryConfig struct {
	// MaxAttempts is the total number of attempts (including the first).
	// Values < 1 are treated as 1, i.e. no retry.
	MaxAttempts int
	// BaseDelay is the initial backoff between attempts; doubled each retry.
	BaseDelay time.Duration
	// MaxDelay caps a single backoff sleep, including a server-supplied
	// Retry-After value. Zero or negative means no cap.
	MaxDelay time.Duration
	// Jitter, when true, multiplies each computed backoff by a random factor
	// in [0.5, 1.0). Disabled in tests for determinism. Has no effect when a
	// Retry-After header drives the wait.
	Jitter bool

	// now is a clock injection point for parsing HTTP-date Retry-After values.
	// Nil falls back to time.Now.
	now func() time.Time
	// sleep is a context-aware sleep injection point. Nil falls back to a
	// real timer that respects ctx cancellation.
	sleep func(ctx context.Context, d time.Duration) error
}

// HTTPStatusError preserves an upstream HTTP status for callers that need to
// classify permanent vs transient failures without string parsing.
type HTTPStatusError struct {
	Provider   string
	StatusCode int
	Body       string
}

func (e HTTPStatusError) Error() string {
	provider := strings.TrimSpace(e.Provider)
	if provider == "" {
		provider = "upstream"
	}
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("%s API %d", provider, e.StatusCode)
	}
	return fmt.Sprintf("%s API %d: %s", provider, e.StatusCode, body)
}

// DefaultRetryConfig returns the default policy: 3 total attempts (initial + 2
// retries), 500ms base, 30s max delay, jitter on.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: 3,
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    30 * time.Second,
		Jitter:      true,
	}
}

// httpDoWithRetry executes the request returned by reqFactory. It retries on
// network errors, HTTP 429, and 5xx responses with capped exponential backoff.
// A server-supplied Retry-After header (delta-seconds or HTTP-date) takes
// precedence over the computed backoff but is still capped by cfg.MaxDelay.
//
// reqFactory is invoked before every attempt so callers with non-replayable
// request bodies (e.g. an io.Reader created from json.Marshal) can hand back
// a fresh *http.Request each time. The returned response on success is the
// caller's responsibility to close.
func httpDoWithRetry(ctx context.Context, client *http.Client, reqFactory func() (*http.Request, error), cfg RetryConfig) (*http.Response, error) {
	if client == nil {
		return nil, fmt.Errorf("httpDoWithRetry: nil client")
	}
	if reqFactory == nil {
		return nil, fmt.Errorf("httpDoWithRetry: nil reqFactory")
	}
	if cfg.MaxAttempts < 1 {
		cfg.MaxAttempts = 1
	}
	nowFn := cfg.now
	if nowFn == nil {
		nowFn = time.Now
	}
	sleepFn := cfg.sleep
	if sleepFn == nil {
		sleepFn = sleepWithContext
	}

	var lastErr error
	for attempt := 0; attempt < cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return nil, fmt.Errorf("%w (last attempt error: %v)", err, lastErr)
			}
			return nil, err
		}

		req, err := reqFactory()
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			// Context cancellation surfaces as a Do error; don't retry it.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			lastErr = err
			if attempt == cfg.MaxAttempts-1 {
				return nil, err
			}
			wait := backoffDelay(cfg, attempt, 0)
			if waitErr := sleepFn(ctx, wait); waitErr != nil {
				return nil, waitErr
			}
			continue
		}

		if !shouldRetryStatus(resp.StatusCode) || attempt == cfg.MaxAttempts-1 {
			return resp, nil
		}

		retryAfter, _ := parseRetryAfter(resp.Header.Get("Retry-After"), nowFn())
		// Drain & close so the underlying connection can be reused.
		drainAndClose(resp)
		lastErr = fmt.Errorf("http %d", resp.StatusCode)

		wait := backoffDelay(cfg, attempt, retryAfter)
		if waitErr := sleepFn(ctx, wait); waitErr != nil {
			return nil, waitErr
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("exhausted %d attempts", cfg.MaxAttempts)
	}
	return nil, lastErr
}

// shouldRetryStatus returns true for status codes that warrant another attempt:
// 429 Too Many Requests and 5xx server errors. 4xx other than 429 are caller
// errors and should not be retried.
func shouldRetryStatus(code int) bool {
	return code == http.StatusTooManyRequests || (code >= 500 && code <= 599)
}

// backoffDelay returns the wait duration before the (attempt+1)-th retry.
// retryAfter, when > 0, overrides the exponential backoff.
func backoffDelay(cfg RetryConfig, attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		if cfg.MaxDelay > 0 && retryAfter > cfg.MaxDelay {
			return cfg.MaxDelay
		}
		return retryAfter
	}
	if cfg.BaseDelay <= 0 {
		return 0
	}
	d := time.Duration(float64(cfg.BaseDelay) * math.Pow(2, float64(attempt)))
	if cfg.MaxDelay > 0 && d > cfg.MaxDelay {
		d = cfg.MaxDelay
	}
	if cfg.Jitter {
		// [0.5, 1.0) full-jitter-ish: keeps backoff monotone-ish but spreads
		// concurrent retries to avoid thundering herd against an upstream
		// proxy that is just coming back online.
		f := 0.5 + rand.Float64()*0.5
		d = time.Duration(float64(d) * f)
	}
	return d
}

// parseRetryAfter parses an RFC 7231 Retry-After header value. The header may
// be either delta-seconds ("30") or an HTTP-date. Returns (duration, true) on
// success; (0, false) when the header is missing or malformed. Negative
// computed durations (HTTP-date already in the past) clamp to 0 with ok=true,
// signalling "retry immediately".
func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(value); err == nil {
		if secs < 0 {
			return 0, true
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(value); err == nil {
		if d := t.Sub(now); d > 0 {
			return d, true
		}
		return 0, true
	}
	return 0, false
}

// sleepWithContext sleeps for d, returning early with ctx.Err() if ctx is
// cancelled. d <= 0 returns immediately.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// drainAndClose drains the response body so the connection can be reused, then
// closes it. Errors are intentionally ignored — at this point we are about to
// retry and a stale body is not interesting.
func drainAndClose(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
}
