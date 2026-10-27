package main

import (
	"math"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// The two lenses of a dual-lens camera (CW700S): wide-angle and telephoto. The camera
// chooses the lens by its own zoom (caps.Tele, xccore.TeleZoom): below it the wide lens
// cropped, from it on the telephoto sensor in full detail. The camera sends one lens at a
// time, so "the second lens" is a zoom value: the lens control sets it, the zoom steps stop
// at it on the way, and the control shows the lens the zoom is on.

// buildLens makes the Wide | Tele control (hidden on one-lens cameras: applyCaps).
func (u *ui) buildLens() *segmented {
	u.lens = newSegmented(func(string) {
		if u.lens.SelectedIndex() == 1 {
			name := u.camera.Selected
			u.keepWide(name) // the overview while on the telephoto lens (aimui.go)
			go func() {
				if p, err := xccore.MotorPosition(name); err == nil {
					u.noteWidePos(name, p, xccore.PictureTurn(name))
				}
			}()
			u.video.setDigitalZoom(1, nil) // the lens's own picture, not a crop of it
			u.setZoom(u.caps.Tele)
		} else {
			u.zoomReset()
		}
	})
	u.lens.Options = []string{T("Szeroki"), T("Tele")}
	u.lens.Selected = u.lens.Options[0]
	u.lens.tip = T("Obiektyw kamery: szeroki kąt albo teleobiektyw – ostre szczegóły z daleka. Kamera wysyła obraz jednego obiektywu naraz, więc widzą go wszyscy, którzy oglądają tę kamerę (OBS, inne programy)")
	return u.lens
}

// showLens highlights the lens the camera zoom z is on.
func (u *ui) showLens(z float64) {
	if u.lens == nil || u.caps.Tele == 0 {
		return
	}
	i := 0
	if onTele(z, u.caps.Tele) {
		i = 1
	}
	if u.lens.Selected != u.lens.Options[i] {
		u.lens.Selected = u.lens.Options[i]
		u.lens.Refresh()
	}
}

func onTele(z, tele float64) bool { return tele > 0 && z >= tele-0.01 }

// nextZoom returns the camera zoom one step (delta) from z. A dual-lens camera stops at
// its telephoto lens on the way (tele: its sharpest picture, the steps of 0.5 would jump
// over it), and the next step from there goes back to the usual steps.
func nextZoom(z, delta, tele float64) float64 {
	n := z + delta
	switch {
	case tele == 0 || math.Abs(delta) > 1: // one lens, or a jump (back to 1×)
	case math.Abs(z-tele) < 0.01 && delta > 0:
		n = math.Ceil((tele+0.01)/zoomStep) * zoomStep
	case math.Abs(z-tele) < 0.01 && delta < 0:
		n = math.Floor((tele-0.01)/zoomStep) * zoomStep
	case z < tele && n > tele, z > tele && n < tele:
		n = tele
	}
	return min(max(n, 1), maxZoom)
}

// snapTele - a zoom value less than half a step from the telephoto lens (the slider moves
// in steps of 0.5) is the lens itself.
func snapTele(z, tele float64) float64 {
	if tele > 0 && math.Abs(z-tele) < zoomStep/2 {
		return tele
	}
	return z
}
