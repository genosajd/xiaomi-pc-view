package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// showSettings - app settings: folders for recordings and snapshots.
func (u *ui) showSettings() {
	folderRow := func(pref string, current func() string) fyne.CanvasObject {
		label := widget.NewLabel(current())
		label.Truncation = fyne.TextTruncateEllipsis
		choose := newButton(T("Zmień…"), theme.FolderOpenIcon(), func() {
			d := dialog.NewFolderOpen(func(dir fyne.ListableURI, err error) {
				if err != nil || dir == nil {
					return
				}
				u.app.Preferences().SetString(pref, dir.Path())
				label.SetText(current())
			}, u.win)
			if l, err := storage.ListerForURI(storage.NewFileURI(current())); err == nil {
				d.SetLocation(l)
			}
			d.Resize(fyne.NewSize(700, 480))
			d.Show()
		})
		reset := newButton("", theme.ViewRefreshIcon(), func() {
			u.app.Preferences().RemoveValue(pref) // back to the system folder
			label.SetText(current())
		})
		choose.tip = T("Wybierz folder")
		reset.tip = T("Przywróć domyślny folder systemowy")
		return container.NewBorder(nil, nil, nil, container.NewHBox(choose, reset), label)
	}
	modes := []struct{ name, id string }{
		{"Wybrana kamera", recordSelected},
		{"Każda kamera osobno (osobne pliki)", recordEach},
		{"Siatka wszystkich kamer (jeden plik)", recordGrid},
	}
	var modeNames []string
	for _, m := range modes {
		modeNames = append(modeNames, T(m.name))
	}
	mode := widget.NewSelect(modeNames, nil)
	for i, m := range modes {
		if m.id == recordMode() {
			mode.SetSelectedIndex(i)
		}
	}
	if mode.SelectedIndex() < 0 {
		mode.SetSelectedIndex(0)
	}
	// cameras recorded in the each/grid modes
	recorded := map[string]bool{}
	for _, n := range recordCameras() {
		recorded[n] = true
	}
	camChecks := container.NewVBox()
	for _, name := range cameraNames() {
		name := name
		c := newCheck(name, func(on bool) { setRecordCamera(name, on) })
		c.SetChecked(recorded[name])
		camChecks.Add(c)
	}
	camLabel := widget.NewLabel(T("Nagrywane kamery:"))
	showCams := func() {
		if id := modes[max(mode.SelectedIndex(), 0)].id; id == recordEach || id == recordGrid {
			camLabel.Show()
			camChecks.Show()
		} else {
			camLabel.Hide()
			camChecks.Hide()
		}
	}
	mode.OnChanged = func(string) {
		u.app.Preferences().SetString(prefRecordMode, modes[max(mode.SelectedIndex(), 0)].id)
		showCams()
	}
	showCams()
	modeHelp := widget.NewLabel(T("Wybrana kamera i osobne pliki kopiują obraz z kamer bez przekodowania (prawie bez obciążenia). Siatka składa kamery w jeden film H.265 – to przekodowanie obciąża procesor."))
	modeHelp.Wrapping = fyne.TextWrapWord
	rtlAlign(modeHelp)
	modeHelp.Importance = widget.LowImportance

	autostart := newCheck(T("Uruchamiaj przy logowaniu (schowana w zasobniku)"), func(on bool) {
		if err := setAutostart(on); err != nil {
			dialog.ShowError(err, u.win)
		}
	}).withTip(T("Aplikacja startuje po zalogowaniu i czeka w zasobniku – zadania AI i powiadomienia działają od razu"))
	autostart.SetChecked(autostartEnabled())

	// the PC archive (archive.go): off until switched on here
	archive := newCheck(T("Odtwarzaj historię z archiwum na komputerze"), func(on bool) {
		u.app.Preferences().SetBool(prefArchive, on)
	})
	archive.SetChecked(u.app.Preferences().Bool(prefArchive))
	archiveAddr := widget.NewEntry()
	archiveAddr.SetPlaceHolder(archiveDefault)
	archiveAddr.SetText(u.app.Preferences().String(prefArchiveURL))
	archiveAddr.OnChanged = func(s string) {
		u.app.Preferences().SetString(prefArchiveURL, strings.TrimSpace(s))
	}
	archiveHelp := widget.NewLabel(T("Nagrania usługi cam-archive (osobny program, który nagrywa kamerę na tym komputerze) w pasku historii: tydzień wstecz bez łączenia z kamerą."))
	archiveHelp.Wrapping = fyne.TextWrapWord
	rtlAlign(archiveHelp)
	archiveHelp.Importance = widget.LowImportance

	startViews := []struct{ name, id string }{
		{"Siatka wszystkich kamer", ""},
		{"Ostatni widok (kamera albo siatka)", "last"},
	}
	var viewNames []string
	for _, v := range startViews {
		viewNames = append(viewNames, T(v.name))
	}
	startView := newSelect(viewNames, nil)
	startView.SetSelectedIndex(0)
	for i, v := range startViews {
		if v.id == u.app.Preferences().String(prefStartView) {
			startView.SetSelectedIndex(i)
		}
	}
	startView.OnChanged = func(string) {
		u.app.Preferences().SetString(prefStartView, startViews[max(startView.SelectedIndex(), 0)].id)
	}
	startView.tip = T("Co pokazuje okno po uruchomieniu aplikacji (siatka – przy 2 i więcej kamerach)")

	// the way back to the watch view after manual control (guard.go)
	var backNames []string
	for _, m := range watchReturnChoices {
		if m == 0 {
			backNames = append(backNames, T("Wyłączony"))
		} else {
			backNames = append(backNames, fmt.Sprintf(T("%d min"), m))
		}
	}
	back := newSelect(backNames, nil)
	for i, m := range watchReturnChoices {
		if m == watchReturnMinutes() {
			back.SetSelectedIndex(i)
		}
	}
	back.OnChanged = func(string) {
		u.app.Preferences().SetInt(prefWatchReturn, watchReturnChoices[max(back.SelectedIndex(), 0)])
	}
	back.tip = T("Kamera obrócona albo przybliżona ręcznie (też przez API lub AI) wraca sama tam, gdzie patrzyła wcześniej, po tylu minutach bez sterowania – żeby obserwacja (wykrywanie na niebie, nagrywanie, transmisja) nie została ślepa")

	// the camera on air: one RTSP address for OBS, the camera behind it is chosen here
	onAirNames := cameraNames()
	onAir := newSelect(onAirNames, nil)
	onAir.SetSelected(xccore.OnAirCamera())
	onAir.OnChanged = func(name string) {
		go func() {
			if err := xccore.SetOnAir(name); err != nil {
				fyne.Do(func() { dialog.ShowError(err, u.win) })
			}
		}()
	}
	onAir.tip = T("Kamera, którą OBS pokazuje pod stałym adresem (tylko obraz, bez mikrofonu)")
	onAirURL := widget.NewEntry()
	onAirURL.SetText(xccore.OnAirURL())
	onAirURL.Disable()
	copyURL := newButton(T("Kopiuj"), theme.ContentCopyIcon(), func() {
		u.app.Clipboard().SetContent(xccore.OnAirURL())
		u.status.SetText(T("Adres skopiowany"))
	})
	onAirHelp := widget.NewLabel(T("W OBS: Źródło multimediów, odznacz „Plik lokalny”, wklej adres. Zmiana kamery tutaj przełącza obraz w OBS bez ruszania sceny."))
	onAirHelp.Wrapping = fyne.TextWrapWord
	rtlAlign(onAirHelp)
	onAirHelp.Importance = widget.LowImportance

	// the phone page (phone.go): off until switched on here
	var phone *ptrCheck
	phone = newCheck(T("Strona dla telefonu"), func(on bool) {
		if err := setPhonePage(on); err != nil {
			dialog.ShowError(err, u.win)
			phone.SetChecked(false)
			return
		}
		u.app.Preferences().SetBool(prefPhonePage, on)
	}).withTip(T("Podgląd i sterowanie kamer w przeglądarce telefonu"))
	phone.SetChecked(u.app.Preferences().Bool(prefPhonePage))
	phoneHelp := widget.NewLabel(T("Adres http://127.0.0.1:11985 działa tylko na tym komputerze. Do telefonu: Tailscale (tailscale serve --bg 11985) – bez otwierania portów w routerze."))
	phoneHelp.Wrapping = fyne.TextWrapWord
	rtlAlign(phoneHelp)
	phoneHelp.Importance = widget.LowImportance

	form := widget.NewForm(
		widget.NewFormItem(T("Na antenie (OBS)"), container.NewVBox(onAir,
			container.NewBorder(nil, nil, nil, copyURL, onAirURL), onAirHelp)),
		widget.NewFormItem(T("Telefon"), container.NewVBox(phone, phoneHelp)),
		widget.NewFormItem(T("Start"), autostart),
		widget.NewFormItem(T("Widok na starcie"), startView),
		widget.NewFormItem(T("Powrót do obserwacji"), back),
		widget.NewFormItem(T("Tryb nagrywania"), container.NewVBox(mode, camLabel, camChecks, modeHelp)),
		widget.NewFormItem(T("Nagrania"), folderRow(prefRecordingsDir, recordingsDir)),
		widget.NewFormItem(T("Zdjęcia"), folderRow(prefSnapshotsDir, snapshotsDir)),
		widget.NewFormItem(T("Archiwum na komputerze"), container.NewVBox(archive, archiveAddr, archiveHelp)),
	)
	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon(T("Ogólne"), theme.SettingsIcon(), container.NewVBox(form)),
		container.NewTabItemWithIcon("AI", theme.ComputerIcon(), u.aiSettings()),
		container.NewTabItemWithIcon(T("Zdarzenia"), lucide("bell"), u.eventsSettings()),
		container.NewTabItemWithIcon(T("Powiadomienia"), theme.MailComposeIcon(), u.notificationSettings()),
	)
	d := dialog.NewCustom(T("Ustawienia"), T("Zamknij"), tabs, u.win)
	d.SetOnClosed(func() {
		watch.mu.Lock()
		watch.onChange = nil
		watch.mu.Unlock()
		u.ai.onChange = nil
		u.stopAIRunSpinners()
	})
	d.Resize(fyne.NewSize(860, 640))
	d.Show()
}

// toggleRecording starts recording in the mode from the settings, or stops all recordings.
func (u *ui) toggleRecording() {
	name := u.camera.Selected
	if name == "" {
		return
	}
	if u.recButton.isBusy() {
		return
	}
	done := u.busy(u.recButton)
	if len(activeRecordings()) > 0 {
		go func() {
			stopped := stopAll()
			fyne.Do(func() {
				done()
				u.updateRecordButton()
				var files, errs []string
				for _, r := range stopped {
					if r.err != nil {
						errs = append(errs, r.err.Error())
					} else {
						files = append(files, filepath.Base(r.path))
					}
				}
				if len(files) > 0 {
					u.status.SetText(TJ("Zapisano ", strings.Join(files, ", ")))
				} else {
					u.status.SetText("")
				}
				if len(errs) > 0 {
					dialog.ShowError(errors.New(strings.Join(errs, "\n")), u.win)
				}
			})
		}()
		return
	}
	mode := recordMode()
	go func() {
		_, err := startRecordings(mode, name, nil, 0)
		fyne.Do(func() {
			done()
			u.updateRecordButton()
			if err != nil {
				dialog.ShowError(err, u.win)
			}
		})
	}()
}

// updateRecordButton - record button look and the status line while recording
// (UI thread, called every second and when a recording starts/stops).
func (u *ui) updateRecordButton() {
	if u.recButton == nil {
		return
	}
	active := activeRecordings()
	if len(active) == 0 {
		if u.recButton.Importance != widget.MediumImportance {
			u.recButton.setIdleIcon(recordIcon)
			u.recButton.SetText(T("Nagraj"))
			u.recButton.Importance = widget.MediumImportance
			u.recButton.Refresh()
		}
		return
	}
	if u.recButton.Importance != widget.DangerImportance {
		u.recButton.setIdleIcon(theme.MediaStopIcon())
		u.recButton.SetText(T("Stop"))
		u.recButton.Importance = widget.DangerImportance
		u.recButton.Refresh()
	}
	if !active[0].writing() {
		u.status.SetText(T("● Łączenie z kamerami…"))
		return
	}
	d := time.Since(active[0].started).Truncate(time.Second)
	if len(active) > 1 {
		u.status.SetText(fmt.Sprintf(T("● Nagrywanie %s (%d pliki)"), d, len(active)))
	} else {
		u.status.SetText(fmt.Sprintf(T("● Nagrywanie %s"), d))
	}
}
