package httputil

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestClient_Offline(t *testing.T) {
	client := NewClient(WithOffline(true))
	req, err := http.NewRequest(http.MethodGet, "http://example.com", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	_, err = client.Do(req)
	if !errors.Is(err, ErrOffline) {
		t.Fatalf("expected ErrOffline, got: %v", err)
	}
}

func TestClient_UserAgentInjection(t *testing.T) {
	var capturedUA string
	mockRT := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedUA = req.Header.Get("User-Agent")
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
			Header:     make(http.Header),
		}, nil
	})

	customUA := "CustomAgent/1.0"
	client := NewClient(WithTransport(mockRT), WithUserAgent(customUA))

	resp, err := client.Get("http://example.com/test")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if capturedUA != customUA {
		t.Errorf("expected User-Agent %q, got %q", customUA, capturedUA)
	}
}

func TestClient_RetryOn429(t *testing.T) {
	var attempts int32
	mockRT := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		curr := atomic.AddInt32(&attempts, 1)
		if curr <= 2 {
			resp := &http.Response{
				StatusCode: http.StatusTooManyRequests,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("rate limited")),
			}
			resp.Header.Set("Retry-After", "0")
			return resp, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("success")),
		}, nil
	})

	client := NewClient(
		WithTransport(mockRT),
		WithMaxRetries(3),
		WithBaseDelay(1*time.Millisecond),
		WithMaxDelay(10*time.Millisecond),
	)

	resp, err := client.Get("http://example.com/retry429")
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestClient_RetryOn500(t *testing.T) {
	var attempts int32
	mockRT := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		curr := atomic.AddInt32(&attempts, 1)
		if curr == 1 {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("server error")),
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("recovered")),
		}, nil
	})

	client := NewClient(
		WithTransport(mockRT),
		WithMaxRetries(2),
		WithBaseDelay(1*time.Millisecond),
	)

	resp, err := client.Get("http://example.com/retry500")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestClient_NonIdempotentNotRetried(t *testing.T) {
	var attempts int32
	mockRT := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("server error")),
		}, nil
	})

	client := NewClient(
		WithTransport(mockRT),
		WithMaxRetries(3),
		WithBaseDelay(1*time.Millisecond),
	)

	resp, err := client.Post("http://example.com/post", "text/plain", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if attempts != 1 {
		t.Errorf("expected non-idempotent POST to only be attempted once, got %d", attempts)
	}
}

func TestClient_ContextCanceledStopsRetry(t *testing.T) {
	mockRT := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusServiceUnavailable,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("service unavailable")),
		}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	client := NewClient(
		WithTransport(mockRT),
		WithMaxRetries(5),
		WithBaseDelay(50*time.Millisecond),
	)

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.com/timeout", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Fatalf("expected context timeout error, got nil")
	}
}
