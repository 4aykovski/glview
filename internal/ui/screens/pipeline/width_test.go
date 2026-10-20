package pipeline

import "testing"

func TestStageWidth(t *testing.T) {
	cases := []struct{ w, n, want int }{
		{160, 2, 80},
		{120, 5, 25}, // 4 stages + downstream do not fit: fixed width, scrolling
		{120, 4, 30},
		{100, 4, 25},
		{80, 1, 80},
		{0, 3, 25},
		{120, 0, 25},
	}
	for _, c := range cases {
		if got := stageWidth(c.w, c.n); got != c.want {
			t.Errorf("stageWidth(%d, %d) = %d, want %d", c.w, c.n, got, c.want)
		}
	}
}

func TestColWidthRemainderGoesToLast(t *testing.T) {
	const w, n = 122, 4 // 30 each, remainder 2
	sum := 0
	for i := range n {
		sum += colWidth(w, n, i)
	}
	if sum != w || colWidth(w, n, n-1) != 32 || colWidth(w, n, 0) != 30 {
		t.Fatalf("widths sum %d, last %d", sum, colWidth(w, n, n-1))
	}
	// Scrolling columns keep a fixed width, the last one included.
	if colWidth(120, 5, 4) != 25 {
		t.Fatalf("scrolled last column = %d", colWidth(120, 5, 4))
	}
}
