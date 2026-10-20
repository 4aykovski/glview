package joblog

import (
	"testing"

	"github.com/4aykovski/glview/internal/gitlab"
)

func TestGoldenLog(t *testing.T) {
	d, _, _, _ := setup(t, gitlab.StatusFailed, sample)
	d.RequireGoldenSizes("joblog")
}

func TestGoldenLogSearch(t *testing.T) {
	d, _, _, _ := setup(t, gitlab.StatusFailed, sample)
	d.Key("/")
	typeLetters(d, "timeout")
	d.Key("enter")
	d.Key("/")
	d.RequireGoldenSizes("joblog-search")
}

func typeLetters(d interface{ Key(...string) }, s string) {
	for _, r := range s {
		d.Key(string(r))
	}
}
