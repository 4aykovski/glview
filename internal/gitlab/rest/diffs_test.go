package rest

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestDiffsAllPages(t *testing.T) {
	var pages []string
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/merge_requests/479/diffs": func(w http.ResponseWriter, r *http.Request) {
			q := r.URL.Query()
			pages = append(pages, q.Get("page")+"/"+q.Get("per_page"))
			w.Header().Set("Content-Type", "application/json")
			if q.Get("page") == "1" {
				w.Header().Set("X-Next-Page", "2")
				w.Write([]byte(`[{"old_path":"internal/slots/alloc.go","new_path":"internal/slots/alloc.go","diff":"@@ -1,2 +1,2 @@\n a\n-b\n+c\n","new_file":false,"renamed_file":false,"deleted_file":false,"generated_file":true}]`))
				return
			}
			w.Write([]byte(`[{"old_path":"a.bin","new_path":"b.bin","diff":"","new_file":false,"renamed_file":true,"deleted_file":false,"too_large":true},
				{"old_path":"c.txt","new_path":"c.txt","diff":"","collapsed":true}]`))
		},
	})
	fs, err := c.MergeRequests().Diffs(context.Background(), proj, 479)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(pages, ",") != "1/100,2/100" {
		t.Errorf("pages = %v", pages)
	}
	if len(fs) != 3 {
		t.Fatalf("files = %+v", fs)
	}
	if !fs[0].Generated || fs[0].TooLarge || fs[0].Diff == "" {
		t.Errorf("file 0 = %+v", fs[0])
	}
	if !fs[1].RenamedFile || !fs[1].TooLarge || fs[1].OldPath != "a.bin" || fs[1].NewPath != "b.bin" {
		t.Errorf("file 1 = %+v", fs[1])
	}
	if !fs[2].TooLarge {
		t.Errorf("collapsed must map to TooLarge: %+v", fs[2])
	}
}

func TestCommentLinePosition(t *testing.T) {
	refs := gitlab.DiffRefs{BaseSHA: "b1", StartSHA: "s1", HeadSHA: "h1"}
	for _, tc := range []struct {
		name    string
		pos     gitlab.DiffPosition
		wantNew float64 // 0 = the parameter must be absent
		wantOld float64
	}{
		{"added", gitlab.DiffPosition{DiffRefs: refs, OldPath: "x.go", NewPath: "x.go", NewLine: 42}, 42, 0},
		{"removed", gitlab.DiffPosition{DiffRefs: refs, OldPath: "x.go", NewPath: "x.go", OldLine: 41}, 0, 41},
		{"context", gitlab.DiffPosition{DiffRefs: refs, OldPath: "x.go", NewPath: "x.go", OldLine: 40, NewLine: 40}, 40, 40},
	} {
		var got map[string]any
		c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
			"POST /api/v4/projects/42/merge_requests/479/discussions": func(w http.ResponseWriter, r *http.Request) {
				got = body(t, r)
				jsonReply(201, `{"id":"d9","notes":[{"id":9,"body":"Why?","author":{"username":"egor"},"resolvable":true,"position":{"new_path":"x.go","new_line":42,"old_path":"x.go"}}]}`)(w, r)
			},
		})
		d, err := c.MergeRequests().CommentLine(context.Background(), proj, 479, "Why?", tc.pos)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if d.ID != "d9" || d.Position == nil || d.Position.NewLine != 42 || d.Position.OldPath != "x.go" {
			t.Errorf("%s: discussion = %+v", tc.name, d)
		}
		pos, _ := got["position"].(map[string]any)
		if got["body"] != "Why?" || pos["position_type"] != "text" || pos["base_sha"] != "b1" || pos["start_sha"] != "s1" ||
			pos["head_sha"] != "h1" || pos["old_path"] != "x.go" || pos["new_path"] != "x.go" {
			t.Errorf("%s: request = %v", tc.name, got)
		}
		for key, want := range map[string]float64{"new_line": tc.wantNew, "old_line": tc.wantOld} {
			v, ok := pos[key]
			if (want == 0) == ok || (ok && v != want) {
				t.Errorf("%s: %s = %v, want %v", tc.name, key, v, want)
			}
		}
	}
}

func TestCommentLineBadRequest(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"POST /api/v4/projects/42/merge_requests/479/discussions": jsonReply(400, `{"message":"400 Bad request - line_code can't be blank"}`),
	})
	_, err := c.MergeRequests().CommentLine(context.Background(), proj, 479, "x", gitlab.DiffPosition{NewPath: "x.go", NewLine: 1})
	var ge *gitlab.Error
	if !errors.As(err, &ge) || ge.Status != 400 || !strings.Contains(ge.Message, "line_code") {
		t.Fatalf("err = %v", err)
	}
}

func TestDiscussionPositionFields(t *testing.T) {
	c, _ := newServer(t, "", map[string]func(http.ResponseWriter, *http.Request){
		"GET /api/v4/projects/42/merge_requests/479/discussions": jsonReply(200, `[{"id":"d1","notes":[{"id":1,"body":"x","author":{"username":"anna.k"},"position":{"old_path":"a.go","new_path":"b.go","old_line":7,"new_line":0,"base_sha":"b0b0","start_sha":"5757","head_sha":"a91f"}}]}]`),
	})
	ds, err := c.MergeRequests().Discussions(context.Background(), proj, 479)
	if err != nil {
		t.Fatal(err)
	}
	want := gitlab.NotePosition{Path: "b.go", Line: 0, OldPath: "a.go", NewPath: "b.go", OldLine: 7, BaseSHA: "b0b0", StartSHA: "5757", HeadSHA: "a91f"}
	if ds[0].Position == nil || *ds[0].Position != want {
		t.Fatalf("position = %+v, want %+v", ds[0].Position, want)
	}
}
