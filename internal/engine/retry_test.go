package engine

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fastNoSleepConfig is a deterministic policy useful for retry unit tests:
// no jitter, zero backoff, attempts capped at the supplied count.
func fastNoSleepConfig(attempts int) RetryConfig {
	return RetryConfig{MaxAttempts: attempts, BaseDelay: 0, MaxDelay: 0, Jitter: false}
}

// recordingSleepConfig captures every requested sleep duration without actually
// sleeping, so assertions can verify Retry-After / backoff math precisely.
func recordingSleepConfig(attempts int, base, max time.Duration, calls *[]time.Duration) RetryConfig {
	return RetryConfig{
		MaxAttempts: attempts,
		BaseDelay:   base,
		MaxDelay:    max,
		Jitter:      false,
		sleep: func(ctx context.Context, d time.Duration) error {
			*calls = append(*calls, d)
			return nil
		},
	}
}

func newRetryServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, func() (*http.Request, error)) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	factory := func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, srv.URL, nil)
	}
	return srv, factory
}

func TestParseRetryAfter_Seconds(t *testing.T) {
	d, ok := parseRetryAfter("42", time.Now())
	if !ok || d != 42*time.Second {
		t.Fatalf("got (%v, %v)", d, ok)
	}
}

func TestParseRetryAfter_NegativeSecondsClampsToZero(t *testing.T) {
	d, ok := parseRetryAfter("-5", time.Now())
	if !ok || d != 0 {
		t.Fatalf("negative seconds should clamp; got (%v, %v)", d, ok)
	}
}

func TestParseRetryAfter_HTTPDateInFuture(t *testing.T) {
	now := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	future := now.Add(90 * time.Second)
	d, ok := parseRetryAfter(future.Format(http.TimeFormat), now)
	if !ok {
		t.Fatal("expected ok for valid HTTP-date")
	}
	// HTTP TimeFormat is second-precision.
	if d < 89*time.Second || d > 91*time.Second {
		t.Fatalf("expected ~90s, got %v", d)
	}
}

func TestParseRetryAfter_HTTPDateInPastClampsToZero(t *testing.T) {
	now := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour)
	d, ok := parseRetryAfter(past.Format(http.TimeFormat), now)
	if !ok || d != 0 {
		t.Fatalf("got (%v, %v)", d, ok)
	}
}

func TestParseRetryAfter_EmptyAndGarbage(t *testing.T) {
	if _, ok := parseRetryAfter("", time.Now()); ok {
		t.Fatal("empty should not parse")
	}
	if _, ok := parseRetryAfter("not-a-thing", time.Now()); ok {
		t.Fatal("garbage should not parse")
	}
}

func TestShouldRetryStatus(t *testing.T) {
	retryable := []int{429, 500, 502, 503, 504, 599}
	notRetryable := []int{200, 201, 301, 400, 401, 403, 404, 409, 422}
	for _, c := range retryable {
		if !shouldRetryStatus(c) {
			t.Errorf("%d should be retryable", c)
		}
	}
	for _, c := range notRetryable {
		if shouldRetryStatus(c) {
			t.Errorf("%d should NOT be retryable", c)
		}
	}
}

func TestHTTPDoWithRetry_Retries429ThenSucceeds(t *testing.T) {
	var calls int32
	_, factory := newRetryServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	})

	resp, err := httpDoWithRetry(context.Background(), http.DefaultClient, factory, fastNoSleepConfig(3))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("final status = %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("expected 2 calls, got %d", got)
	}
}

func TestHTTPDoWithRetry_Retries5xxThenSucceeds(t *testing.T) {
	var calls int32
	_, factory := newRetryServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		switch n {
		case 1:
			w.WriteHeader(http.StatusInternalServerError)
		case 2:
			w.WriteHeader(http.StatusBadGateway)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})

	resp, err := httpDoWithRetry(context.Background(), http.DefaultClient, factory, fastNoSleepConfig(3))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("final status = %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 calls, got %d", got)
	}
}

func TestHTTPDoWithRetry_NonRetryable4xxNotRetried(t *testing.T) {
	var calls int32
	_, factory := newRetryServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	})

	resp, err := httpDoWithRetry(context.Background(), http.DefaultClient, factory, fastNoSleepConfig(5))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("4xx must not retry; got %d calls", got)
	}
}

func TestHTTPDoWithRetry_ExhaustsAttemptsReturnsLastResponse(t *testing.T) {
	var calls int32
	_, factory := newRetryServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	resp, err := httpDoWithRetry(context.Background(), http.DefaultClient, factory, fastNoSleepConfig(3))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 503 {
		t.Fatalf("final status = %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("expected 3 calls, got %d", got)
	}
}

func TestHTTPDoWithRetry_RespectsRetryAfterSeconds(t *testing.T) {
	var calls int32
	_, factory := newRetryServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	var sleeps []time.Duration
	cfg := recordingSleepConfig(3, 100*time.Millisecond, 30*time.Second, &sleeps)

	resp, err := httpDoWithRetry(context.Background(), http.DefaultClient, factory, cfg)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()
	if len(sleeps) != 1 || sleeps[0] != 7*time.Second {
		t.Fatalf("expected one 7s sleep, got %v", sleeps)
	}
}

func TestHTTPDoWithRetry_RetryAfterCappedByMaxDelay(t *testing.T) {
	var calls int32
	_, factory := newRetryServer(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	var sleeps []time.Duration
	cfg := recordingSleepConfig(3, 100*time.Millisecond, 5*time.Second, &sleeps)

	resp, err := httpDoWithRetry(context.Background(), http.DefaultClient, factory, cfg)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()
	if len(sleeps) != 1 || sleeps[0] != 5*time.Second {
		t.Fatalf("Retry-After should cap at MaxDelay=5s, got %v", sleeps)
	}
}

func TestHTTPDoWithRetry_ExponentialBackoffWithoutJitter(t *testing.T) {
	var calls int32
	_, factory := newRetryServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusInternalServerError)
	})

	var sleeps []time.Duration
	cfg := recordingSleepConfig(4, 100*time.Millisecond, 10*time.Second, &sleeps)

	resp, _ := httpDoWithRetry(context.Background(), http.DefaultClient, factory, cfg)
	if resp != nil {
		resp.Body.Close()
	}
	want := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond, 400 * time.Millisecond}
	if len(sleeps) != len(want) {
		t.Fatalf("sleeps = %v, want %v", sleeps, want)
	}
	for i, d := range want {
		if sleeps[i] != d {
			t.Fatalf("sleep[%d] = %v, want %v (full=%v)", i, sleeps[i], d, sleeps)
		}
	}
}

func TestHTTPDoWithRetry_NetworkErrorRetries(t *testing.T) {
	// Server that closes the connection without writing a response on the first
	// call, then responds 200 thereafter. The first Do() returns a transport
	// error which the retry loop should treat as retryable.
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		if n == 1 {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatal("hijack unsupported")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatal(err)
			}
			_ = conn.Close()
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	factory := func() (*http.Request, error) {
		return http.NewRequest(http.MethodGet, srv.URL, nil)
	}
	resp, err := httpDoWithRetry(context.Background(), http.DefaultClient, factory, fastNoSleepConfig(3))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&calls); got < 2 {
		t.Fatalf("expected at least 2 calls, got %d", got)
	}
}

func TestHTTPDoWithRetry_NetworkErrorExhausts(t *testing.T) {
	// Point at a closed listener so every Do() fails with a transport error.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	var attempts int
	factory := func() (*http.Request, error) {
		attempts++
		return http.NewRequest(http.MethodGet, url, nil)
	}
	_, err := httpDoWithRetry(context.Background(), http.DefaultClient, factory, fastNoSleepConfig(3))
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}

func TestHTTPDoWithRetry_ContextCancellationStops(t *testing.T) {
	_, factory := newRetryServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	ctx, cancel := context.WithCancel(context.Background())
	cfg := RetryConfig{
		MaxAttempts: 5,
		BaseDelay:   10 * time.Millisecond,
		MaxDelay:    time.Second,
		Jitter:      false,
		sleep: func(ctx context.Context, d time.Duration) error {
			cancel() // simulate cancellation during the backoff sleep
			return ctx.Err()
		},
	}
	_, err := httpDoWithRetry(ctx, http.DefaultClient, factory, cfg)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestHTTPDoWithRetry_PrebuildErrorReturnedImmediately(t *testing.T) {
	wantErr := errors.New("boom")
	factory := func() (*http.Request, error) { return nil, wantErr }
	_, err := httpDoWithRetry(context.Background(), http.DefaultClient, factory, fastNoSleepConfig(3))
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected wrapped boom error, got %v", err)
	}
}
