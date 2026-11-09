package httputil

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrOffline indicates network access was requested while offline mode is active.
	ErrOffline = errors.New("network access disabled: patchwork is running in offline mode")
	// DefaultUserAgent is the standard User-Agent header for all outbound HTTP calls.
	DefaultUserAgent = "Patchwork-CLI (+https://github.com/karan-banwasi/Patchwork)"
)

// ClientOptions configures the standard HTTP client.
type ClientOptions struct {
	Timeout    time.Duration
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	UserAgent  string
	Offline    bool
	Transport  http.RoundTripper
}

// Option configures ClientOptions.
type Option func(*ClientOptions)

func WithTimeout(d time.Duration) Option {
	return func(o *ClientOptions) { o.Timeout = d }
}

func WithMaxRetries(n int) Option {
	return func(o *ClientOptions) { o.MaxRetries = n }
}

func WithBaseDelay(d time.Duration) Option {
	return func(o *ClientOptions) { o.BaseDelay = d }
}

func WithMaxDelay(d time.Duration) Option {
	return func(o *ClientOptions) { o.MaxDelay = d }
}

func WithUserAgent(ua string) Option {
	return func(o *ClientOptions) { o.UserAgent = ua }
}

func WithOffline(offline bool) Option {
	return func(o *ClientOptions) { o.Offline = offline }
}

func WithTransport(rt http.RoundTripper) Option {
	return func(o *ClientOptions) { o.Transport = rt }
}

// NewClient returns a standard *http.Client configured with retry transport and timeouts.
func NewClient(opts ...Option) *http.Client {
	cfg := ClientOptions{
		Timeout:    15 * time.Second,
		MaxRetries: 3,
		BaseDelay:  250 * time.Millisecond,
		MaxDelay:   5 * time.Second,
		UserAgent:  DefaultUserAgent,
		Offline:    false,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	baseTransport := cfg.Transport
	if baseTransport == nil {
		baseTransport = http.DefaultTransport
	}

	rt := &RetryTransport{
		Base:       baseTransport,
		MaxRetries: cfg.MaxRetries,
		BaseDelay:  cfg.BaseDelay,
		MaxDelay:   cfg.MaxDelay,
		UserAgent:  cfg.UserAgent,
		Offline:    cfg.Offline,
	}

	return &http.Client{
		Transport: rt,
		Timeout:   cfg.Timeout,
	}
}

// RetryTransport wraps an http.RoundTripper with retries, exponential backoff, jitter, and offline gating.
type RetryTransport struct {
	Base       http.RoundTripper
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
	UserAgent  string
	Offline    bool
	mu         sync.RWMutex
}

func (t *RetryTransport) SetOffline(offline bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Offline = offline
}

func (t *RetryTransport) IsOffline() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Offline
}

func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.IsOffline() {
		return nil, ErrOffline
	}

	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	// Inject default User-Agent if missing
	if req.Header.Get("User-Agent") == "" && t.UserAgent != "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.UserAgent)
	}

	var resp *http.Response
	var err error

	for attempt := 0; attempt <= t.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := t.calculateDelay(attempt, resp)
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(delay):
			}
		}

		// Rewind request body if applicable
		if req.Body != nil && req.GetBody != nil {
			body, bErr := req.GetBody()
			if bErr != nil {
				return nil, fmt.Errorf("failed to rewind request body for retry: %w", bErr)
			}
			req.Body = body
		}

		resp, err = base.RoundTrip(req)

		if !t.shouldRetry(req, resp, err, attempt) {
			return resp, err
		}

		if resp != nil && resp.Body != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}

	return resp, err
}

func (t *RetryTransport) shouldRetry(req *http.Request, resp *http.Response, err error, attempt int) bool {
	if attempt >= t.MaxRetries {
		return false
	}
	if req.Context().Err() != nil {
		return false
	}

	// Only retry idempotent HTTP methods
	method := strings.ToUpper(req.Method)
	if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		return false
	}

	if err != nil {
		// Retry on transient network errors
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return true
		}
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return true
		}
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "connection reset") || strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "wsarecv") {
			return true
		}
		return false
	}

	if resp != nil {
		switch resp.StatusCode {
		case http.StatusTooManyRequests, // 429
			http.StatusInternalServerError, // 500
			http.StatusBadGateway,          // 502
			http.StatusServiceUnavailable,  // 503
			http.StatusGatewayTimeout,      // 504
			http.StatusRequestTimeout:      // 408
			return true
		}
	}

	return false
}

func (t *RetryTransport) calculateDelay(attempt int, resp *http.Response) time.Duration {
	// Check Retry-After header
	if resp != nil && resp.StatusCode == http.StatusTooManyRequests {
		if raw := resp.Header.Get("Retry-After"); raw != "" {
			if secs, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && secs > 0 {
				delay := time.Duration(secs) * time.Second
				if delay > t.MaxDelay {
					return t.MaxDelay
				}
				return delay
			}
			if date, err := http.ParseTime(raw); err == nil {
				delay := time.Until(date)
				if delay > 0 {
					if delay > t.MaxDelay {
						return t.MaxDelay
					}
					return delay
				}
			}
		}
	}

	// Exponential backoff: baseDelay * 2^(attempt-1)
	multiplier := math.Pow(2, float64(attempt-1))
	delay := time.Duration(float64(t.BaseDelay) * multiplier)

	// Add up to 25% jitter
	jitter := time.Duration(rand.Float64() * 0.25 * float64(delay))
	delay = delay + jitter

	if delay > t.MaxDelay {
		return t.MaxDelay
	}
	return delay
}
