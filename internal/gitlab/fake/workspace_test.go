package fake

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestWorkspaceBatchesFollowDelays(t *testing.T) {
	c := New()
	c.SetBatchSize(2)
	var ps []gitlab.ProjectRef
	for i := range 4 {
		path := fmt.Sprintf("g/p%d", i)
		ps = append(ps, gitlab.ProjectRef{FullPath: path})
		if i != 3 {
			c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{ID: int64(i), FullPath: path}})
		}
	}
	c.SetProjectDelay("g/p0", 60*time.Millisecond) // first batch arrives last
	var order []string
	var groups []gitlab.ProjectGroup[gitlab.MergeRequest]
	for res := range c.Workspace().MergeRequests(context.Background(), ps, gitlab.MRFilter{State: gitlab.MRStateOpened}, 3) {
		order = append(order, res.Groups[0].Project.FullPath)
		groups = append(groups, res.Groups...)
	}
	if len(order) != 2 || order[0] != "g/p2" || order[1] != "g/p0" {
		t.Fatalf("batch order = %v", order)
	}
	for _, g := range groups {
		if g.Project.FullPath == "g/p3" && !errors.Is(g.Err, gitlab.ErrNotFound) {
			t.Fatalf("unknown project: %+v", g)
		}
	}
	if c.CallCount("Workspace.MergeRequests") != 1 {
		t.Fatal("one call per batch request")
	}
}

func TestStarredGroupsSearch(t *testing.T) {
	c := New()
	c.AddProject(gitlab.Project{ProjectRef: gitlab.ProjectRef{FullPath: "platform/route-service"}})
	c.SetStarred("platform/route-service")
	ps, _ := c.Projects().Starred(context.Background())
	if len(ps) != 1 || !ps[0].Starred {
		t.Fatalf("starred = %+v", ps)
	}
	var many []string
	for i := range 342 {
		many = append(many, fmt.Sprintf("platform/p%d", i))
	}
	c.AddGroup(gitlab.Group{ID: 9, FullPath: "platform"}, many...)
	got, total, _ := c.Projects().ListGroupProjects(context.Background(), 9, 100)
	if len(got) != 100 || total != 342 {
		t.Fatalf("group = %d of %d", len(got), total)
	}
	sp, sg, _ := c.Search().Search(context.Background(), "ROUTE")
	if len(sp) != 1 || len(sg) != 0 {
		t.Fatalf("search = %+v %+v", sp, sg)
	}
}
