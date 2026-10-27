package main

import (
	"fyne.io/fyne/v2"
	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// updateCaps detects the capabilities of the selected camera (in the background) and
// shows/hides its controls.
func (u *ui) updateCaps() {
	name := u.camera.Selected
	go func() {
		c := allCaps
		if name != "" {
			c = cameraCaps(name)
		}
		fyne.Do(func() {
			if name == u.camera.Selected {
				u.caps = c
				u.applyCaps()
			}
		})
	}()
}

func (u *ui) applyCaps() {
	show := func(on bool, objs ...fyne.CanvasObject) {
		for _, o := range objs {
			if o == nil {
				continue
			}
			if on {
				o.Show()
			} else {
				o.Hide()
			}
		}
	}
	c := u.caps
	show(c.PTZ, u.capPTZ, u.capOverlayPTZ)
	show(c.Points, u.capPoints)
	show(true, u.capZoom) // the digital zoom works on every camera (digizoom.go)
	show(c.Zoom, u.capOverlayZoom)
	show(c.Tele > 0, u.lens)
	u.showLens(u.camZoom)
	show(xccore.CanAim(u.camera.Selected), u.aimBtn)
	u.setAimMode(false)
	u.showBack()
	u.showWide()
	show(c.Light, u.capLight)
	show(c.Brightness, u.brightness)
	show(c.Siren, u.siren)
	if c.NightMode {
		u.setNightList(cameraNightModes(u.camera.Selected))
	}
	show(c.NightMode, u.capNight)
	show(c.WDR, u.wdr)
	show(c.LowLightColor, u.lowLight)
	show(c.Light || c.Siren || c.NightMode || c.WDR || c.LowLightColor, u.camControls)
	show(c.History, u.hist.button)
	show(len(cameraEvents(u.camera.Selected)) > 0, u.hist.eventBtns...)
}
