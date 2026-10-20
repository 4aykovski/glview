package graphql

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab/httpx"
)

// recorder is a scripted GraphQL endpoint: respond maps a request to the
// raw JSON body of the response.
type recorder struct {
	mu      sync.Mutex
	queries []string
	vars    []map[string]any
	respond func(n int, query string, vars map[string]any) string
}

func newRecorder(t *testing.T, respond func(n int, query string, vars map[string]any) string) (*Client, *recorder) {
	t.Helper()
	rec := &recorder{respond: respond}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		rec.mu.Lock()
		rec.queries = append(rec.queries, req.Query)
		rec.vars = append(rec.vars, req.Variables)
		n := len(rec.queries)
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(respond(n, req.Query, req.Variables)))
	}))
	t.Cleanup(srv.Close)
	hc, _ := httpx.NewClient(httpx.Options{})
	return New(srv.URL, "tok", hc, 10), rec
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.queries)
}

// fixture reads testdata/<name>.
func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
