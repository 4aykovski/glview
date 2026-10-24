package rest

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

const tagJSON = `[
 {"name":"v1.14.2","message":"release: v1.14.2","protected":true,
  "commit":{"id":"9fa2c51aaaa","short_id":"9fa2c51","title":"Fix slot race","created_at":"2026-10-20T10:00:00Z"},
  "release":{"tag_name":"v1.14.2","description":"Fixes the slot race\n\nMore"}},
 {"name":"v1.14.1","message":"","protected":false,
  "commit":{"id":"5b1c0aa","short_id":"5b1c0aa","title":"Bump deps","created_at":"2026-10-10T10:00:00Z"},"release":null}]`

func TestTagsList(t *testing.T) {
	var query string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/platform%2Froute-service/repository/tags": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			w.Header().Set("X-Total", "58")
			w.Header().Set("X-Next-Page", "2")
			jsonReply(200, tagJSON)(w, r)
		},
	})
	page, err := c.Tags().List(context.Background(), gitlab.ProjectRef{FullPath: "platform/route-service"}, "v1.1", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"order_by=version", "sort=desc", "search=v1.1", "per_page=20"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query %q lacks %s", query, want)
		}
	}
	if page.Total != 58 || page.NextCursor != "2" || len(page.Items) != 2 {
		t.Fatalf("page = %+v", page)
	}
	t0, t1 := page.Items[0], page.Items[1]
	if t0.Name != "v1.14.2" || t0.Message != "release: v1.14.2" || !t0.Protected || t0.Commit.ShortSHA != "9fa2c51" || t0.Commit.Title != "Fix slot race" {
		t.Fatalf("t0 = %+v", t0)
	}
	if t0.Release == nil || !strings.HasPrefix(t0.Release.Description, "Fixes the slot race") {
		t.Fatalf("release = %+v", t0.Release)
	}
	if t1.Release != nil || t1.Message != "" {
		t.Fatalf("t1 = %+v", t1)
	}
}

func TestTagsPipelines(t *testing.T) {
	var query string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/pipelines": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			jsonReply(200, `[{"id":91761,"iid":120,"status":"success","ref":"v1.14.2","sha":"9fa2c51aaaa","web_url":"https://gitlab.example.com/platform/route-service/-/pipelines/91761","created_at":"2026-10-20T10:01:00Z"}]`)(w, r)
		},
	})
	p := gitlab.ProjectRef{ID: 42, FullPath: "platform/route-service"}
	pl, err := c.Tags().Pipelines(context.Background(), p, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"scope=tags", "per_page=100", "order_by=id", "sort=desc"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query %q lacks %s", query, want)
		}
	}
	if len(pl) != 1 || pl[0].ID != 91761 || pl[0].Ref != "v1.14.2" || pl[0].Status != "success" || pl[0].Project != p {
		t.Fatalf("pl = %+v", pl)
	}
}

func TestTagsCreate(t *testing.T) {
	var body string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v4/projects/42/repository/tags": func(w http.ResponseWriter, r *http.Request) {
			var sb strings.Builder
			buf := make([]byte, 512)
			n, _ := r.Body.Read(buf)
			sb.Write(buf[:n])
			body = sb.String()
			jsonReply(201, `{"name":"v1.14.3","message":"release: v1.14.3","commit":{"id":"0b7d3e9ffff","short_id":"0b7d3e9","title":"Merge"}}`)(w, r)
		},
	})
	p := gitlab.ProjectRef{ID: 42}
	tag, err := c.Tags().Create(context.Background(), p, "v1.14.3", "main", "release: v1.14.3")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"tag_name":"v1.14.3"`, `"ref":"main"`, `"message":"release: v1.14.3"`} {
		if !strings.Contains(body, want) {
			t.Fatalf("body %q lacks %s", body, want)
		}
	}
	if tag.Name != "v1.14.3" || tag.Commit.ShortSHA != "0b7d3e9" {
		t.Fatalf("tag = %+v", tag)
	}
	if _, err = c.Tags().Create(context.Background(), p, "v1.14.4", "main", ""); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "message") {
		t.Fatalf("a lightweight tag sends no message: %q", body)
	}
}

func TestTagsCreateErrors(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v4/projects/42/repository/tags": jsonReply(403, `{"message":"403 Forbidden"}`),
	})
	_, err := c.Tags().Create(context.Background(), gitlab.ProjectRef{ID: 42}, "v1.15.0", "main", "")
	if !errors.Is(err, gitlab.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestTagsCreateDuplicate(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v4/projects/42/repository/tags": jsonReply(400, `{"message":"Tag v1.14.2 already exists"}`),
	})
	_, err := c.Tags().Create(context.Background(), gitlab.ProjectRef{ID: 42}, "v1.14.2", "main", "")
	var ge *gitlab.Error
	if !errors.As(err, &ge) || ge.Status != 400 || ge.Message != "Tag v1.14.2 already exists" {
		t.Fatalf("err = %#v", err)
	}
}

func TestTagsDelete(t *testing.T) {
	var path string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"DELETE /api/v4/projects/42/repository/tags/{name}": func(w http.ResponseWriter, r *http.Request) {
			path = r.PathValue("name")
			if path == "v1.14.2" {
				jsonReply(403, `{"message":"protected"}`)(w, r)
				return
			}
			w.WriteHeader(204)
		},
	})
	p := gitlab.ProjectRef{ID: 42}
	if err := c.Tags().Delete(context.Background(), p, "v1.14.1"); err != nil || path != "v1.14.1" {
		t.Fatalf("err = %v path = %q", err, path)
	}
	if err := c.Tags().Delete(context.Background(), p, "v1.14.2"); !errors.Is(err, gitlab.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}

func TestTagsCompare(t *testing.T) {
	var query string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/repository/compare": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			jsonReply(200, `{"commit":{"id":"0b7d3e9ffff","short_id":"0b7d3e9","title":"Merge fix"},
			 "commits":[{"id":"aaa1","short_id":"aaa1","title":"first"},{"id":"bbb2","short_id":"bbb2","title":"second"}]}`)(w, r)
		},
	})
	cmp, err := c.Tags().Compare(context.Background(), gitlab.ProjectRef{ID: 42}, "v1.14.2", "main")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "from=v1.14.2") || !strings.Contains(query, "to=main") {
		t.Fatalf("query = %q", query)
	}
	if cmp.Head == nil || cmp.Head.ShortSHA != "0b7d3e9" {
		t.Fatalf("head = %+v", cmp.Head)
	}
	if len(cmp.Commits) != 2 || cmp.Commits[0].Title != "second" || cmp.Commits[1].Title != "first" {
		t.Fatalf("commits = %+v, want newest first", cmp.Commits)
	}
}

func TestWorkspaceTags(t *testing.T) {
	var cur, peak atomic.Int32
	var mu sync.Mutex
	var queries []string
	routes := map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/{id}/repository/tags": func(w http.ResponseWriter, r *http.Request) {
			n := cur.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			defer cur.Add(-1)
			time.Sleep(15 * time.Millisecond) // let the requests overlap
			mu.Lock()
			queries = append(queries, r.URL.RawQuery)
			mu.Unlock()
			switch r.PathValue("id") {
			case "7":
				jsonReply(404, `{"message":"404 Project Not Found"}`)(w, r)
			case "8":
				w.Header().Set("X-Total", "0")
				jsonReply(200, `[]`)(w, r)
			default:
				w.Header().Set("X-Total", "58")
				jsonReply(200, tagJSON)(w, r)
			}
		},
		"GET /api/v4/projects/{id}/pipelines": func(w http.ResponseWriter, r *http.Request) {
			jsonReply(200, `[{"id":91761,"status":"success","ref":"v1.14.2"},{"id":91000,"status":"failed","ref":"main"}]`)(w, r)
		},
	}
	c, _ := newServer(t, "", routes)
	var ps []gitlab.ProjectRef
	for i := int64(1); i <= 10; i++ {
		ps = append(ps, gitlab.ProjectRef{ID: i, FullPath: "platform/p" + string(rune('a'+i))})
	}
	groups := map[int64]gitlab.ProjectGroup[gitlab.Tag]{}
	for res := range c.Workspace().Tags(context.Background(), ps, "v1", 3) {
		for _, g := range res.Groups {
			groups[g.Project.ID] = g
		}
	}
	if len(groups) != 10 {
		t.Fatalf("groups = %d", len(groups))
	}
	if peak.Load() > 4 {
		t.Fatalf("parallel requests = %d, want <= 4", peak.Load())
	}
	for _, q := range queries {
		for _, want := range []string{"order_by=version", "sort=desc", "per_page=3", "search=v1"} {
			if !strings.Contains(q, want) {
				t.Fatalf("query %q lacks %s", q, want)
			}
		}
	}
	if g := groups[1]; g.Err != nil || g.Total != 58 || len(g.Items) != 2 || g.Items[0].Pipeline == nil || g.Items[0].Pipeline.ID != 91761 || g.Items[1].Pipeline != nil {
		t.Fatalf("group 1 = %+v", g)
	}
	if g := groups[7]; !errors.Is(g.Err, gitlab.ErrNotFound) {
		t.Fatalf("group 7 err = %v", g.Err)
	}
	if g := groups[8]; g.Err != nil || len(g.Items) != 0 {
		t.Fatalf("group 8 = %+v", g)
	}
}
