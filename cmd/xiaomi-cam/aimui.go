package main

import (
	"fmt"
	"image"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// Aiming in the picture (CW700S): Ctrl+click or the aim button and a click turn the camera
// so that the point comes to the centre of the telephoto lens (xccore.AimAt). While on the
// telephoto lens, the wide picture from before zooming in stays in the corner with a frame
// of where the lens looks; a click on it aims there. Manual control starts the countdown of
// the way back to the watch view (guard.go), shown on the back button.

// wideView - the wide picture of a camera kept for the overview, and where it was taken.
type wideView struct {
	img  *image.RGBA
	pos  xccore.Position
	turn int
	ok   bool // the position is known
}

// initAim wires the aiming, the overview and the way back into the UI (after build).
func (u *ui) initAim() {
	u.wide = map[string]*wideView{}
	u.camPos = map[string]xccore.Position{}
	u.guard = newWatchGuard(func(name string, err error) {
		fyne.Do(func() {
			if name != u.camera.Selected {
				return
			}
			u.showBack()
			away, returning, _, _ := u.guard.state(name)
			switch {
			case returning:
				u.status.SetText(T("Wracam do widoku obserwacji…"))
			case err != nil:
				u.status.SetText(TJ("Powrót do obserwacji: ", err.Error()))
			case !away && u.backShown:
				u.status.SetText(T("Kamera wróciła do widoku obserwacji"))
				u.video.setDigitalZoom(1, nil)
				u.refreshPosition(name, 0)
			}
			u.backShown = away
		})
	})
	apiManual, apiGuard = u.guard.touch, u.guard
	go u.guard.watchRest(cameraNames)
	go func() { // the countdown on the back button
		for range time.Tick(time.Second) {
			fyne.Do(u.showBack)
		}
	}()
}

// aimTap - a click on the picture with Ctrl or in the aim mode (videoView.onAimTap).
func (u *ui) aimTap(fx, fy float64) bool {
	if !u.aimMode && !u.video.ctrlDown {
		return false
	}
	if xccore.Moving(u.camera.Selected) {
		// the picture comes 0.8-1.3 s after the camera (measured): a point clicked on it while
		// the camera turns is not where the camera is - wait until it stands (aim mode stays)
		u.status.SetText(T("Kamera jeszcze się obraca, a obraz dochodzi z opóźnieniem około sekundy – kliknij, gdy stanie"))
		return true
	}
	u.setAimMode(false)
	if u.grid || !xccore.CanAim(u.camera.Selected) {
		return false
	}
	u.aimAt(fx, fy)
	return true
}

// buildAimButtons - the aim button and the way back (hidden until the camera is away).
func (u *ui) buildAimButtons() fyne.CanvasObject {
	u.aimBtn = overlayButton(lucide("circle-dot"), T("Celowanie: kliknij punkt na obrazie – kamera obróci się do niego i przełączy na teleobiektyw (albo Ctrl+klik na obrazie)"), func() {
		u.setAimMode(!u.aimMode)
	})
	u.aimBtn.Hide()
	u.backBtn = overlayButton(lucide("history"), "", u.goBackNow)
	u.backBtn.Hide()
	return container.NewHBox(u.aimBtn, u.backBtn)
}

// setAimMode: the next click on the picture aims the camera (crosshair cursor).
func (u *ui) setAimMode(on bool) {
	u.aimMode = on
	u.video.aimMode = on
	if u.aimBtn != nil {
		u.aimBtn.Importance = widget.LowImportance
		if on {
			u.aimBtn.Importance = widget.HighImportance
		}
		u.aimBtn.Refresh()
	}
	if on {
		u.status.SetText(T("Kliknij punkt na obrazie, do którego ma się obrócić kamera (Esc – anuluj)"))
	}
}

// keepWide keeps the picture shown now as the overview of the camera, when it is the wide
// one (the position comes with the move: noteWidePos).
func (u *ui) keepWide(name string) {
	// (a turning camera: the picture shown is ~1 s older than the position read with it)
	if onTele(u.camZoom, u.caps.Tele) || u.video.frame == nil || u.video.zoomed() || xccore.Moving(name) {
		return
	}
	u.wide[name] = &wideView{img: shrink(u.video.frame, 480)}
}

func (u *ui) noteWidePos(name string, pos xccore.Position, turn int) {
	fyne.Do(func() {
		if w := u.wide[name]; w != nil && !w.ok {
			w.pos, w.turn, w.ok = pos, turn, true
		}
		u.camPos[name] = pos
	})
}

// aimAt turns the selected camera to the point (fx, fy) of its picture (fractions from the
// centre) on the telephoto lens.
func (u *ui) aimAt(fx, fy float64) {
	name := u.camera.Selected
	u.guard.touch(name)
	u.keepWide(name)
	z := u.camZoom
	u.status.SetText(T("Celuję kamerę…"))
	done := u.busy(nil)
	go func() {
		cur, err := xccore.MotorPosition(name)
		var p xccore.Position
		if err == nil {
			if cur.Scale >= 10 {
				z = float64(cur.Scale) / 10
			}
			u.noteWidePos(name, cur, xccore.PictureTurn(name))
			p, err = xccore.AimFrom(name, cur, z, fx, fy, "tele")
		}
		u.aimDone(name, p, err, done)
	}()
}

// aimAtWide - a click on the overview: the point of the wide picture (fractions from its
// centre) to the centre of the lens the camera is on.
func (u *ui) aimAtWide(fx, fy float64) {
	name := u.camera.Selected
	w := u.wide[name]
	if w == nil || !w.ok {
		return
	}
	if xccore.Moving(name) {
		u.status.SetText(T("Kamera jeszcze się obraca, a obraz dochodzi z opóźnieniem około sekundy – kliknij, gdy stanie"))
		return
	}
	u.guard.touch(name)
	u.status.SetText(T("Celuję kamerę…"))
	done := u.busy(nil)
	go func() {
		p, err := xccore.AimFrom(name, w.pos, 1, fx, fy, "")
		u.aimDone(name, p, err, done)
	}()
}

func (u *ui) aimDone(name string, p xccore.Position, err error, done func()) {
	fyne.Do(func() {
		done()
		if err != nil {
			u.status.SetText(TJ("Celowanie: ", err.Error()))
			u.refreshPosition(name, 0)
			return
		}
		u.camPos[name] = p
		u.video.setDigitalZoom(1, nil)
		u.status.SetText(T("Kamera wycelowana"))
		u.showWide()
	})
}

// refreshPosition reads where the camera looks after a while (a turn by hand ended) and
// moves the frame on the overview.
func (u *ui) refreshPosition(name string, after time.Duration) {
	if !xccore.CanAim(name) {
		return
	}
	go func() {
		time.Sleep(after)
		p, err := xccore.MotorPosition(name)
		if err != nil {
			return
		}
		fyne.Do(func() {
			u.camPos[name] = p
			u.showWide()
		})
	}()
}

// showWide shows the overview while the selected camera is on its telephoto lens.
func (u *ui) showWide() {
	if u.guard == nil || u.video == nil {
		return
	}
	name := u.camera.Selected
	w, pos := u.wide[name], u.camPos[name]
	if u.grid || w == nil || !w.ok || !onTele(u.camZoom, u.caps.Tele) || pos == (xccore.Position{}) {
		u.video.hideWide()
		return
	}
	cx, cy, fw, fh := xccore.WideFrame(name, w.pos, pos, u.camZoom, w.turn)
	u.video.showWideView(w.img, cx, cy, fw, fh)
}

// showBack shows the back button of the selected camera while it is away from its watch
// view, with the time left.
func (u *ui) showBack() {
	if u.backBtn == nil || u.guard == nil {
		return
	}
	away, returning, until, _ := u.guard.state(u.camera.Selected)
	if !away {
		if u.backBtn.Visible() {
			u.backBtn.Hide()
		}
		return
	}
	left := max(time.Until(until), 0).Round(time.Second)
	text := fmt.Sprintf("%d:%02d", int(left.Minutes()), int(left.Seconds())%60)
	if returning {
		text = "…"
	}
	if u.backBtn.Text != text {
		u.backBtn.SetText(text)
	}
	u.backBtn.tip = fmt.Sprintf(T("Wróć do widoku obserwacji teraz – kamera wróci sama za %s"), text)
	if !u.backBtn.Visible() {
		u.backBtn.Show()
	}
}

// goBackNow - the back button.
func (u *ui) goBackNow() {
	name := u.camera.Selected
	go func() { _ = u.guard.goBack(name) }()
}

// shrink returns a copy of f at most w pixels wide (box filter: the overview is small).
func shrink(f *image.RGBA, w int) *image.RGBA {
	b := f.Bounds()
	step := max(1, (b.Dx()+w-1)/w)
	out := image.NewRGBA(image.Rect(0, 0, b.Dx()/step, b.Dy()/step))
	for y := 0; y < out.Rect.Dy(); y++ {
		for x := 0; x < out.Rect.Dx(); x++ {
			var r, g, bl, n int
			for dy := 0; dy < step; dy++ {
				i := (b.Min.Y+y*step+dy)*f.Stride + (b.Min.X+x*step)*4
				for dx := 0; dx < step; dx++ {
					r += int(f.Pix[i])
					g += int(f.Pix[i+1])
					bl += int(f.Pix[i+2])
					i += 4
					n++
				}
			}
			o := y*out.Stride + x*4
			out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3] = uint8(r/n), uint8(g/n), uint8(bl/n), 255
		}
	}
	return out
}
