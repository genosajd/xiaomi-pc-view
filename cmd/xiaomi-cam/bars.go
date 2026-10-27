package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// Compact bars: less used controls are in a menu (top bar) and a panel (bottom bar), so the
// window can be narrow.

// appMenuButton - the ☰ menu of the top bar: view modes, cameras, event detection and the
// events (cameras with events), language, shortcuts and version.
func (u *ui) appMenuButton() *ptrButton {
	var b *ptrButton
	b = newButton("", theme.MenuIcon(), func() {
		items := []*fyne.MenuItem{
			fyne.NewMenuItemWithIcon(TM("Tylko obraz – ukrywa panele i ramkę okna (H, powrót: Esc)"), theme.VisibilityOffIcon(), func() { u.setViewOnly(true) }),
			fyne.NewMenuItemWithIcon(TM("Pełny ekran (F, F11, podwójne kliknięcie)"), theme.ViewFullScreenIcon(), u.toggleFullScreen),
			fyne.NewMenuItemSeparator(),
			fyne.NewMenuItemWithIcon(TM("Kamery – dodawanie, zmiana nazwy, usuwanie"), cameraIcon, u.showCameras),
		}
		if xccore.CanTurnPicture(u.camera.Selected) {
			items = append(items, fyne.NewMenuItemWithIcon(TM("Obróć obraz o 180° (kamera zawieszona do góry nogami)"),
				theme.ViewRefreshIcon(), u.turnPicture))
		}
		if len(cameraEvents(u.camera.Selected)) > 0 {
			items = append(items,
				fyne.NewMenuItemWithIcon(TM("Wykrywanie zdarzeń – ruch, ludzie, samochody"), lucide("scan-eye"), u.showDetection))
		}
		if hasEventCameras() {
			items = append(items, fyne.NewMenuItemWithIcon(TM("Zdarzenia – ostatnie wykrycia"), lucide("bell"), u.showEvents))
		}
		items = append(items, fyne.NewMenuItemSeparator())
		for _, l := range languages {
			code := l.code
			it := fyne.NewMenuItem(l.name, func() { u.setLanguage(code) })
			it.Checked = code == lang
			items = append(items, it)
		}
		items = append(items, fyne.NewMenuItemSeparator(),
			fyne.NewMenuItemWithIcon(TM("Skróty klawiaturowe i wersja"), theme.HelpIcon(), u.showShortcuts))
		showMenuBelow(b, fyne.NewMenu("", items...))
	}).withTip(T("Menu: tylko obraz, pełny ekran, kamery, zdarzenia, język, skróty klawiaturowe i wersja"))
	return b
}

// turnPicture turns the camera's own picture by 180° (0 <-> 180): a camera hung upside down
// to tilt up to the sky shows the sky upright - in every viewer and recording, as the camera
// itself sends it turned. The Mi cloud answers off the UI thread.
func (u *ui) turnPicture() {
	name := u.camera.Selected
	var err error
	var now int
	inBackground(u.win, func() {
		props, e := getProps(name, propImageRollover)
		if e != nil || len(props) == 0 || props[0].Code != 0 {
			err = fmt.Errorf("%s", T("Kamera nie podała obrotu obrazu"))
			if e != nil {
				err = e
			}
			return
		}
		cur, _ := props[0].Value.(float64)
		now = 180
		if int(cur) == 180 {
			now = 0
		}
		err = setProp(name, propImageRollover, now)
	}, func() {
		if err != nil {
			dialog.ShowError(err, u.win)
		}
	})
}

// showMenuBelow opens a menu under the button, aligned to its right edge.
func showMenuBelow(b fyne.CanvasObject, m *fyne.Menu) {
	c := fyne.CurrentApp().Driver().CanvasForObject(b)
	if c == nil {
		return
	}
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(b)
	pop := widget.NewPopUpMenu(m, c)
	w := pop.MinSize().Width
	pop.ShowAtPosition(fyne.NewPos(max(pos.X+b.Size().Width-w, 0), pos.Y+b.Size().Height))
}

// cameraControlsButton - the bottom bar button of the panel with the spotlight, siren and
// night mode (the controls the camera has, caps.go).
func (u *ui) cameraControlsButton() *ptrButton {
	var b *ptrButton
	b = newButton("", lightIcon, func() {
		c := u.win.Canvas()
		panel := container.NewVBox(
			container.NewGridWrap(fyne.NewSize(300, u.capLight.MinSize().Height), u.capLight),
			u.siren,
			u.capNight,
			u.wdr,
			u.lowLight,
		)
		pop := widget.NewPopUp(container.NewPadded(panel), c)
		// above the button, kept inside the window
		pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(b)
		size := pop.MinSize()
		x := min(pos.X, c.Size().Width-size.Width)
		pop.ShowAtPosition(fyne.NewPos(max(x, 0), max(pos.Y-size.Height-4, 0)))
	}).withTip(T("Reflektor, jasność, syrena, tryb nocny i ustawienia obrazu kamery"))
	return b
}
