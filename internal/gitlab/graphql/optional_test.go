package graphql

import (
	"context"
	"strings"
	"testing"
)

func TestParseGID(t *testing.T) {
	for in, want := range map[string]int64{
		"gid://gitlab/Ci::Pipeline/123":   123,
		"gid://gitlab/MergeRequest/7":     7,
		"gid://gitlab/Project/9999999999": 9999999999,
	} {
		got, err := parseGID(in)
		if err != nil || got != want {
			t.Errorf("parseGID(%q) = %d, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "123", "gid://gitlab/Project/x"} {
		if _, err := parseGID(bad); err == nil && bad != "123" {
			t.Errorf("parseGID(%q) must fail", bad)
		}
	}
	if lower("NOT_APPROVED") != "not_approved" {
		t.Fatal("lower")
	}
}

const optQuery = `query {
  project(fullPath: "a/b") {
    name
    #@opt diffStatsSummary
    diffStatsSummary { additions }
    #@end
    #@opt a,b
    a b
    #@end
  }
}`

func TestOptionalFieldRetriedOnceThenRemembered(t *testing.T) {
	c, rec := newRecorder(t, func(n int, query string, _ map[string]any) string {
		if strings.Contains(query, "diffStatsSummary") {
			return `{"errors":[{"message":"Field 'diffStatsSummary' doesn't exist on type 'Project'"}]}`
		}
		return `{"data":{"project":{"name":"x"}}}`
	})
	var out struct {
		Project struct{ Name string } `json:"project"`
	}
	if err := c.DoOpt(context.Background(), optQuery, nil, &out); err != nil || out.Project.Name != "x" {
		t.Fatalf("first: %+v %v", out, err)
	}
	if rec.count() != 2 {
		t.Fatalf("requests = %d, want 2 (one retry)", rec.count())
	}
	if strings.Contains(rec.queries[1], "diffStatsSummary") || strings.Contains(rec.queries[1], "#@") {
		t.Fatalf("retry query still has the field or markers:\n%s", rec.queries[1])
	}
	if !strings.Contains(rec.queries[1], "a b") {
		t.Fatal("other optional segments must stay")
	}
	if err := c.DoOpt(context.Background(), optQuery, nil, &out); err != nil {
		t.Fatal(err)
	}
	if rec.count() != 3 || strings.Contains(rec.queries[2], "diffStatsSummary") {
		t.Fatalf("second call must go straight without the field, requests = %d", rec.count())
	}
}

func TestOptionalOtherErrorsPass(t *testing.T) {
	c, rec := newRecorder(t, func(int, string, map[string]any) string {
		return `{"errors":[{"message":"Field 'nope' doesn't exist on type 'Project'"}]}`
	})
	if err := c.DoOpt(context.Background(), optQuery, nil, nil); err == nil {
		t.Fatal("unknown required field must fail")
	}
	if rec.count() != 1 {
		t.Fatalf("requests = %d", rec.count())
	}
}
