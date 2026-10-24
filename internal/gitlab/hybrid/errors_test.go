package hybrid

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/graphql"
	"github.com/4aykovski/glview/internal/gitlab/httpx"
	"github.com/4aykovski/glview/internal/gitlab/rest"
)

// The error table of SPEC §11.1, checked against both transports: a REST
// call and a GraphQL call must give the same domain error for the same
// HTTP answer or transport failure.

var auditProject = gitlab.ProjectRef{FullPath: "platform/route-service"}

// callers are one REST and one GraphQL call against base.
func callers(t *testing.T, base string, hc *http.Client) map[string]func() error {
	t.Helper()
	r, err := rest.New(base, "glpat-test-token-123456", hc)
	if err != nil {
		t.Fatal(err)
	}
	g := graphql.New(base, "glpat-test-token-123456", hc, 10)
	return map[string]func() error{
		"rest": func() error { _, err := r.Users().Current(context.Background()); return err },
		"graphql": func() error {
			_, err := g.ProjectMRs(context.Background(), auditProject, gitlab.MRFilter{}, "egor", "", 20)
			return err
		},
	}
}

func TestHTTPStatusesMapToDomainErrors(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		retryAfter string
		kind       error
		alsoIs     error
		wait       time.Duration
	}{
		{name: "401", status: 401, body: `{"message":"401 Unauthorized"}`, kind: gitlab.ErrUnauthorized},
		{name: "403", status: 403, body: `{"message":"403 Forbidden"}`, kind: gitlab.ErrForbidden},
		{name: "403 insufficient_scope", status: 403, body: `{"error":"insufficient_scope"}`, kind: gitlab.ErrInsufficientScope, alsoIs: gitlab.ErrForbidden},
		{name: "404", status: 404, body: `{"message":"404 Not Found"}`, kind: gitlab.ErrNotFound},
		{name: "409", status: 409, body: `{"message":"SHA does not match HEAD of source branch"}`, kind: gitlab.ErrConflict},
		{name: "405", status: 405, body: `{"message":"405 Method Not Allowed"}`, kind: gitlab.ErrNotPossible},
		{name: "422", status: 422, body: `{"message":"Branch cannot be merged"}`, kind: gitlab.ErrNotPossible},
		{name: "429 with Retry-After", status: 429, body: `{}`, retryAfter: "120", kind: gitlab.ErrRateLimited, wait: 120 * time.Second},
		{name: "429 without Retry-After", status: 429, body: `{}`, kind: gitlab.ErrRateLimited},
		{name: "502", status: 502, body: `<html>bad gateway</html>`, kind: gitlab.ErrServer},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if c.retryAfter != "" {
					w.Header().Set("Retry-After", c.retryAfter)
				}
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			defer srv.Close()
			hc, err := httpx.NewClient(httpx.Options{MaxRetryWait: time.Second})
			if err != nil {
				t.Fatal(err)
			}
			for name, call := range callers(t, srv.URL, hc) {
				err := call()
				if !errors.Is(err, c.kind) || (c.alsoIs != nil && !errors.Is(err, c.alsoIs)) {
					t.Errorf("%s: %v, want %v", name, err, c.kind)
				}
				var ge *gitlab.Error
				if errors.As(err, &ge) && ge.RetryAfter != c.wait {
					t.Errorf("%s: RetryAfter = %v, want %v", name, ge.RetryAfter, c.wait)
				}
			}
		})
	}
}

func TestNullProjectIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"project":null}}`))
	}))
	defer srv.Close()
	hc, _ := httpx.NewClient(httpx.Options{})
	if err := callers(t, srv.URL, hc)["graphql"](); !errors.Is(err, gitlab.ErrNotFound) {
		t.Fatalf("project: null = %v, want ErrNotFound", err)
	}
}

// failingTransport fails every request with err.
type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

func TestTransportFailuresMapToNetworkAndTLS(t *testing.T) {
	// DNS: the lookup error as the dialer reports it.
	dns := &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: "gitlab.example.com", IsNotFound: true}}
	for name, call := range callers(t, "https://gitlab.example.com", &http.Client{Transport: failingTransport{dns}}) {
		if err := call(); !errors.Is(err, gitlab.ErrNetwork) || errors.Is(err, gitlab.ErrTLS) {
			t.Errorf("DNS %s: %v, want ErrNetwork", name, err)
		}
	}

	// Connection refused.
	for name, call := range callers(t, "http://127.0.0.1:1", &http.Client{}) {
		if err := call(); !errors.Is(err, gitlab.ErrNetwork) {
			t.Errorf("refused %s: %v, want ErrNetwork", name, err)
		}
	}

	// Timeout: the server does not answer within the client's timeout.
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer slow.Close()
	hc, _ := httpx.NewClient(httpx.Options{Timeout: 50 * time.Millisecond})
	for name, call := range callers(t, slow.URL, hc) {
		if err := call(); !errors.Is(err, gitlab.ErrNetwork) {
			t.Errorf("timeout %s: %v, want ErrNetwork", name, err)
		}
	}

	// TLS: a certificate the client does not trust.
	secure := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer secure.Close()
	hc, _ = httpx.NewClient(httpx.Options{})
	for name, call := range callers(t, secure.URL, hc) {
		if err := call(); !errors.Is(err, gitlab.ErrTLS) || errors.Is(err, gitlab.ErrNetwork) {
			t.Errorf("TLS %s: %v, want ErrTLS", name, err)
		}
	}
}
