package project

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
	"github.com/4aykovski/glview/internal/gitlab/fake"
)

func lastListFilter(calls []fake.Call) gitlab.MRFilter {
	var f gitlab.MRFilter
	for _, c := range calls {
		if c.Method == "MergeRequests.List" {
			f = c.Args[1].(gitlab.MRFilter)
		}
	}
	return f
}

func TestSCyclesStateAndRequeries(t *testing.T) {
	client := newFake(2)
	d, _, scr := open(t, client, nil)
	mrs := scr.tabs[0].(*mrsTab)
	d.RequireView("s state")
	d.Key("?")
	d.RequireView("⌫", "reset filters", "state")
	d.Key("esc")
	for _, want := range []gitlab.MRState{gitlab.MRStateMerged, gitlab.MRStateClosed, gitlab.MRStateAll, gitlab.MRStateOpened} {
		n := client.CallCount("MergeRequests.List")
		d.Key("s")
		if client.CallCount("MergeRequests.List") != n+1 || lastListFilter(client.Calls()).State != want || mrs.filter.State != want {
			t.Fatalf("state %q: calls=%d filter=%+v", want, client.CallCount("MergeRequests.List"), lastListFilter(client.Calls()))
		}
		d.RequireView("state:" + string(want))
	}
}

func TestBackspaceResetsMRFilters(t *testing.T) {
	client := newFake(2)
	d, _, scr := open(t, client, nil)
	mrs := scr.tabs[0].(*mrsTab)
	d.Key("/")
	typeAll(d, "state:closed reviewer:@me labels:bug fix")
	d.Key("enter", "m")
	if !mrs.filter.Mine || mrs.filter.State != gitlab.MRStateClosed {
		t.Fatalf("setup: %+v", mrs.filter)
	}
	d.Key("backspace")
	f := lastListFilter(client.Calls())
	if f.State != gitlab.MRStateOpened || f.Scope != gitlab.MRScopeAll || f.Reviewer != "" || f.Mine || len(f.Labels) != 0 || f.Search != "" || f.Draft != nil {
		t.Fatalf("filter after reset: %+v", f)
	}
	d.RequireView("state:opened", "scope:all", "reviewer:any", "draft:any")
}

func TestSCyclesPipelineStatus(t *testing.T) {
	client := newFake(1)
	addPipelines(client)
	d, _, tab := openPipelines(t, client)
	d.Key("s", "s")
	if tab.filter.Status != "failed" {
		t.Fatalf("status = %q", tab.filter.Status)
	}
	d.RequireView("status:failed", "#101")
	d.RequireNoView("#102")
	var last gitlab.PipelineFilter
	for _, c := range client.Calls() {
		if c.Method == "Pipelines.List" {
			last = c.Args[1].(gitlab.PipelineFilter)
		}
	}
	if last.Status != "failed" {
		t.Fatalf("request filter = %+v", last)
	}
	d.Key("s", "s", "s") // success, manual, any
	if tab.filter.Status != "" {
		t.Fatalf("cycle must return to any: %q", tab.filter.Status)
	}
	d.Key("m", "s")
	d.Key("backspace")
	if (tab.filter != gitlab.PipelineFilter{}) {
		t.Fatalf("reset: %+v", tab.filter)
	}
}

func TestPipelinesBrowserKeys(t *testing.T) {
	client := newFake(1)
	addPipelines(client)
	client.UpdatePipeline(path, 102, func(p *gitlab.Pipeline) {
		p.WebURL = "https://gitlab.example.com/platform/route-service/-/pipelines/102"
	})
	d, _, tab := openPipelines(t, client)
	var opened []string
	tab.env().Browser = func(u string) error { opened = append(opened, u); return nil }

	d.Key("o")
	if len(opened) != 1 || opened[0] != "https://gitlab.example.com/platform/route-service/-/pipelines" {
		t.Fatalf("no filters: %v", opened)
	}
	d.Key("/")
	typeAll(d, "ref:main status:failed")
	d.Key("enter", "o")
	if opened[1] != "https://gitlab.example.com/platform/route-service/-/pipelines?ref=main&status=failed" {
		t.Fatalf("filtered: %q", opened[1])
	}
	d.Key("backspace", "m", "o")
	if opened[2] != "https://gitlab.example.com/platform/route-service/-/pipelines?username=egor" {
		t.Fatalf("@me expands to the login: %q", opened[2])
	}
	d.Key("backspace")
	sel := tab.selected()
	d.Key("O")
	if len(opened) != 4 || opened[3] != sel.WebURL {
		t.Fatalf("O opens the selected pipeline: %v vs %q", opened, sel.WebURL)
	}
	d.Key("?")
	d.RequireView("list in browser", "pipeline in browser")
}
