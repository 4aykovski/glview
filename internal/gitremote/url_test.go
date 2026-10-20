package gitremote

import (
	"strings"
	"testing"
)

func TestParseURL(t *testing.T) {
	cases := []struct {
		in, host, path string
	}{
		{"git@gitlab.example.com:platform/route-service.git", "gitlab.example.com", "platform/route-service"},
		{"git@gitlab.example.com:group/sub/proj", "gitlab.example.com", "group/sub/proj"},
		{"gitlab-work:team/app.git", "gitlab-work", "team/app"},
		{"ssh://git@gitlab.example.com:2222/team/app.git", "gitlab.example.com", "team/app"},
		{"ssh://gitlab.example.com/team/app", "gitlab.example.com", "team/app"},
		{"https://gitlab.example.com/group/sub/proj.git", "gitlab.example.com", "group/sub/proj"},
		{"https://GitLab.Example.com:8443/group/proj", "gitlab.example.com", "group/proj"},
		{"https://oauth2:token@gitlab.com/a/b.git", "gitlab.com", "a/b"},
		{"http://example.com/gitlab/group/proj.git/", "example.com", "gitlab/group/proj"},
	}
	for _, c := range cases {
		r, err := ParseURL(c.in)
		if err != nil {
			t.Errorf("%s: %v", c.in, err)
			continue
		}
		if r.Host != c.host || r.Path != c.path {
			t.Errorf("%s: got %q %q, want %q %q", c.in, r.Host, r.Path, c.host, c.path)
		}
	}
}

func TestParseURLErrors(t *testing.T) {
	for _, in := range []string{"", "/local/path/repo", "./rel/repo", "file:///tmp/repo", "https:///nohost"} {
		if _, err := ParseURL(in); err == nil {
			t.Errorf("%q: expected error", in)
		}
	}
}

func TestInstanceProjectPath(t *testing.T) {
	inst, err := NewInstance("https://gitlab.example.com", []string{"gitlab-work"})
	if err != nil {
		t.Fatal(err)
	}
	prefixed, _ := NewInstance("https://example.com/gitlab", nil)

	cases := []struct {
		name   string
		inst   Instance
		remote string
		want   string
		ok     bool
	}{
		{"scp", inst, "git@gitlab.example.com:platform/route-service.git", "platform/route-service", true},
		{"ssh port ignored", inst, "ssh://git@gitlab.example.com:2222/team/app.git", "team/app", true},
		{"alias", inst, "git@gitlab-work:team/app.git", "team/app", true},
		{"foreign", inst, "git@github.com:me/dotfiles.git", "", false},
		{"relative root http", prefixed, "https://example.com/gitlab/group/sub/proj.git", "group/sub/proj", true},
		{"relative root ssh", prefixed, "git@example.com:group/proj.git", "group/proj", true},
		{"no namespace", inst, "git@gitlab.example.com:proj.git", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := ParseURL(c.remote)
			if err != nil {
				t.Fatal(err)
			}
			got, ok := c.inst.ProjectPath(r)
			if got != c.want || ok != c.ok {
				t.Fatalf("got %q %v, want %q %v", got, ok, c.want, c.ok)
			}
		})
	}
}

func TestGitConfigParsing(t *testing.T) {
	cfg := parseGitConfig(strings.NewReader(`
# comment
[Remote "origin"]
	URL = "git@h:a/b.git" ; trailing comment
	url = git@h:c/d.git
[branch "feat/x"]
	remote = origin
[core]
	bare
`))
	if got := cfg.get("remote.origin", "url"); got != "git@h:c/d.git" {
		t.Fatalf("last value must win, got %q", got)
	}
	if got := cfg.get("branch.feat/x", "remote"); got != "origin" {
		t.Fatalf("subsection with slash: %q", got)
	}
	if got := cfg.get("core", "bare"); got != "true" {
		t.Fatalf("bare key = %q", got)
	}
	if v := parseValue(`"a # not comment" # comment`); v != "a # not comment" {
		t.Fatalf("quoted comment char: %q", v)
	}
}
