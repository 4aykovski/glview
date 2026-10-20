package config

import (
	"context"
	"errors"
	"testing"
)

func env(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

type fakeGlab struct {
	token string
	err   error
	asked string
}

func (f *fakeGlab) Token(_ context.Context, host string) (string, error) {
	f.asked = host
	return f.token, f.err
}

func TestResolveHostPriority(t *testing.T) {
	cases := []struct {
		name    string
		cfg     Config
		in      HostInput
		env     map[string]string
		want    string
		wantSrc Source
	}{
		{"flag wins", Config{Host: "a.com"}, HostInput{FlagURL: "https://f.com", RemoteHost: "r.com"}, map[string]string{"GITLAB_HOST": "e.com"}, "https://f.com", SourceFlag},
		{"config", Config{Host: "https://a.com/"}, HostInput{RemoteHost: "r.com"}, map[string]string{"GITLAB_HOST": "e.com"}, "https://a.com", SourceConfig},
		{"env", Config{}, HostInput{RemoteHost: "r.com"}, map[string]string{"GITLAB_HOST": "e.com"}, "https://e.com", SourceEnv},
		{"remote", Config{}, HostInput{RemoteHost: "gitlab.example.com"}, nil, "https://gitlab.example.com", SourceRemote},
		{"none", Config{}, HostInput{}, nil, "", SourceNone},
		{"path prefix kept", Config{Host: "https://Example.com/gitlab/"}, HostInput{}, nil, "https://example.com/gitlab", SourceConfig},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, src, err := ResolveHost(c.cfg, c.in, env(c.env))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want || src != c.wantSrc {
				t.Fatalf("got %q/%s, want %q/%s", got, src, c.want, c.wantSrc)
			}
		})
	}
}

func TestResolveHostRejectsBadScheme(t *testing.T) {
	if _, _, err := ResolveHost(Config{Host: "ftp://x"}, HostInput{}, env(nil)); err == nil {
		t.Fatal("expected error")
	}
}

func TestResolveTokenPriority(t *testing.T) {
	ctx := context.Background()
	host := "https://gitlab.example.com"

	g := &fakeGlab{token: "glab-token"}
	tok, src := ResolveToken(ctx, Config{Host: host, Token: "cfg-token"}, host, env(map[string]string{"GITLAB_TOKEN": "env-token"}), g)
	if tok != "cfg-token" || src != SourceConfig {
		t.Fatalf("config: %q %s", tok, src)
	}

	tok, src = ResolveToken(ctx, Config{}, host, env(map[string]string{"GITLAB_TOKEN": "env-token"}), g)
	if tok != "env-token" || src != SourceEnv {
		t.Fatalf("env: %q %s", tok, src)
	}

	tok, src = ResolveToken(ctx, Config{}, host, env(nil), g)
	if tok != "glab-token" || src != SourceGlab {
		t.Fatalf("glab: %q %s", tok, src)
	}
	if g.asked != "gitlab.example.com" {
		t.Fatalf("glab asked for %q, want bare hostname", g.asked)
	}

	tok, src = ResolveToken(ctx, Config{}, host, env(nil), nil)
	if tok != "" || src != SourceNone {
		t.Fatalf("none: %q %s", tok, src)
	}
}

func TestConfigTokenIgnoredForOtherHost(t *testing.T) {
	tok, src := ResolveToken(context.Background(),
		Config{Host: "https://a.com", Token: "a-token"}, "https://b.com", env(nil), nil)
	if tok != "" || src != SourceNone {
		t.Fatalf("token for a.com must not be used for b.com: %q %s", tok, src)
	}
}

func TestGlabErrorFallsThrough(t *testing.T) {
	tok, src := ResolveToken(context.Background(), Config{}, "https://x.com", env(nil), &fakeGlab{err: errors.New("boom")})
	if tok != "" || src != SourceNone {
		t.Fatalf("got %q %s", tok, src)
	}
}
