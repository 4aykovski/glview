package gitremote

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanClassifies(t *testing.T) {
	ws := t.TempDir()
	mkRepo(t, filepath.Join(ws, "route-service"), originCfg, "ref: refs/heads/fix/slot-race")
	mkRepo(t, filepath.Join(ws, "dotfiles"), "[remote \"origin\"]\n\turl = git@github.com:egor/dotfiles.git\n", "ref: refs/heads/main")
	mkRepo(t, filepath.Join(ws, "scratch"), "[core]\n\tbare = false\n", "ref: refs/heads/main")
	if err := os.MkdirAll(filepath.Join(ws, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	mkRepo(t, filepath.Join(ws, ".cache"), originCfg, "ref: refs/heads/main")
	write(t, filepath.Join(ws, "README.md"), "file, not a folder")

	// Symlink to a repository outside the workspace is followed.
	outside := t.TempDir()
	mkRepo(t, filepath.Join(outside, "linked"), "[remote \"origin\"]\n\turl = https://gitlab.example.com/infra/router-configs.git\n", "ref: refs/heads/main")
	if err := os.Symlink(filepath.Join(outside, "linked"), filepath.Join(ws, "router-configs")); err != nil {
		t.Fatal(err)
	}

	inst, _ := NewInstance("https://gitlab.example.com", nil)
	got, err := Scan(ws, inst)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]Kind{
		"dotfiles":       KindForeign,
		"notes":          KindNoGit,
		"route-service":  KindGitLab,
		"router-configs": KindGitLab,
		"scratch":        KindNoRemote,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries: %+v", len(got), got)
	}
	for i, p := range got {
		if i > 0 && got[i-1].Name > p.Name {
			t.Fatalf("not sorted: %q before %q", got[i-1].Name, p.Name)
		}
		k, ok := want[p.Name]
		if !ok {
			t.Fatalf("unexpected entry %q", p.Name)
		}
		if p.Kind != k {
			t.Fatalf("%s: kind %v, want %v", p.Name, p.Kind, k)
		}
	}
	for _, p := range got {
		if p.Name == "route-service" && (p.ProjectPath != "platform/route-service" || p.Repo.Branch != "fix/slot-race") {
			t.Fatalf("route-service: %+v", p)
		}
	}
}
