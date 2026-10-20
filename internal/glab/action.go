package glab

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"text/template"

	"github.com/4aykovski/glview/internal/gitlab"
)

// Context is the kind of object an action works on.
type Context string

const (
	ContextMR       Context = "mr"
	ContextPipeline Context = "pipeline"
	ContextJob      Context = "job"
	ContextTag      Context = "tag"
	ContextIssue    Context = "issue"
	ContextProject  Context = "project"
)

// RefreshKind tells what to re-read after the program exits.
type RefreshKind string

const (
	RefreshBranch RefreshKind = "branch"
	RefreshMR     RefreshKind = "mr"
)

// Action is an external command bound to a key. Built-in actions and the
// future custom commands share this shape (SPEC §10.3).
type Action struct {
	ID        string // "mr.checkout"
	Key       string // "C"
	Context   Context
	Command   string // text/template: "glab mr checkout {{.MR.IID}}"
	NeedsRepo bool   // runs in the project's local folder
	NeedsGlab bool
	Refresh   []RefreshKind
}

// TemplateData is what command templates can reference.
type TemplateData struct {
	Host     string
	Project  gitlab.Project
	MR       *gitlab.MergeRequest
	Pipeline *gitlab.PipelineSummary
	Job      *gitlab.Job
	Tag      *gitlab.Tag
	Issue    *gitlab.Issue
}

// BuiltinActions returns the actions shipped with the program.
func BuiltinActions() []Action {
	return []Action{{
		ID: "mr.checkout", Key: "C", Context: ContextMR,
		Command:   "glab mr checkout {{.MR.IID}}",
		NeedsRepo: true, NeedsGlab: true,
		Refresh: []RefreshKind{RefreshBranch, RefreshMR},
	}}
}

// Find returns the built-in action with the given ID.
func Find(id string) (Action, bool) {
	for _, a := range BuiltinActions() {
		if a.ID == id {
			return a, true
		}
	}
	return Action{}, false
}

// Cmd renders the command template and builds the process. The leading
// "glab" word is replaced by the discovered binary; repoDir becomes the
// working directory.
func (a Action) Cmd(g *Glab, d TemplateData, repoDir string) (*exec.Cmd, error) {
	tpl, err := template.New(a.ID).Option("missingkey=error").Parse(a.Command)
	if err != nil {
		return nil, fmt.Errorf("action %s: %w", a.ID, err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, d); err != nil {
		return nil, fmt.Errorf("action %s: %w", a.ID, err)
	}
	fields := strings.Fields(buf.String())
	if len(fields) == 0 {
		return nil, fmt.Errorf("action %s: empty command", a.ID)
	}
	name := fields[0]
	if name == "glab" && g != nil {
		name = g.Path
	}
	cmd := exec.Command(name, fields[1:]...)
	if a.NeedsRepo {
		cmd.Dir = repoDir
	}
	return cmd, nil
}
