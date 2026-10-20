package rest

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

var proj = gitlab.ProjectRef{ID: 42, FullPath: "platform/route-service"}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"17.11.2-ee", true}, {"17.11.0", true}, {"17.10.9", false}, {"18.0.0-pre", true},
		{"16.11.4-ee", false}, {"17.2.0", false}, {"garbage", false}, {"", false}, {"17.x", false},
	}
	for _, c := range cases {
		if got := versionAtLeast(c.v, 17, 11); got != c.want {
			t.Errorf("versionAtLeast(%q) = %v, want %v", c.v, got, c.want)
		}
	}
}

func body(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	b, _ := io.ReadAll(r.Body)
	var m map[string]any
	if len(b) > 0 {
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("request body %q: %v", b, err)
		}
	}
	return m
}

func TestMergeAutoMergeByVersion(t *testing.T) {
	for _, tc := range []struct {
		version, want, notWant string
	}{
		{"17.11.2-ee", "auto_merge", "merge_when_pipeline_succeeds"},
		{"16.11.0", "merge_when_pipeline_succeeds", "auto_merge"},
	} {
		var got map[string]any
		var versionCalls atomic.Int32
		c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
			"GET /api/v4/version": func(w http.ResponseWriter, r *http.Request) {
				versionCalls.Add(1)
				jsonReply(200, `{"version":"`+tc.version+`","revision":"x"}`)(w, r)
			},
			"PUT /api/v4/projects/42/merge_requests/479/merge": func(w http.ResponseWriter, r *http.Request) {
				got = body(t, r)
				jsonReply(200, `{"iid":479,"state":"opened","detailed_merge_status":"mergeable","sha":"abc"}`)(w, r)
			},
		})
		mr, err := c.MergeRequests().Merge(context.Background(), proj, 479, gitlab.MergeOptions{AutoMerge: true, Squash: true, RemoveSourceBranch: false, SHA: "abc"})
		if err != nil {
			t.Fatal(err)
		}
		if got[tc.want] != true {
			t.Errorf("%s: body = %v, want %s=true", tc.version, got, tc.want)
		}
		if _, bad := got[tc.notWant]; bad {
			t.Errorf("%s: body must not contain %s: %v", tc.version, tc.notWant, got)
		}
		if got["sha"] != "abc" || got["squash"] != true || got["should_remove_source_branch"] != false {
			t.Errorf("%s: body = %v", tc.version, got)
		}
		if mr.IID != 479 || mr.MergeStatus != "mergeable" {
			t.Errorf("mr = %+v", mr)
		}
		// Version is cached for the session.
		_, _ = c.MergeRequests().Merge(context.Background(), proj, 479, gitlab.MergeOptions{AutoMerge: true})
		if versionCalls.Load() != 1 {
			t.Errorf("version requested %d times, want 1", versionCalls.Load())
		}
	}
}

func TestMergeNowHasNoAutoFlags(t *testing.T) {
	var got map[string]any
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"PUT /api/v4/projects/42/merge_requests/1/merge": func(w http.ResponseWriter, r *http.Request) {
			got = body(t, r)
			jsonReply(200, `{"iid":1,"state":"merged"}`)(w, r)
		},
	})
	if _, err := c.MergeRequests().Merge(context.Background(), proj, 1, gitlab.MergeOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["auto_merge"]; ok {
		t.Fatalf("body = %v", got)
	}
	if _, ok := got["sha"]; ok {
		t.Fatalf("empty sha must be omitted: %v", got)
	}
}

func TestMergeErrorsMapped(t *testing.T) {
	for status, kind := range map[int]error{409: gitlab.ErrConflict, 405: gitlab.ErrNotPossible, 422: gitlab.ErrNotPossible, 403: gitlab.ErrForbidden} {
		c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
			"PUT /api/v4/projects/42/merge_requests/1/merge": jsonReply(status, `{"message":"SHA does not match HEAD of source branch"}`),
		})
		_, err := c.MergeRequests().Merge(context.Background(), proj, 1, gitlab.MergeOptions{SHA: "old"})
		if !errors.Is(err, kind) {
			t.Errorf("status %d: err = %v, want kind %v", status, err, kind)
		}
	}
}

func TestApproveUnapprove(t *testing.T) {
	var approveBody map[string]any
	var unapproved bool
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v4/projects/42/merge_requests/479/approve": func(w http.ResponseWriter, r *http.Request) {
			approveBody = body(t, r)
			jsonReply(201, `{"id":1,"iid":479}`)(w, r)
		},
		"POST /api/v4/projects/42/merge_requests/479/unapprove": func(w http.ResponseWriter, r *http.Request) {
			unapproved = true
			jsonReply(201, `{}`)(w, r)
		},
	})
	if err := c.MergeRequests().Approve(context.Background(), proj, 479, "a91f2c0"); err != nil {
		t.Fatal(err)
	}
	if approveBody["sha"] != "a91f2c0" {
		t.Errorf("approve body = %v", approveBody)
	}
	if err := c.MergeRequests().Unapprove(context.Background(), proj, 479); err != nil || !unapproved {
		t.Fatalf("unapprove: %v %v", unapproved, err)
	}
}

func TestRebase(t *testing.T) {
	var called bool
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"PUT /api/v4/projects/42/merge_requests/479/rebase": func(w http.ResponseWriter, r *http.Request) {
			called = true
			jsonReply(202, `{"rebase_in_progress":true}`)(w, r)
		},
	})
	if err := c.MergeRequests().Rebase(context.Background(), proj, 479); err != nil || !called {
		t.Fatalf("%v %v", called, err)
	}
}

func TestDiscussionsAllPagesAndPositions(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/merge_requests/479/discussions": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("page") == "1" {
				w.Header().Set("X-Next-Page", "2")
				w.Write([]byte(`[{"id":"d1","notes":[
					{"id":1,"body":"Why?","system":false,"author":{"username":"anna.k","name":"Anna"},"created_at":"2026-10-19T10:00:00Z","resolvable":true,"resolved":false,
					 "position":{"new_path":"internal/slot.go","new_line":42}},
					{"id":2,"body":"Because","system":false,"author":{"username":"egor"},"created_at":"2026-10-19T10:05:00Z","resolvable":true,"resolved":false}]}]`))
				return
			}
			w.Write([]byte(`[{"id":"d2","notes":[{"id":3,"body":"added 1 commit","system":true,"author":{"username":"anna.k"},"created_at":"2026-10-19T11:00:00Z"}]}]`))
		},
	})
	ds, err := c.MergeRequests().Discussions(context.Background(), proj, 479)
	if err != nil {
		t.Fatal(err)
	}
	if len(ds) != 2 {
		t.Fatalf("discussions = %+v", ds)
	}
	d := ds[0]
	if d.Position == nil || d.Position.Path != "internal/slot.go" || d.Position.Line != 42 || !d.Resolvable || d.Resolved || len(d.Notes) != 2 {
		t.Errorf("d1 = %+v", d)
	}
	if !ds[1].Notes[0].System || ds[1].Resolvable {
		t.Errorf("d2 = %+v", ds[1])
	}
}

func TestCommitsAllPagesInAPIOrder(t *testing.T) {
	var pages []string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/merge_requests/479/commits": func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			pages = append(pages, q.Get("page")+"/"+q.Get("per_page"))
			w.Header().Set("Content-Type", "application/json")
			switch q.Get("page") {
			case "1":
				w.Header().Set("X-Next-Page", "2")
				w.Write([]byte(`[{"id":"c3c3c3c3c3c3","short_id":"c3c3c3c3","title":"slot: fix race","author_name":"Anna K","web_url":"https://gitlab.example.com/platform/route-service/-/commit/c3c3c3c3c3c3","created_at":"2026-10-19T11:00:00Z"}]`))
			case "2":
				w.Header().Set("X-Next-Page", "3")
				w.Write([]byte(`[{"id":"b2b2b2b2b2b2","short_id":"b2b2b2b2","title":"slot: add test","author_name":"Egor","created_at":"2026-10-19T10:00:00Z"}]`))
			default:
				w.Write([]byte(`[{"id":"a1a1a1a1a1a1","short_id":"a1a1a1a1","title":"slot: first","author_name":"Egor","created_at":"2026-10-18T09:00:00Z"}]`))
			}
		},
	})
	cs, err := c.MergeRequests().Commits(context.Background(), proj, 479)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(pages, ",") != "1/100,2/100,3/100" {
		t.Errorf("pages = %v", pages)
	}
	if len(cs) != 3 || cs[0].ShortSHA != "c3c3c3c3" || cs[1].ShortSHA != "b2b2b2b2" || cs[2].ShortSHA != "a1a1a1a1" {
		t.Fatalf("commits must keep API order: %+v", cs)
	}
	want := gitlab.CommitSummary{SHA: "c3c3c3c3c3c3", ShortSHA: "c3c3c3c3", Title: "slot: fix race", AuthorName: "Anna K",
		WebURL: "https://gitlab.example.com/platform/route-service/-/commit/c3c3c3c3c3c3", CreatedAt: time.Date(2026, 10, 19, 11, 0, 0, 0, time.UTC)}
	if got := cs[0]; got.SHA != want.SHA || got.Title != want.Title || got.AuthorName != want.AuthorName || got.WebURL != want.WebURL || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("mapping = %+v", got)
	}
}

func TestCommitsError(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/merge_requests/479/commits": jsonReply(404, `{"message":"404 Not found"}`),
	})
	if _, err := c.MergeRequests().Commits(context.Background(), proj, 479); !errors.Is(err, gitlab.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestRecheckMergeStatus(t *testing.T) {
	var query string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/merge_requests/596": func(w http.ResponseWriter, r *http.Request) {
			query = r.URL.RawQuery
			jsonReply(200, `{"iid":596,"detailed_merge_status":"checking"}`)(w, r)
		},
	})
	st, err := c.MergeRequests().RecheckMergeStatus(context.Background(), proj, 596)
	if err != nil || st != "checking" {
		t.Fatalf("status = %q, err = %v", st, err)
	}
	if query != "with_merge_status_recheck=true" {
		t.Errorf("query = %q", query)
	}
}

func TestRecheckMergeStatusForbidden(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/merge_requests/1": jsonReply(403, `{"message":"403 Forbidden"}`),
	})
	_, err := c.MergeRequests().RecheckMergeStatus(context.Background(), proj, 1)
	if !errors.Is(err, gitlab.ErrForbidden) {
		t.Fatalf("err = %v, want ErrForbidden", err)
	}
}
