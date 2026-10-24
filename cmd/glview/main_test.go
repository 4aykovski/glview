package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if code := run([]string{"--version"}, &out, &errb); code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, errb.String())
	}
	if !strings.HasPrefix(out.String(), "glview ") {
		t.Fatalf("unexpected output %q", out.String())
	}
}

func TestProjectFlagAliases(t *testing.T) {
	for _, args := range [][]string{{"-p", "g/p"}, {"--project", "g/p"}} {
		o, err := parseFlags(args, &bytes.Buffer{})
		if err != nil {
			t.Fatal(err)
		}
		if o.project != "g/p" {
			t.Fatalf("%v: project = %q", args, o.project)
		}
	}
}

func TestUnexpectedArgs(t *testing.T) {
	var errb bytes.Buffer
	if code := run([]string{"extra"}, &bytes.Buffer{}, &errb); code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
}
