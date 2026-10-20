package startup

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/4aykovski/glview/internal/config"
)

type glabStub struct{ token string }

func (g glabStub) Token(context.Context, string) (string, error) { return g.token, nil }

func mkRepo(t *testing.T, dir, remote string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[core]\n"
	if remote != "" {
		cfg += "[remote \"origin\"]\n\turl = " + remote + "\n"
	}
	os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(cfg), 0o644)
	os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main\n"), 0o644)
}

func noEnv(string) string { return "" }

func TestProjectModeFromRepo(t *testing.T) {
	dir := t.TempDir()
	mkRepo(t, dir, "git@gitlab.example.com:platform/route-service.git")
	p, err := Resolve(context.Background(), Inputs{Cwd: dir, Getenv: noEnv, Glab: glabStub{"glab-tok"}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeProject || p.HostURL != "https://gitlab.example.com" || p.HostSource != config.SourceRemote {
		t.Fatalf("plan = %+v", p)
	}
	if p.Token != "glab-tok" || p.TokenSource != config.SourceGlab {
		t.Fatalf("token from glab expected: %q %s", p.Token, p.TokenSource)
	}
	tg, err := p.Target(p.HostURL, nil)
	if err != nil || tg.ProjectPath != "platform/route-service" {
		t.Fatalf("target = %+v %v", tg, err)
	}
}

func TestForeignRemote(t *testing.T) {
	dir := t.TempDir()
	mkRepo(t, dir, "git@github.com:me/dotfiles.git")
	p, err := Resolve(context.Background(), Inputs{
		Config: config.Config{Host: "https://gitlab.example.com", Token: "t"},
		Cwd:    dir, Getenv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.HostURL != "https://gitlab.example.com" {
		t.Fatalf("config host must win over remote: %q", p.HostURL)
	}
	tg, _ := p.Target(p.HostURL, nil)
	if tg.Reason != TargetForeign || tg.ProjectPath != "" {
		t.Fatalf("target = %+v", tg)
	}
	if p.ParentDir() != filepath.Dir(dir) {
		t.Fatalf("parent = %q", p.ParentDir())
	}
}

func TestGitHubRemoteNotUsedAsHost(t *testing.T) {
	dir := t.TempDir()
	mkRepo(t, dir, "git@github.com:4aykovski/glview.git")
	p, _ := Resolve(context.Background(), Inputs{Cwd: dir, Getenv: noEnv})
	if p.HostURL != "" {
		t.Fatalf("github.com must not become the GitLab host: %q", p.HostURL)
	}
}

func TestNoRemote(t *testing.T) {
	dir := t.TempDir()
	mkRepo(t, dir, "")
	p, _ := Resolve(context.Background(), Inputs{Config: config.Config{Host: "https://h.com"}, Cwd: dir, Getenv: noEnv})
	tg, _ := p.Target(p.HostURL, nil)
	if tg.Reason != TargetNoRemote {
		t.Fatalf("target = %+v", tg)
	}
}

func TestWorkspaceMode(t *testing.T) {
	dir := t.TempDir()
	p, err := Resolve(context.Background(), Inputs{
		Cwd: dir,
		Getenv: func(k string) string {
			return map[string]string{"GITLAB_HOST": "gitlab.example.com", "GITLAB_TOKEN": "env-tok"}[k]
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeWorkspace || p.WorkspaceDir != dir || p.HostSource != config.SourceEnv || p.TokenSource != config.SourceEnv {
		t.Fatalf("plan = %+v", p)
	}
}

func TestNothingKnownGoesToSetup(t *testing.T) {
	p, err := Resolve(context.Background(), Inputs{Cwd: t.TempDir(), Getenv: noEnv})
	if err != nil {
		t.Fatal(err)
	}
	if p.HostURL != "" || p.Token != "" {
		t.Fatalf("plan = %+v", p)
	}
}

func TestProjectFlag(t *testing.T) {
	cases := []struct {
		flag, cfgHost, host, path string
		wantErr                   bool
	}{
		{"platform/route-service", "", "", "platform/route-service", false},
		{"group/sub/proj/", "", "", "group/sub/proj", false},
		{"https://gitlab.example.com/group/proj", "", "https://gitlab.example.com", "group/proj", false},
		{"https://gitlab.example.com/group/proj/-/merge_requests/479", "", "https://gitlab.example.com", "group/proj", false},
		{"https://example.com/gitlab/group/proj", "https://example.com/gitlab", "https://example.com/gitlab", "group/proj", false},
		{"proj", "", "", "", true},
		{"https://gitlab.example.com/", "", "", "", true},
	}
	for _, c := range cases {
		host, path, err := ParseProjectFlag(c.flag, c.cfgHost)
		if (err != nil) != c.wantErr {
			t.Errorf("%q: err = %v", c.flag, err)
			continue
		}
		if !c.wantErr && (host != c.host || path != c.path) {
			t.Errorf("%q: got %q %q, want %q %q", c.flag, host, path, c.host, c.path)
		}
	}
}

func TestProjectFlagMode(t *testing.T) {
	p, err := Resolve(context.Background(), Inputs{
		ProjectFlag: "https://gitlab.example.com/team/app",
		Cwd:         t.TempDir(), Getenv: noEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeProject || p.HostSource != config.SourceFlag || p.ProjectPath != "team/app" || p.Repo != nil {
		t.Fatalf("plan = %+v", p)
	}
}
