package glab

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestCheckoutCmd(t *testing.T) {
	a, ok := Find("mr.checkout")
	if !ok || a.Key != "C" || !a.NeedsRepo || !a.NeedsGlab {
		t.Fatalf("builtin = %+v", a)
	}
	g := &Glab{Path: "/usr/local/bin/glab"}
	cmd, err := a.Cmd(g, TemplateData{MR: &gitlab.MergeRequest{IID: 479}}, "/src/route-service")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Path != "/usr/local/bin/glab" || len(cmd.Args) != 4 || cmd.Args[1] != "mr" || cmd.Args[2] != "checkout" || cmd.Args[3] != "479" {
		t.Fatalf("cmd = %v", cmd.Args)
	}
	if cmd.Dir != "/src/route-service" {
		t.Fatalf("dir = %q", cmd.Dir)
	}
}

func TestCmdErrors(t *testing.T) {
	a, _ := Find("mr.checkout")
	if _, err := a.Cmd(&Glab{Path: "glab"}, TemplateData{}, "/x"); err == nil {
		t.Fatal("a template that needs the MR must fail without one")
	}
	if _, err := (Action{ID: "x", Command: "{{.Nope"}).Cmd(nil, TemplateData{}, ""); err == nil {
		t.Fatal("broken template")
	}
	if _, err := (Action{ID: "x", Command: "  "}).Cmd(nil, TemplateData{}, ""); err == nil {
		t.Fatal("empty command")
	}
}
