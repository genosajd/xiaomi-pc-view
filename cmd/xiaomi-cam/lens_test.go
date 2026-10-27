package main

import (
	"fmt"
	"testing"
)

// The zoom steps of a dual-lens camera stop at the telephoto lens on the way up and down,
// and go back to the usual steps after it; a one-lens camera keeps the steps of 0.5.
func TestNextZoomStopsAtTheTelephotoLens(t *testing.T) {
	walk := func(from, delta, tele float64, n int) string {
		s, z := "", from
		for i := 0; i < n; i++ {
			z = nextZoom(z, delta, tele)
			s += fmt.Sprintf(" %.1f", z)
		}
		return s
	}
	cases := []struct {
		from, delta, tele float64
		n                 int
		want              string
	}{
		{1, zoomStep, 2.8, 6, " 1.5 2.0 2.5 2.8 3.0 3.5"},
		{4, -zoomStep, 2.8, 6, " 3.5 3.0 2.8 2.5 2.0 1.5"},
		{1, zoomStep, 0, 5, " 1.5 2.0 2.5 3.0 3.5"},
		{8.5, zoomStep, 2.8, 2, " 9.0 9.0"}, // the camera's maximum
		{1.5, -zoomStep, 2.8, 2, " 1.0 1.0"},
		{9, -maxZoom, 2.8, 1, " 1.0"}, // a jump back doesn't stop at the lens
	}
	for _, c := range cases {
		if got := walk(c.from, c.delta, c.tele, c.n); got != c.want {
			t.Errorf("from %.1f by %.1f (tele %.1f):%s, want%s", c.from, c.delta, c.tele, got, c.want)
		}
	}
}

func TestSnapTele(t *testing.T) {
	for _, c := range []struct{ z, tele, want float64 }{
		{3.0, 2.8, 2.8}, {2.5, 2.8, 2.5}, {2.8, 2.8, 2.8}, {3.5, 2.8, 3.5}, {3.0, 0, 3.0},
	} {
		if got := snapTele(c.z, c.tele); got != c.want {
			t.Errorf("snapTele(%.1f, %.1f) = %.1f, want %.1f", c.z, c.tele, got, c.want)
		}
	}
	if !onTele(2.8, 2.8) || !onTele(4, 2.8) || onTele(2.7, 2.8) || onTele(5, 0) {
		t.Error("onTele")
	}
}
