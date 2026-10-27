package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// Camera points 1-3 (preset positions stored in the camera): buttons in the bottom bar,
// keys 1-3, and a dialog to save the current position / rename / remove.

// pointNameSize - the point name under its number (small, the button stays compact)
const pointNameSize = 10

func (u *ui) buildPoints() fyne.CanvasObject {
	box := container.NewHBox()
	for i := 0; i < presetSlots; i++ {
		slot := i + 1
		b := newButton("", nil, func() { u.pointPressed(slot) })
		b.tip = fmt.Sprintf(T("Punkt %d – kliknij, aby obrócić kamerę; pusty: zapisz bieżącą pozycję (klawisz %d)"), slot, slot)
		u.pointButtons[i] = b
		// the number with the name under it, on the button
		num := canvas.NewText(fmt.Sprint(slot), palette.text)
		num.TextStyle.Bold = true
		num.Alignment = fyne.TextAlignCenter
		name := canvas.NewText("", palette.muted)
		name.TextSize = pointNameSize
		name.Alignment = fyne.TextAlignCenter
		u.pointNames[i] = name
		texts := container.NewCenter(container.New(&tightVBox{}, num, name))
		b.busyChanged = func(busy bool) { // the spinner is shown instead of the texts
			if busy {
				texts.Hide()
			} else {
				texts.Show()
			}
		}
		u.pointWraps[i] = container.NewGridWrap(fyne.NewSize(44, 40), b)
		box.Add(container.NewStack(u.pointWraps[i], texts))
	}
	box.Add(newButton("", theme.SettingsIcon(), u.showPoints).withTip(T("Punkty kamery – zapisz bieżącą pozycję, usuń")))
	return box
}

// loadPoints reads the points of the selected camera (in the background) and updates the buttons.
func (u *ui) loadPoints() {
	name := u.camera.Selected
	if cameraSource(name) == "" && ipCam(name) == nil {
		return // no camera
	}
	go func() {
		list, err := getPresets(name)
		fyne.Do(func() {
			if name != u.camera.Selected {
				return // another camera was selected meanwhile
			}
			u.points = presetSlotsOf(list) // empty on error (e.g. a camera without points)
			_ = err
			for i, b := range u.pointButtons {
				if b == nil {
					continue
				}
				name := u.pointNames[i]
				if p := u.points[i]; p != nil {
					label := p.Label()
					if r := []rune(label); len(r) > 10 {
						label = string(r[:9]) + "…"
					}
					if label == fmt.Sprint(i+1) {
						label = "" // no name (the number only)
					}
					name.Text = label
					b.Importance = widget.MediumImportance
				} else {
					name.Text = ""
					b.Importance = widget.LowImportance
				}
				// a name wider than the button widens it
				w := max(fyne.MeasureText(name.Text, pointNameSize, name.TextStyle).Width+14, 44)
				u.pointWraps[i].Layout = layout.NewGridWrapLayout(fyne.NewSize(w, 40))
				u.pointWraps[i].Refresh()
				name.Refresh()
				b.Refresh()
			}
		})
	}()
}

// pointPressed turns to a defined point, or offers to save the current position.
func (u *ui) pointPressed(slot int) {
	name := u.camera.Selected
	if name == "" || !u.caps.Points {
		return
	}
	b := u.pointButtons[slot-1]
	if b.isBusy() {
		return
	}
	if p := u.points[slot-1]; p != nil {
		u.status.SetText(fmt.Sprintf(T("Obracam do punktu %d (%s)…"), slot, p.Label()))
		pt := *p
		done := u.busy(b)
		if u.guard != nil {
			u.guard.touch(name)
		}
		go func() {
			err := gotoPreset(name, pt.Idx)
			if err == nil {
				pointReached(name, pt)
			}
			fyne.Do(func() {
				done()
				if err != nil {
					dialog.ShowError(err, u.win)
				}
			})
		}()
		return
	}
	u.savePointDialog(slot)
}

func (u *ui) savePointDialog(slot int) {
	u.pointNameDialog(slot, false)
}

// pointNameDialog asks for the name of point slot: rename only (the position stays) or save
// the current position with the name.
func (u *ui) pointNameDialog(slot int, rename bool) {
	entry := widget.NewEntry()
	if p := u.points[slot-1]; p != nil {
		entry.SetText(p.Label())
	}
	entry.SetPlaceHolder(T("np. auto, podjazd, brama"))
	title := fmt.Sprintf(T("Punkt %d – zapisz bieżącą pozycję"), slot)
	if rename {
		title = fmt.Sprintf(T("Punkt %d – zmień nazwę"), slot)
	}
	d := dialog.NewForm(title, T("Zapisz"), T("Anuluj"),
		[]*widget.FormItem{widget.NewFormItem(T("Nazwa"), entry)}, func(ok bool) {
			if !ok {
				return
			}
			name, label := u.camera.Selected, strings.TrimSpace(entry.Text)
			u.status.SetText(fmt.Sprintf(T("Zapisuję punkt %d…"), slot))
			done := u.busy(u.pointButtons[slot-1])
			go func() {
				var err error
				if rename {
					err = renamePresetSlot(name, slot, label)
				} else {
					err = savePresetSlot(name, slot, label)
				}
				fyne.Do(func() {
					done()
					if err != nil {
						dialog.ShowError(err, u.win)
						return
					}
					u.status.SetText(fmt.Sprintf(T("Zapisano punkt %d"), slot))
					u.loadPoints()
				})
			}()
		}, u.win)
	entry.OnSubmitted = func(string) { d.Submit() }
	d.Resize(fyne.NewSize(420, 0))
	d.Show()
	u.win.Canvas().Focus(entry) // typing goes to the name, not to the keyboard shortcuts
}

// showPoints - manage points 1-3: save the current position, remove.
func (u *ui) showPoints() {
	rows := container.NewVBox()
	var d dialog.Dialog
	for i := 0; i < presetSlots; i++ {
		slot := i + 1
		desc := T("(pusty)")
		if p := u.points[i]; p != nil {
			desc = p.Label()
		}
		var del *ptrButton
		del = newButton("", theme.DeleteIcon(), func() {
			p := u.points[slot-1]
			if p == nil {
				return
			}
			name, idx := u.camera.Selected, p.Idx
			done := u.busy(del)
			go func() {
				err := deletePreset(name, idx)
				fyne.Do(func() {
					done()
					if err != nil {
						dialog.ShowError(err, u.win)
					}
					d.Hide()
					u.loadPoints()
				})
			}()
		})
		ren := newButton("", theme.DocumentCreateIcon(), func() {
			d.Hide()
			u.pointNameDialog(slot, true)
		}).withTip(T("Zmień nazwę punktu (pozycja zostaje)"))
		if u.points[i] == nil {
			del.Disable()
			ren.Disable()
		}
		rows.Add(container.NewBorder(nil, nil,
			widget.NewLabelWithStyle(fmt.Sprintf(T("Punkt %d"), slot), fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			container.NewHBox(
				ren,
				newButton(T("Zapisz bieżącą pozycję"), theme.DocumentSaveIcon(), func() {
					d.Hide()
					u.savePointDialog(slot)
				}),
				del),
			widget.NewLabel(desc)))
	}
	help := widget.NewLabel(T("Ustaw kamerę (strzałki, zoom) i zapisz pozycję jako punkt. Kliknięcie punktu lub klawisz 1–3 obraca kamerę do niego. Punkty są zapisane w kamerze (widoczne też w Mi Home)."))
	help.Wrapping = fyne.TextWrapWord
	rtlAlign(help)
	help.Importance = widget.LowImportance
	d = dialog.NewCustom(T("Punkty kamery"), T("Zamknij"), container.NewVBox(rows, help), u.win)
	d.Resize(fyne.NewSize(620, 0))
	d.Show()
}
