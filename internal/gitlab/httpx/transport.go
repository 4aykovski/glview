// Package httpx builds the shared HTTP client used by both the REST and the
// GraphQL transports: per-host concurrency limit, a single 429 retry that
// honours Retry-After, custom TLS settings and redacted debug logging.
package httpx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/4aykovski/glview/internal/debuglog"
)

// Options configure NewClient. Zero values select sensible defaults.
type Options struct {
	CAFile             string
	InsecureSkipVerify bool
	MaxConcurrent      int           // default 6
	Timeout            time.Duration // per request, default 15s
	MaxRetryWait       time.Duration // longest Retry-After we honour, default 30s
	UserAgent          string
}

// NewClient returns an *http.Client with the gitlab-tui transport chain.
func NewClient(o Options) (*http.Client, error) {
	if o.MaxConcurrent <= 0 {
		o.MaxConcurrent = 6
	}
	if o.Timeout <= 0 {
		o.Timeout = 15 * time.Second
	}
	if o.MaxRetryWait <= 0 {
		o.MaxRetryWait = 30 * time.Second
	}

	base := http.DefaultTransport.(*http.Transport).Clone()
	if o.CAFile != "" || o.InsecureSkipVerify {
		tlsCfg := &tls.Config{InsecureSkipVerify: o.InsecureSkipVerify} //nolint:gosec // explicit user opt-in
		if o.CAFile != "" {
			pem, err := os.ReadFile(o.CAFile)
			if err != nil {
				return nil, fmt.Errorf("read ca_file: %w", err)
			}
			pool, err := x509.SystemCertPool()
			if err != nil || pool == nil {
				pool = x509.NewCertPool()
			}
			if !pool.AppendCertsFromPEM(pem) {
				return nil, fmt.Errorf("ca_file %s: no certificates found", o.CAFile)
			}
			tlsCfg.RootCAs = pool
		}
		base.TLSClientConfig = tlsCfg
	}

	return &http.Client{
		Timeout: o.Timeout,
		Transport: &Transport{
			Base:         base,
			Sem:          make(chan struct{}, o.MaxConcurrent),
			MaxRetryWait: o.MaxRetryWait,
			UserAgent:    o.UserAgent,
		},
	}, nil
}

// Transport limits concurrency and retries a 429 once.
type Transport struct {
	Base         http.RoundTripper
	Sem          chan struct{}
	MaxRetryWait time.Duration
	UserAgent    string
	// Sleep is replaceable in tests.
	Sleep func(ctx context.Context, d time.Duration) error
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.UserAgent != "" && req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.UserAgent)
	}
	resp, err := t.do(req)
	if err != nil || resp.StatusCode != http.StatusTooManyRequests {
		return resp, err
	}

	wait, ok := RetryAfter(resp.Header, time.Now())
	if !ok {
		wait = time.Second
	}
	if wait > t.MaxRetryWait || (req.Body != nil && req.GetBody == nil) {
		return resp, nil
	}
	drain(resp)
	if err := t.sleep(req.Context(), wait); err != nil {
		return nil, err
	}
	retry := req.Clone(req.Context())
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		retry.Body = body
	}
	return t.do(retry)
}

func (t *Transport) do(req *http.Request) (*http.Response, error) {
	if t.Sem != nil {
		select {
		case t.Sem <- struct{}{}:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		defer func() { <-t.Sem }()
	}
	start := time.Now()
	resp, err := t.Base.RoundTrip(req)
	log := debuglog.L()
	if err != nil {
		log.Debug("http", "method", req.Method, "path", req.URL.Path, "err", err.Error(), "dur", time.Since(start))
		return nil, err
	}
	log.Debug("http", "method", req.Method, "path", req.URL.Path, "status", resp.StatusCode, "dur", time.Since(start))
	return resp, nil
}

func (t *Transport) sleep(ctx context.Context, d time.Duration) error {
	if t.Sleep != nil {
		return t.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RetryAfter parses the Retry-After header (seconds or HTTP date).
func RetryAfter(h http.Header, now time.Time) (time.Duration, bool) {
	v := h.Get("Retry-After")
	if v == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(v); err == nil && secs >= 0 {
		return time.Duration(secs) * time.Second, true
	}
	if at, err := http.ParseTime(v); err == nil {
		if d := at.Sub(now); d > 0 {
			return d, true
		}
		return 0, true
	}
	return 0, false
}

func drain(resp *http.Response) {
	if resp.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		resp.Body.Close()
	}
}
