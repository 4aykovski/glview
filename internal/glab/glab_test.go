package glab

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeGlab writes a shell script that mimics glab for the given scenario.
func fakeGlab(t *testing.T, script string) *Glab {
	t.Helper()
	path := filepath.Join(t.TempDir(), "glab")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	g := Detect(path, false)
	if g == nil {
		t.Fatal("fake glab not detected")
	}
	// The first exec of a fresh script can be slow on macOS (binary scanning)
	// when many test packages run in parallel; timeouts are tested separately.
	g.Timeout = 10 * time.Second
	return g
}

func TestTokenFromConfig(t *testing.T) {
	g := fakeGlab(t, `
if [ "$1 $2 $3 $4 $5" = "config get token --host gitlab.example.com" ]; then
  echo "glpat-from-config"
  exit 0
fi
exit 1
`)
	tok, err := g.Token(context.Background(), "gitlab.example.com")
	if err != nil || tok != "glpat-from-config" {
		t.Fatalf("got %q, %v", tok, err)
	}
}

func TestTokenFallsBackToAuthStatusJSON(t *testing.T) {
	g := fakeGlab(t, `
case "$1 $2" in
  "config get") echo ""; exit 0 ;;
  "auth status")
    echo '{"hosts":[{"host":"gitlab.example.com","user":"egor","token":"glpat-from-keyring"}]}'
    exit 0 ;;
esac
exit 1
`)
	tok, err := g.Token(context.Background(), "gitlab.example.com")
	if err != nil || tok != "glpat-from-keyring" {
		t.Fatalf("got %q, %v", tok, err)
	}
}

func TestTokenNotLoggedIn(t *testing.T) {
	g := fakeGlab(t, `exit 1`)
	tok, err := g.Token(context.Background(), "gitlab.example.com")
	if err == nil || tok != "" {
		t.Fatalf("got %q, %v", tok, err)
	}
}

func TestTokenTimeout(t *testing.T) {
	g := fakeGlab(t, `sleep 5`)
	g.Timeout = 200 * time.Millisecond
	start := time.Now()
	_, err := g.Token(context.Background(), "gitlab.example.com")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("want deadline error, got %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("timeout not enforced: %v", time.Since(start))
	}
}

func TestLoggedIn(t *testing.T) {
	ok := fakeGlab(t, `[ "$1 $2" = "auth status" ] && exit 0; exit 1`)
	if !ok.LoggedIn(context.Background(), "h") {
		t.Fatal("expected logged in")
	}
	no := fakeGlab(t, `exit 1`)
	if no.LoggedIn(context.Background(), "h") {
		t.Fatal("expected not logged in")
	}
}

func TestDetect(t *testing.T) {
	if Detect("/definitely/not/here/glab", false) != nil {
		t.Fatal("missing binary must give nil")
	}
	g := fakeGlab(t, "exit 0")
	if Detect(g.Path, true) != nil {
		t.Fatal("disabled must give nil")
	}
}

func TestFindToken(t *testing.T) {
	cases := map[string]string{
		`{"token":"a1"}`:                       "a1",
		`{"x":{"Token":" b2 "}}`:               "b2",
		`[{"token":""},{"token":"c3"}]`:        "c3",
		`not json`:                             "",
		`{"hosts":{"h":{"api_token":"nope"}}}`: "",
	}
	for in, want := range cases {
		if got := findToken([]byte(in)); got != want {
			t.Errorf("findToken(%s) = %q, want %q", in, got, want)
		}
	}
}
