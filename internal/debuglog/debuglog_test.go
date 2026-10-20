package debuglog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactRegisteredSecret(t *testing.T) {
	RegisterSecret("supersecretvalue123")
	got := Redact("Authorization: Bearer supersecretvalue123")
	if strings.Contains(got, "supersecretvalue123") {
		t.Fatalf("secret leaked: %q", got)
	}
}

func TestRedactTokenPattern(t *testing.T) {
	got := Redact("token=glpat-AbCdEf1234567890xyz and more")
	if strings.Contains(got, "glpat-") {
		t.Fatalf("token leaked: %q", got)
	}
	if !strings.Contains(got, "and more") {
		t.Fatalf("unrelated text lost: %q", got)
	}
}

func TestShortSecretIgnored(t *testing.T) {
	RegisterSecret("abc")
	if got := Redact("abc"); got != "abc" {
		t.Fatalf("short value must not be masked, got %q", got)
	}
}

func TestOpenWritesRedacted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "debug.log")
	c, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	RegisterSecret("another-secret-token")
	L().Info("request", "header", "PRIVATE-TOKEN another-secret-token")
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "another-secret-token") {
		t.Fatalf("secret leaked to file: %s", data)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("log perms = %o, want 600", perm)
	}
}
