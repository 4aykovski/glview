package uitest

import (
	"strings"
	"testing"
)

func TestFitError(t *testing.T) {
	ok := strings.Repeat(strings.Repeat("x", 80)+"\n", 24)
	if got := FitError(ok, 80, 24); got != "" {
		t.Fatalf("fits: %s", got)
	}
	if got := FitError(ok+"x\n", 80, 24); !strings.Contains(got, "25 lines") {
		t.Fatalf("too tall: %q", got)
	}
	if got := FitError("xxx"+strings.Repeat("x", 78), 80, 24); !strings.Contains(got, "81 cells") {
		t.Fatalf("too wide: %q", got)
	}
	if got := FitError("┌"+strings.Repeat("─", 80)+"┐", 120, 40); got != "" {
		t.Fatalf("wide terminal: %q", got)
	}
	if got := FitError(strings.Repeat("x", 100), 60, 20); got != "" {
		t.Fatalf("a terminal below the minimum is not checked: %q", got)
	}
}
