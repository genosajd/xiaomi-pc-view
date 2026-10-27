package xccore

import (
	"math"
	"testing"
)

// Aiming at a point and then finding the new picture's centre on the old wide picture
// lands on the point (within one motor unit), on both lenses and with the picture turned.
func TestAimLandsOnThePoint(t *testing.T) {
	m := aimModels["isa.camera.700sa"]
	cur := Position{X: 778, Y: 30, Scale: 10}
	unitX, unitY := m.PxX/aimW, math.Abs(m.PxY)/aimH // one motor unit as a fraction of the picture
	for _, turn := range []int{0, 180} {
		s := 1.0
		if turn == 180 {
			s = -1
		}
		for _, c := range []struct{ fx, fy, zf float64 }{
			{0.2, -0.1, 2.8}, {-0.4, 0.3, 2.8}, {0.1, 0.05, 1}, {0, 0, 2.8}, {-0.25, -0.2, 1},
		} {
			p := aimTarget(m, cur, 1, 2.8, c.zf, turn, c.fx, c.fy)
			gx, gy := widePoint(m, cur, p, c.zf, 2.8, s)
			if math.Abs(gx-c.fx) > unitX || math.Abs(gy-c.fy) > unitY {
				t.Errorf("turn %d, point %.2f,%.2f, zoom %.1f: went to %+v, its centre is at %.3f,%.3f",
					turn, c.fx, c.fy, c.zf, p, gx, gy)
			}
			if p.Scale != int(c.zf*10+0.5) {
				t.Errorf("scale %d for zoom %.1f", p.Scale, c.zf)
			}
		}
	}
}

// The centre of the picture is already there: no turn, except the telephoto lens's own
// offset when switching to it.
func TestAimAtTheCentre(t *testing.T) {
	m := aimModels["isa.camera.700sa"]
	cur := Position{X: 500, Y: 40, Scale: 10}
	if p := aimTarget(m, cur, 1, 2.8, 1, 0, 0, 0); p.X != 500 || p.Y != 40 {
		t.Errorf("wide to wide at the centre moved to %+v", p)
	}
	// the telephoto lens's centre lies 22 px right / 12 px down of the wide one: the camera
	// turns that little (22/8.4 = 3 units, 12/21.8 = 1 unit)
	p := aimTarget(m, cur, 1, 2.8, 2.8, 0, 0, 0)
	if p.X != 503 || p.Y != 39 {
		t.Errorf("wide centre to the telephoto lens: %+v", p)
	}
	// a point on the telephoto picture: its own fraction is 2.8x smaller in the wide one
	q := aimTarget(m, cur, 2.8, 2.8, 2.8, 0, 0.28, 0)
	if want := 500 - int(math.Round(0.28*aimW/2.8/m.PxX)); q.X != want {
		t.Errorf("a point on the telephoto picture: x %d, want %d", q.X, want)
	}
	// the motor's lowest tilt is 0
	if p := aimTarget(m, Position{X: 500, Y: 2}, 1, 2.8, 1, 0, 0, -0.45); p.Y != 0 {
		t.Errorf("below the range: y %d", p.Y)
	}
}
