package rest

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/httpx"
)

// newServer serves fixtures under the given prefix (relative URL root).
func newServer(t *testing.T, prefix string, routes map[string]func(w http.ResponseWriter, r *http.Request)) (*Client, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	for pattern, h := range routes {
		mux.HandleFunc(pattern, h)
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	hc, err := httpx.NewClient(httpx.Options{})
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(srv.URL+prefix, "glpat-test-token-123456", hc)
	if err != nil {
		t.Fatal(err)
	}
	return c, srv
}

func jsonReply(status int, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(body))
	}
}

func TestCurrentUser(t *testing.T) {
	var gotToken string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/user": func(w http.ResponseWriter, r *http.Request) {
			gotToken = r.Header.Get("PRIVATE-TOKEN")
			jsonReply(200, `{"id":7,"username":"egor","name":"Egor S","web_url":"https://gitlab.example.com/egor"}`)(w, r)
		},
	})
	u, err := c.Users().Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.ID != 7 || u.Username != "egor" || u.Name != "Egor S" {
		t.Fatalf("user = %+v", u)
	}
	if gotToken != "glpat-test-token-123456" {
		t.Fatalf("token header = %q", gotToken)
	}
}

func TestCurrentUserUnauthorized(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/user": jsonReply(401, `{"message":"401 Unauthorized"}`),
	})
	_, err := c.Users().Current(context.Background())
	if !errors.Is(err, gitlab.ErrUnauthorized) {
		t.Fatalf("want ErrUnauthorized, got %v", err)
	}
}

func TestRelativeURLRoot(t *testing.T) {
	c, _ := newServer(t, "/gitlab", map[string]func(http.ResponseWriter, *http.Request){
		"GET /gitlab/api/v4/version": jsonReply(200, `{"version":"17.11.2-ee","revision":"abc"}`),
	})
	v, err := c.Instance().Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != "17.11.2-ee" {
		t.Fatalf("version = %+v", v)
	}
	if got := c.Instance().URL(); got[len(got)-7:] != "/gitlab" {
		t.Fatalf("URL = %q", got)
	}
}

func TestGetProjectByPath(t *testing.T) {
	var gotURI string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/{id}": func(w http.ResponseWriter, r *http.Request) {
			gotURI = r.URL.EscapedPath()
			jsonReply(200, `{"id":42,"name":"route-service","path_with_namespace":"platform/route-service",
				"web_url":"https://gitlab.example.com/platform/route-service","default_branch":"main",
				"last_activity_at":"2026-10-19T08:00:00Z","archived":false}`)(w, r)
		},
	})
	p, err := c.Projects().Get(context.Background(), gitlab.ProjectRef{FullPath: "platform/route-service"})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != 42 || p.FullPath != "platform/route-service" || p.DefaultBranch != "main" || p.LastActivityAt.IsZero() {
		t.Fatalf("project = %+v", p)
	}
	if gotURI != "/api/v4/projects/platform%2Froute-service" {
		t.Fatalf("path must be URL-encoded, got %q", gotURI)
	}
}

func TestGetProjectNotFoundAndScope(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/{id}": jsonReply(404, `{"message":"404 Project Not Found"}`),
		"GET /api/v4/user":          jsonReply(403, `{"error":"insufficient_scope"}`),
	})
	if _, err := c.Projects().Get(context.Background(), gitlab.ProjectRef{FullPath: "a/b"}); !errors.Is(err, gitlab.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if _, err := c.Users().Current(context.Background()); !errors.Is(err, gitlab.ErrInsufficientScope) {
		t.Fatalf("want ErrInsufficientScope, got %v", err)
	}
}

func TestNetworkError(t *testing.T) {
	hc, _ := httpx.NewClient(httpx.Options{})
	c, err := New("http://127.0.0.1:1", "t", hc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Users().Current(context.Background()); !errors.Is(err, gitlab.ErrNetwork) {
		t.Fatalf("want ErrNetwork, got %v", err)
	}
}
