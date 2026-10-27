// Xiaomi Cam – desktop viewer for Xiaomi Mi Home cameras (e.g. Outdoor Camera CW700S).
//
// One binary: embedded go2rtc (with CS2 connection via Xiaomi servers, so it works from
// any network like the Mi Home app), Fyne UI with tray icon, PTZ, digital zoom, spotlight.
// Video decoding uses system ffmpeg.
package main

import (
	"errors"
	"fmt"
	"github.com/AlexxIT/go2rtc/internal/xccore"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/go-gl/glfw/v3.4/glfw"
)

// startDebug prints a start stage (XC_STARTDEBUG=1): finding why the app doesn't start on a PC
func startDebug(stage string) {
	if os.Getenv("XC_STARTDEBUG") == "1" {
		fmt.Fprintln(os.Stderr, time.Now().Format("15:04:05.000"), "xiaomi-cam:", stage)
	}
}

// devHook runs a self-test (devtest.go, build with -tags devtest)
var devHook func(u *ui)

// version is set at build time: -ldflags "-X main.version=v1.0.0"
var version = "dev"

// quality - a picture quality of a camera: name in the list and picture size
type quality struct {
	name string
	w, h int
}

// cameraQualities returns the qualities of the camera and the index of the selected one:
// Xiaomi 2.5K/720p/SD, IP camera main/sub stream.
func cameraQualities(name string) ([]quality, int) {
	if c := ipCam(name); c != nil {
		size := func(w, h int) (int, int) {
			if w <= 0 || h <= 0 {
				return 1920, 1080 // not known (yet): the picture is fitted in
			}
			return w, h
		}
		w, h := size(c.W, c.H)
		// names: the picture height; "main" / "sub" only when both are the same
		qs := []quality{{fmt.Sprintf("%dp", h), w, h}}
		if c.Sub != "" {
			sw, sh := size(c.SubW, c.SubH)
			qs = append(qs, quality{fmt.Sprintf("%dp", sh), sw, sh})
			if sh == h {
				qs[0].name, qs[1].name = T("Główny"), T("Podgląd")
			}
		}
		return qs, min(max(c.Quality, 0), len(qs)-1)
	}
	var qs []quality
	for _, q := range xccore.CameraQualities(name) {
		qs = append(qs, quality{q.Name, q.W, q.H})
	}
	return qs, xccore.XiaomiQuality(name)
}

// currentQuality returns the selected quality of the camera.
func currentQuality(name string) quality {
	qs, i := cameraQualities(name)
	return qs[i]
}

var connections = []struct{ name, p2p string }{
	{"Zdalne", "server"},
	{"Lokalne (LAN)", ""},
}

type ui struct {
	app fyne.App
	win fyne.Window

	video  *videoView
	player player
	status *statusLabel

	activity   *spinner // spinner next to the status line: an action in progress
	busyCount  int      // actions in progress (busy.go)
	snapButton *ptrButton
	enhButton  *ptrButton   // the enhanced photo (photo.go)
	picWait    *pictureWait // sound on/off: waiting for the new decoder's picture
	night      *ptrSelect   // night vision mode
	nightList  []nightMode  // the night modes of the selected camera (the options of night)
	// wallpaper mode (wallpaper.go)
	wallpaper     bool
	wallStop      chan struct{} // keepBelow running
	remapping     bool          // the window is being re-mapped (window type, frame): not a minimize
	remapSeq      int
	wallpaperItem *fyne.MenuItem
	trayMenu      *fyne.Menu
	capNight      fyne.CanvasObject
	// "run now" buttons of AI tasks being checked (Settings)
	aiRunButtons []*ptrButton

	camera     *ptrSelect
	quality    *segmented
	connection *ptrSelect
	connBox    fyne.CanvasObject // the connection list (Xiaomi only)
	// light, siren, night mode: in a panel opened by this button (bottom bar)
	camControls *ptrButton
	menuButton  *ptrButton
	// controls on the picture (overlay.go)
	pictureControls, bottomControls, statusChip *fyne.Container
	soundBox                                    *fyne.Container
	liveDot                                     *canvas.Circle
	histPreview                                 *histPreview
	areaEdit                                    *areaEditor // the car area / fence editor (areaeditor.go)
	areaTitle                                   *widget.Label
	areaButtons, areaBox                        *fyne.Container
	detectionDlg                                dialog.Dialog // ☰ Event detection, while open
	lastThumb                                   int64         // the unix minute of the last thumbnail
	controlsTimer                               *time.Timer
	// onVideoMouse shows the controls lying on the picture (they hide after a while)
	onVideoMouse func()
	light        *ptrCheck
	brightness   *ptrSlider
	siren        *ptrCheck
	wdr          *ptrCheck // picture settings of some models (models.go)
	lowLight     *ptrCheck
	sound        *ptrCheck
	bars         []fyne.CanvasObject
	exitView     *fyne.Container // video-only overlay: exit/move buttons (top right), pan/tilt (bottom left)
	viewOnly     bool
	hideTimer    *time.Timer
	normalSize   fyne.Size // window size before video-only mode

	hist                                 history // SD card playback
	recButton                            *ptrButton
	ai                                   *aiEngine            // AI tasks (Settings → AI)
	points                               [presetSlots]*preset // camera points 1-3
	grid                                 bool                 // grid of all cameras
	tiles                                []*tile
	videoArea                            *fyne.Container
	gridButton                           *ptrButton
	caps                                 camCaps // features of the selected camera
	capPTZ, capPoints, capZoom, capLight fyne.CanvasObject
	capOverlayPTZ, capOverlayZoom        fyne.CanvasObject
	pointButtons                         [presetSlots]*ptrButton
	pointNames                           [presetSlots]*canvas.Text // the names under the numbers
	pointWraps                           [presetSlots]*fyne.Container
	gotFrame                             bool   // first frame of the current playback arrived
	decodeW                              int    // width the video is decoded to (tier, see decodeSize)
	zoomCam                              string // the camera the digital zoom belongs to
	decodeEnhanced                       bool   // the decoder sharpens/denoises (zoomed: enhance.go)
	decodeStable                         int    // seconds the window has needed another decode size

	camZoom    float64 // camera zoom 1.0-9.0
	zoomTimer  *time.Timer
	zoomDone   func() // ends the busy indicator of the pending zoom command
	zoomLabel  *widget.Label
	zoomSlider *zoomSlider // vertical, over the picture in video-only mode
	lens       *segmented  // wide | tele lens of a dual-lens camera (lens.go)

	// aiming in the picture, the overview of the wide lens, the way back (aimui.go, guard.go)
	aimMode   bool
	aimBtn    *ptrButton
	backBtn   *ptrButton // back to the watch view, with the time left
	backShown bool
	guard     *watchGuard
	wide      map[string]*wideView
	camPos    map[string]xccore.Position // where each camera looks (as last read or aimed)

	dragX, dragY float32      // accumulated mouse drag for camera moves
	mover        *mover       // pan/tilt steps
	current      atomic.Value // name of the selected camera (string), for background goroutines

	syncing bool // skip On* handlers while syncing controls with camera state
	visible bool
}

func main() {
	// video frames are large and short-lived: collect garbage more often to keep memory low
	debug.SetGCPercent(50)
	startHidden := false
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "mcp": // MCP server for AI assistants (stdio)
			if err := runMCP(); err != nil {
				fmt.Fprintln(os.Stderr, "xiaomi-cam mcp:", err)
				os.Exit(1)
			}
			return
		case "--tray": // start hidden in the tray (used by the MCP server)
			startHidden = true
		}
	}
	if showRunningInstance() {
		startDebug("another instance runs")
		return
	}
	prepareOpenGL() // Windows without a GPU driver: software OpenGL (gl_windows.go)
	raisePriority()
	probeGPU() // enhance.go, in the background
	if err := startBackend(); err != nil {
		fmt.Fprintln(os.Stderr, "xiaomi-cam:", err)
		os.Exit(1)
	}
	startDebug("backend started")

	loadRecordings() // SD card files found before (history.go)
	evStore.load()   // camera events read before (events.go)
	a := fyneapp.NewWithID("pl.lawinski.xiaomi-cam")
	a.SetIcon(appIcon)
	a.Settings().SetTheme(appTheme{})
	lang = a.Preferences().StringWithFallback(prefLanguage, defaultLanguage())
	appPrefs = a.Preferences()
	if appPrefs.Bool(prefPhonePage) {
		if err := setPhonePage(true); err != nil { // phone.go
			fmt.Fprintln(os.Stderr, "xiaomi-cam: phone page:", err)
		}
	}
	u := &ui{app: a, win: a.NewWindow("Xiaomi Cam")}
	u.current.Store("")
	u.caps = allCaps
	u.camZoom = 1
	u.mover = newMover(func() string {
		name := u.current.Load().(string)
		if u.guard != nil {
			u.guard.touch(name) // turning by hand: the way back to the watch view starts
		}
		return name
	}, func(err error) {
		fyne.Do(func() { u.status.SetText(TJ("Obrót: ", err.Error())) })
	})
	u.build()
	u.initAim()
	xccore.OnZoomState = func(name string, z float64) {
		fyne.Do(func() {
			if name == u.camera.Selected && z != u.camZoom {
				u.camZoom = z
				u.showZoom(z)
			}
		})
	}
	u.setupTray()
	onShowRequest(func() { fyne.Do(u.show) })
	go func() {
		for range time.Tick(time.Second) {
			fyne.Do(func() {
				u.historyStatus()
				u.updateRecordButton()
				u.saveGeometry()
				u.checkDecodeSize()
			})
		}
	}()
	a.Lifecycle().SetOnStarted(func() {
		onMinimize(u.win, u.hide)     // minimize = hide to tray
		onCursorLeave(u.win, func() { // the mouse left the window: hide the video-only buttons soon
			if u.viewOnly {
				u.hideOverlayAfter(time.Second)
			}
		})
		u.restoreGeometry(geomFramed)
		if a.Preferences().Bool(prefWallpaper) {
			time.AfterFunc(time.Second, func() { fyne.Do(func() { u.setWallpaper(true) }) })
		}
	})
	a.Lifecycle().SetOnStopped(func() {
		appStopped.Store(true) // late frames of the players don't go to the UI
		u.player.stop()
		for _, t := range u.tiles {
			t.player.stop()
		}
		stopAllRecordings()
	})

	u.win.Resize(fyne.NewSize(1100, 700))
	u.win.SetCloseIntercept(u.hide)
	onRecordingChanged = func(string) { fyne.Do(u.updateRecordButton) }
	onRecordingFailed = func(msg string) { fyne.Do(func() { dialog.ShowError(errors.New(msg), u.win) }) }
	pointsChanged = func() { fyne.Do(u.loadPoints) }
	u.ai = newAIEngine(aiActions{notify: func(title, text string) {
		a.SendNotification(fyne.NewNotification(title, text))
	}})
	initHTTPAPI(func(name string, z float64) {
		fyne.Do(func() {
			if name == u.camera.Selected {
				u.camZoom = z
				u.showZoom(z)
			}
		})
	})
	if !startHidden {
		u.show()
	}
	if u.startInGrid() {
		u.setGrid(true) // with 2 or more cameras
	}
	if len(cameraNames()) == 0 && !startHidden {
		showChooseCamera(u.win, u.selectCamera)
	}
	u.startEventWatcher()
	startONVIFEvents()
	startHangWatchdog()
	if devHook != nil {
		go devHook(u) // only in builds with -tags devtest
	}
	if path := os.Getenv("XIAOMI_CAM_SCREENSHOT"); path != "" {
		go u.screenshotAndQuit(path) // for development: window screenshot after 15 s
	}
	startDebug("UI starts")
	a.Run()
	startDebug("UI ended")
	saveRecordings()
	evStore.save()
	u.player.stop()
}

// appStopped - the UI has ended (Lifecycle OnStopped): fyne.Do must not be called any more
var appStopped atomic.Bool

func (u *ui) build() {
	u.video = newVideoView()
	u.video.onDoubleTap = u.toggleFullScreen
	u.video.onAimTap = u.aimTap
	u.video.onWideTap = u.aimAtWide
	u.video.onSecondary = func() {
		if u.viewOnly {
			u.setViewOnly(false)
		}
	}
	u.video.onMouseMove = u.pictureControlsActive
	u.onVideoMouse = u.pictureControlsActive
	u.video.onCameraDrag = u.cameraDrag
	u.video.onCameraDragEnd = u.cameraDragEnd
	u.video.onScroll = func(up bool, pos fyne.Position) {
		if up {
			u.zoomAt(zoomStep, &pos)
		} else {
			u.zoomAt(-zoomStep, &pos)
		}
	}
	u.zoomLabel = widget.NewLabel(fmt.Sprintf("%.1f×", u.camZoom))
	u.video.canResize = func() bool { return u.viewOnly && !u.win.FullScreen() }
	u.video.onResize = u.resizeKeepAspect
	u.video.onEdgePress = func(dir int) bool { return wmMoveResize(u.win, dir) }
	u.status = newStatusLabel()
	u.activity = newSpinner(spinnerFrames)
	u.player.onFrame = func(img *image.RGBA, gen int64) {
		if appStopped.Load() {
			return
		}
		fyne.Do(func() {
			if gen != u.player.gen.Load() {
				return // late frame of a stopped playback
			}
			if !u.gotFrame {
				u.gotFrame = true
				u.setLive(!u.hist.on)
				if !u.hist.on {
					u.status.SetText(fmt.Sprintf(T("Na żywo (%s)"), u.quality.Selected))
				}
			}
			u.historyFrame(gen)
			u.video.setFrame(img)
			u.captureThumb(img)
			u.pictureShown(&u.player, gen)
		})
	}
	u.player.onEnd = u.archiveEnded
	u.player.onStatus = func(s string) {
		fyne.Do(func() {
			if u.grid {
				return // a late status of the stopped single view (the grid has its own)
			}
			u.video.setStatus(s)
			u.status.SetText(s)
		})
	}

	u.camera = newSelect(cameraNames(), func(name string) {
		if !u.syncing {
			u.app.Preferences().SetString(prefLastCamera, name)
		}
		if !u.syncing && u.grid {
			u.selectTile(name)
			return
		}
		if !u.syncing {
			if u.hist.on {
				u.goLive() // history is per camera
			}
			u.restart()
			go u.loadCameraState()
			u.updateCaps()
			u.loadPoints()
		}
	})
	var cNames []string
	for _, c := range connections {
		cNames = append(cNames, T(c.name))
	}
	u.quality = newSegmented(func(string) {
		if u.syncing {
			return
		}
		if i := u.quality.SelectedIndex(); ipCam(u.camera.Selected) != nil {
			u.changeIPQuality(i)
		} else {
			u.changeSource(map[string]string{"subtype": xccore.CameraQualities(u.camera.Selected)[i].Subtype})
		}
	})
	u.connection = newSelect(cNames, func(string) {
		if !u.syncing {
			u.changeSource(map[string]string{"p2p": connections[u.connection.SelectedIndex()].p2p})
		}
	})

	u.buildHistory()
	u.gridButton = newButton("", theme.GridIcon(), func() { u.setGrid(!u.grid) })
	u.recButton = newButton(T("Nagraj"), recordIcon, u.toggleRecording)
	u.snapButton = newButton("", theme.MediaPhotoIcon(), u.snapshot).withTip(T("Zdjęcie w pełnej rozdzielczości do folderu zdjęć (S)"))
	u.enhButton = newButton("", lucide("scan"), u.enhancedSnapshot).withTip(T("Zdjęcie ulepszone (E): 15 klatek połączonych w jedno – mniej szumu, bez wymyślania szczegółów; przy zoomie także powiększony fragment"))
	// tooltips of the named controls
	u.camera.tip = T("Wybór kamery")
	u.quality.tip = T("Jakość obrazu z kamery: 2.5K, 720p lub SD (kamera IP: strumień główny lub podgląd)")
	u.connection.tip = T("Połączenie: zdalne przez serwery Xiaomi (z każdej sieci) lub lokalne w sieci LAN")
	u.gridButton.tip = T("Siatka wszystkich kamer / jedna kamera (w siatce: kliknij kamerę, aby nią sterować; podwójne kliknięcie – pełny widok)")
	u.recButton.tip = T("Nagrywanie do MP4 – start/stop (tryb i kamery w Ustawieniach)")
	u.hist.button.tip = T("Historia nagrań z karty SD / powrót na żywo")
	// lists of a fixed width (the window can be narrow); the labels are in the tooltips
	fixed := func(w float32, o fyne.CanvasObject) fyne.CanvasObject {
		return container.NewGridWrap(fyne.NewSize(w, o.MinSize().Height), o)
	}
	u.connBox = fixed(100, u.connection)
	u.menuButton = u.appMenuButton()
	top := container.NewHBox(
		fixed(140, u.camera),
		u.quality,
		u.connBox,
		layout.NewSpacer(),
		// view: grid, history, snapshot, video only, full screen
		u.gridButton,
		u.hist.button,
		u.recButton,
		u.snapButton,
		u.enhButton,
		// app: settings; cameras, language, help in the menu
		newButton("", theme.SettingsIcon(), u.showSettings).withTip(T("Ustawienia: nagrywanie, foldery, zadania AI, powiadomienia")),
		u.menuButton,
	)

	ptz := func(icon fyne.Resource, op int) *holdButton {
		b := newHoldButton(icon, func() {
			u.pictureControlsActive()
			u.mover.Press(op)
		}, u.release).withTip(ptzTip(op))
		b.Importance = widget.LowImportance // on the pill's background
		return b
	}
	u.light = newCheck(T("Reflektor"), func(on bool) {
		u.win.Canvas().Unfocus()
		if !u.syncing {
			u.setProp(propLight, on, u.light)
		}
	})
	u.brightness = newSlider(0, 100)
	u.brightness.Step = 10
	u.brightness.OnChangeEnded = func(v float64) {
		u.win.Canvas().Unfocus()
		if !u.syncing {
			u.setProp(propBrightness, int(v), u.brightness)
		}
	}
	u.siren = newCheck(T("Syrena"), func(on bool) {
		u.win.Canvas().Unfocus()
		if u.syncing {
			return
		}
		if !on {
			u.setProp(propSiren, false, u.siren)
			return
		}
		dialog.ShowConfirm(T("Syrena"), T("Włączyć syrenę alarmową kamery?"), func(ok bool) {
			if ok {
				u.setProp(propSiren, true, u.siren)
			} else {
				u.syncing = true
				u.siren.SetChecked(false)
				u.syncing = false
			}
		}, u.win)
	})
	u.wdr = newCheck(T("WDR – jasne niebo i cień jednocześnie"), func(on bool) {
		u.win.Canvas().Unfocus()
		if !u.syncing {
			u.setProp(propWDR, on, u.wdr)
		}
	})
	u.lowLight = newCheck(T("Kolor przy słabym świetle"), func(on bool) {
		u.win.Canvas().Unfocus()
		if !u.syncing {
			u.setProp(propLowLightColor, on, u.lowLight)
		}
	})
	u.night = newSelect(nil, func(string) {
		u.win.Canvas().Unfocus()
		if i := u.night.SelectedIndex(); !u.syncing && i >= 0 && i < len(u.nightList) {
			u.setNight(u.nightList[i].Value)
		}
	})
	u.setNightList(nightModes)
	u.night.PlaceHolder = T("Tryb nocny")
	u.night.tip = T("Tryb nocny: podczerwień (czarno-biały obraz) lub kolor z reflektorem; auto przełącza o zmroku")
	u.capNight = container.NewBorder(nil, nil, widget.NewIcon(moonIcon), nil, container.NewGridWrap(fyne.NewSize(230, u.night.MinSize().Height), u.night))
	u.sound = newCheck("", func(bool) {
		u.win.Canvas().Unfocus()
		if !u.syncing {
			p := &u.player
			if t := u.tileOf(u.camera.Selected); u.grid && t != nil {
				p = &t.player
			}
			u.busyUntilNewPicture(p, u.sound)
			u.refresh()
		}
	})

	u.light.tip = T("Reflektor kamery (białe światło)")
	u.brightness.tip = T("Jasność reflektora")
	u.siren.tip = T("Syrena alarmowa kamery (z potwierdzeniem)")
	u.wdr.tip = T("Szeroki zakres dynamiki: jasne niebo nie jest przepalone, a cienie nie są czarne")
	u.lowLight.tip = T("Kolorowy obraz o zmierzchu i przy słabym świetle, bez reflektora (w ciemności nadal podczerwień)")
	u.sound.tip = T("Dźwięk z kamery")
	// groups of controls on the picture (overlay.go), hidden when the selected camera doesn't
	// have the feature (caps.go)
	pad := [4]fyne.CanvasObject{
		ptz(theme.NavigateBackIcon(), motorLeft),
		ptz(theme.MoveUpIcon(), motorUp),
		ptz(theme.MoveDownIcon(), motorDown),
		ptz(theme.NavigateNextIcon(), motorRight),
	}
	zoom := container.NewHBox(
		overlayButton(theme.ZoomOutIcon(), T("Oddal (−, kółko myszy)"), func() { u.zoomBy(-zoomStep) }),
		u.zoomLabel,
		overlayButton(theme.ZoomInIcon(), T("Przybliż (+, kółko myszy): zoom kamery, potem cyfrowy z pełnej rozdzielczości; przeciągnij obraz, żeby się po nim przesuwać"), func() { u.zoomBy(zoomStep) }),
		overlayButton(theme.ZoomFitIcon(), T("Zoom 1× (0)"), u.zoomReset),
		container.NewCenter(u.buildLens()),
		u.buildAimButtons())
	u.capLight = container.NewBorder(nil, nil, u.light, nil, u.brightness)
	u.camControls = u.cameraControlsButton()
	u.camControls.Importance = widget.LowImportance
	u.soundBox = container.NewHBox(widget.NewIcon(theme.VolumeUpIcon()), u.sound)
	u.pictureControls = u.buildPictureControls(pad, u.buildPoints(), zoom)
	bottomBox := container.NewVBox(u.hist.bar)
	u.bars = []fyne.CanvasObject{top, bottomBox}
	exitBtn := newButton(T("Pokaż panele"), theme.VisibilityIcon(), func() { u.setViewOnly(false) }).withTip(T("Wróć do widoku z panelami i ramką okna (Esc, prawy przycisk myszy)"))
	exitBtn.Importance = widget.LowImportance
	moveH := newMoveHandle(u.win, widget.LowImportance)
	moveH.tip = T("Przesuń okno – przeciągnij")
	// button lies on the picture, on a translucent background
	exitBox := container.NewStack(canvas.NewRectangle(overlayBg),
		container.NewHBox(moveH, exitBtn))
	// pan/tilt buttons for video-only mode (dragging the picture doesn't move the camera there);
	// pressing keeps the overlay visible
	overlayPTZ := func(icon fyne.Resource, op int) *holdButton {
		b := newHoldButton(icon, func() {
			u.showExitButton()
			u.mover.Press(op)
		}, u.release)
		b.Importance = widget.LowImportance // no button background, only the light overlay one
		return b.withTip(ptzTip(op))
	}
	ptzBox := container.NewStack(canvas.NewRectangle(overlayBg), container.NewHBox(
		overlayPTZ(theme.NavigateBackIcon(), motorLeft),
		overlayPTZ(theme.MoveUpIcon(), motorUp),
		overlayPTZ(theme.MoveDownIcon(), motorDown),
		overlayPTZ(theme.NavigateNextIcon(), motorRight),
	))
	u.zoomSlider = newZoomSlider(1, maxZoom, zoomStep)
	u.zoomSlider.tip = T("Zoom kamery – przeciągnij lub kliknij")
	u.zoomSlider.SetValue(u.camZoom)
	u.zoomSlider.onChanged = func(v float64) {
		u.showExitButton()
		u.zoomLabel.SetText(fmt.Sprintf("%.1f×", v))
	}
	u.zoomSlider.onChangeEnded = func(v float64) {
		u.showExitButton()
		u.setZoom(snapTele(v, u.caps.Tele))
	}
	zoomBox := container.NewStack(canvas.NewRectangle(overlayBg),
		container.New(layout.NewCustomPaddedLayout(4, 4, 4, 8), u.zoomSlider))
	u.capOverlayPTZ, u.capOverlayZoom = ptzBox, zoomBox
	u.exitView = container.NewBorder(
		container.New(layout.NewCustomPaddedLayout(10, 0, 0, 10),
			container.NewHBox(layout.NewSpacer(), exitBox)), // 10 px from top and right edge
		container.New(layout.NewCustomPaddedLayout(0, 10, 10, 0),
			container.NewHBox(ptzBox, layout.NewSpacer())), // 10 px from bottom and left edge
		nil,
		container.New(layout.NewCustomPaddedLayout(0, 0, 0, 10), // 10 px from right edge, centered
			container.NewVBox(layout.NewSpacer(), zoomBox, layout.NewSpacer())))
	u.exitView.Hide()
	u.buildAreaEditor()
	u.videoArea = container.NewStack(u.videoAreaObjects(u.video)...)
	u.updateGridButton()
	u.win.SetContent(container.NewBorder(top, bottomBox, nil, nil, u.videoArea))

	u.win.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		switch e.Name {
		case fyne.KeyF11:
			u.toggleFullScreen()
		case fyne.KeyEscape:
			if u.aimMode {
				u.setAimMode(false)
				u.status.SetText("")
			} else if u.win.FullScreen() {
				u.toggleFullScreen()
			} else if u.viewOnly {
				u.setViewOnly(false)
			}
		}
	})
	// arrows move the camera while held (key down - move, key up - stop)
	arrows := map[fyne.KeyName]int{fyne.KeyLeft: motorLeft, fyne.KeyRight: motorRight, fyne.KeyUp: motorUp, fyne.KeyDown: motorDown}
	if dc, ok := u.win.Canvas().(desktop.Canvas); ok {
		dc.SetOnKeyDown(func(e *fyne.KeyEvent) {
			if u.hist.on && u.hist.pc && (e.Name == fyne.KeyLeft || e.Name == fyne.KeyRight) {
				d := 10 * time.Second // the PC archive: the arrows move the playback
				if e.Name == fyne.KeyLeft {
					d = -d
				}
				u.seekArchive(d)
				return
			}
			if op, ok := arrows[e.Name]; ok && u.caps.PTZ {
				u.mover.Press(op)
			}
		})
		dc.SetOnKeyUp(func(e *fyne.KeyEvent) {
			if _, ok := arrows[e.Name]; ok {
				u.release()
			}
		})
	}
	u.win.Canvas().SetOnTypedRune(func(r rune) {
		switch r {
		case '+', '=':
			u.zoomBy(zoomStep)
		case '-':
			u.zoomBy(-zoomStep)
		case '0':
			u.zoomReset()
		case '1', '2', '3':
			u.pointPressed(int(r - '0'))
		case 'h', 'H':
			u.setViewOnly(!u.viewOnly)
		case 'f', 'F':
			u.toggleFullScreen()
		case 's', 'S':
			u.snapshot()
		case 'e', 'E':
			u.enhancedSnapshot()
		case ' ':
			u.toggleArchivePause()
		}
	})

	if names := cameraNames(); len(names) > 0 {
		first := names[0]
		if last := u.app.Preferences().String(prefLastCamera); cameraExists(last) {
			first = last
		}
		u.syncing = true
		u.camera.SetSelected(first)
		u.syncing = false
		go u.loadCameraState()
		u.updateCaps()
		u.loadPoints()
	}
}

func (u *ui) setupTray() {
	desk, ok := u.app.(desktop.App)
	if !ok {
		return
	}
	show := fyne.NewMenuItem(TN("Pokaż"), u.show)
	hide := fyne.NewMenuItem(TN("Ukryj do zasobnika"), u.hide)
	u.wallpaperItem = fyne.NewMenuItem(TN("Tryb tapety (obraz w tle pulpitu)"), func() { u.setWallpaper(!u.wallpaper) })
	quit := fyne.NewMenuItem(TN("Zakończ"), u.app.Quit)
	quit.IsQuit = true
	u.trayMenu = fyne.NewMenu("Xiaomi Cam", show, hide, u.wallpaperItem, fyne.NewMenuItemSeparator(), quit)
	desk.SetSystemTrayMenu(u.trayMenu)
	desk.SetSystemTrayIcon(appIcon)
}

// hide closes the window to tray and stops video, so the camera is not streaming in the background.
func (u *ui) hide() {
	if u.remapping {
		return // not a minimize: the wallpaper mode re-maps the window
	}
	if u.wallpaper {
		u.leaveWallpaperHidden()
	}
	u.saveGeometry()
	u.visible = false
	u.win.Hide()
	u.player.stop()
	u.stopTiles()
}

func (u *ui) show() {
	if u.wallpaper {
		u.setWallpaper(false) // "Show" brings the normal window back
		return
	}
	u.visible = true
	u.win.Show()
	u.restoreGeometry(u.geometryKey()) // the WM may place a re-shown window elsewhere
	if win := glfwWindow(u.win); win != nil && win.GetAttrib(glfw.Iconified) == glfw.True {
		wmRestore(win) // it was minimized to tray
	}
	u.win.RequestFocus()
	if u.grid {
		for _, t := range u.tiles {
			u.startTile(t, false)
		}
		return
	}
	u.restart()
}

func (u *ui) selectCamera(name string) {
	u.camerasChanged()
	u.camera.SetSelected(name) // triggers restart
}

// restart (re)starts video of the selected camera with current settings.
func (u *ui) restart() {
	name := u.camera.Selected
	if !u.visible {
		return
	}
	if u.grid {
		if t := u.tileOf(name); t != nil {
			u.cameraSelected(name)
			u.startTile(t, false)
		}
		return
	}
	if name == "" {
		u.player.stop()
		u.video.clear(T("Brak kamery – kliknij „Dodaj”"), false)
		return
	}
	if name != u.zoomCam { // another camera: the whole picture again
		u.zoomCam = name
		u.video.setDigitalZoom(1, nil)
	}
	q := u.syncControls(name)
	u.gotFrame = false
	u.setLive(false)
	u.status.SetText(T("Łączenie z kamerą…"))
	u.video.clear(T("Łączenie z kamerą…"), true)
	w, h := u.decodeSize(q.w, q.h)
	u.decodeW = w
	u.setEnhance()
	go u.player.start(rtspURL(name), w, h, u.sound.Checked)
}

// syncControls sets the quality and connection lists from the camera source;
// returns the selected quality.
func (u *ui) syncControls(name string) quality {
	u.current.Store(name)
	src := cameraSource(name)
	u.syncing = true
	defer func() { u.syncing = false }()
	qs, qi := cameraQualities(name)
	var names []string
	for _, q := range qs {
		names = append(names, q.name)
	}
	u.quality.Options = names
	u.quality.SetSelectedIndex(qi)
	// connection: only Xiaomi cameras (servers / LAN)
	if src == "" {
		u.connBox.Hide()
		return qs[qi]
	}
	u.connBox.Show()
	ci := 1
	for i, c := range connections {
		if c.p2p == sourceParam(src, "p2p") {
			ci = i
		}
	}
	u.connection.SetSelectedIndex(ci)
	return qs[qi]
}

// refresh restarts the decoder with the current settings (sound, decode size) without
// reconnecting to the camera: the new decoder joins the same stream and replaces the old
// one when it has a picture.
func (u *ui) refresh() {
	name := u.camera.Selected
	if u.grid {
		if t := u.tileOf(name); t != nil && u.visible {
			u.startTile(t, true)
		}
		return
	}
	if !u.visible || name == "" || u.video.frame == nil {
		u.restart()
		return
	}
	q := currentQuality(name)
	w, h := u.decodeSize(q.w, q.h)
	u.decodeW = w
	u.setEnhance()
	go u.player.switchTo(rtspURL(name), w, h, u.sound.Checked)
}

func (u *ui) changeSource(params map[string]string) {
	name := u.camera.Selected
	if name == "" {
		return
	}
	u.switchSource(name, func() error { return setSourceParams(name, params) })
}

// changeIPQuality switches an IP camera to its main (0) or sub (1) stream.
func (u *ui) changeIPQuality(q int) {
	name := u.camera.Selected
	u.switchSource(name, func() error { return setIPQuality(name, q) })
}

// switchSource stops the players of the camera, changes its source off the UI thread (the
// change can wait on a camera that is slow to answer: the window froze) and restarts the
// video. The quality and connection controls are off meanwhile.
func (u *ui) switchSource(name string, change func() error) {
	u.player.stop() // close camera session, it will be redialed with new params
	if t := u.tileOf(name); t != nil {
		t.player.stop()
	}
	u.status.SetText(T("Łączenie z kamerą…"))
	done := u.busy(nil, u.quality, u.connection)
	go func() {
		time.Sleep(300 * time.Millisecond) // the camera closes the old session
		err := change()
		fyne.Do(func() {
			done()
			if err != nil {
				dialog.ShowError(err, u.win)
			}
			if name == u.camera.Selected {
				u.restart()
			}
		})
	}()
}

func (u *ui) loadCameraState() {
	name := u.camera.Selected
	if cameraSource(name) == "" && !hasImou(name) {
		u.loadIPState(name)
		return
	}
	props, err := getProps(name, propLight, propBrightness, propSiren, propNightShot, propWDR, propLowLightColor)
	if err != nil {
		fyne.Do(func() { u.status.SetText(TJ("Nie udało się odczytać stanu kamery: ", err.Error())) })
		return
	}
	fyne.Do(func() {
		u.syncing = true
		defer func() { u.syncing = false }()
		for _, p := range props {
			if p.Code != 0 {
				continue // the camera has no such property (Imou: no brightness)
			}
			switch {
			case p.Siid == propLight.Siid && p.Piid == propLight.Piid:
				on, _ := p.Value.(bool)
				u.light.SetChecked(on)
			case p.Siid == propBrightness.Siid && p.Piid == propBrightness.Piid:
				v, _ := p.Value.(float64)
				u.brightness.SetValue(v)
			case p.Siid == propSiren.Siid && p.Piid == propSiren.Piid:
				on, _ := p.Value.(bool)
				u.siren.SetChecked(on)
			case p.Same(propWDR):
				on, _ := p.Value.(bool)
				u.wdr.SetChecked(on)
			case p.Same(propLowLightColor):
				on, _ := p.Value.(bool)
				u.lowLight.SetChecked(on)
			case p.Siid == propNightShot.Siid && p.Piid == propNightShot.Piid && p.Code == 0:
				v, _ := p.Value.(float64)
				u.showNight(int(v))
			}
		}
	})
}

// loadIPState reads the state of an IP camera: the night mode (ONVIF IR cut filter).
func (u *ui) loadIPState(name string) {
	if name == "" || len(cameraNightModes(name)) == 0 {
		return
	}
	v, err := getNightMode(name)
	if err != nil {
		return
	}
	fyne.Do(func() {
		if name == u.camera.Selected {
			u.syncing = true
			u.showNight(v)
			u.syncing = false
		}
	})
}

// setNightList sets the night modes the selected camera has as the options.
func (u *ui) setNightList(list []nightMode) {
	if slices.Equal(list, u.nightList) && u.night.Options != nil {
		return // the same modes: keep the selection
	}
	u.nightList = list
	names := make([]string, 0, len(list))
	for _, m := range list {
		names = append(names, T(m.Name))
	}
	u.syncing = true
	u.night.ClearSelected()
	u.night.SetOptions(names)
	u.syncing = false
}

// showNight selects the night mode with the value (u.syncing: set by the caller).
func (u *ui) showNight(v int) {
	for i, m := range u.nightList {
		if m.Value == v {
			u.night.SetSelectedIndex(i)
			return
		}
	}
}

// setNight sets the night mode of the selected camera; the list is disabled until it answers.
func (u *ui) setNight(value int) {
	name := u.camera.Selected
	done := u.busy(nil, u.night)
	go func() {
		if err := setNightMode(name, value); err != nil {
			fyne.Do(func() { dialog.ShowError(err, u.win) })
		}
		u.loadCameraState()
		fyne.Do(done)
	}()
}

// setProp sets a camera property; the control w is disabled until the camera answers.
func (u *ui) setProp(p miotProp, value any, w fyne.Disableable) {
	name := u.camera.Selected
	done := u.busy(nil, w)
	go func() {
		err := setProp(name, p, value)
		if err != nil {
			fyne.Do(func() { dialog.ShowError(err, u.win) })
		}
		u.loadCameraState()
		fyne.Do(done)
	}()
}

// snapshot saves a full resolution JPEG from the camera stream (the window shows a
// downscaled picture) to the snapshots folder from the settings.
// enhancedSnapshot saves the enhanced photo (photo.go): the merged frames, the zoomed part
// enlarged from the full resolution and, with Real-ESRGAN installed, its AI version.
func (u *ui) enhancedSnapshot() {
	name := u.camera.Selected
	if name == "" || u.enhButton.isBusy() {
		return
	}
	var crop *[4]float64
	if f := u.video.frame; f != nil && u.video.zoomed() {
		b := f.Bounds()
		r := u.video.dz.cropRect(b.Dx(), b.Dy())
		crop = &[4]float64{float64(r.Min.X) / float64(b.Dx()), float64(r.Min.Y) / float64(b.Dy()),
			float64(r.Dx()) / float64(b.Dx()), float64(r.Dy()) / float64(b.Dy())}
	}
	u.status.SetText(T("Łączę 15 klatek w ulepszone zdjęcie…"))
	done := u.busy(u.enhButton)
	go func() {
		files, err := enhancedPhoto(name, crop)
		if err == nil && crop != nil && hasAIEnhance() {
			fyne.Do(func() { u.status.SetText(T("Powiększanie AI (Real-ESRGAN)…")) })
			if ai, aerr := aiEnhance(files[len(files)-1]); aerr == nil {
				files = append(files, ai)
			} else {
				err = aerr
			}
		}
		fyne.Do(func() {
			done()
			if err != nil {
				dialog.ShowError(err, u.win)
			}
			if len(files) > 0 {
				names := make([]string, len(files))
				for i, f := range files {
					names[i] = filepath.Base(f)
				}
				u.status.SetText(TJ("Zapisano ", strings.Join(names, ", ")))
			}
		})
	}()
}

func (u *ui) snapshot() {
	name := u.camera.Selected
	if name == "" || u.snapButton.isBusy() {
		return
	}
	u.status.SetText(T("Zapisuję zdjęcie…"))
	done := u.busy(u.snapButton)
	go func() {
		path, err := saveSnapshot(name)
		fyne.Do(func() {
			done()
			if err != nil {
				dialog.ShowError(err, u.win)
				return
			}
			u.status.SetText(TJ("Zapisano ", path))
		})
	}()
}

func (u *ui) toggleFullScreen() {
	if u.win.FullScreen() {
		u.setViewOnly(false)
		return
	}
	u.viewOnly = true
	u.showBars(false)
	u.win.SetPadded(false)
	u.showExitButton()
	u.win.SetFullScreen(true)
}

// setViewOnly shows only the camera picture: no panels and no window frame, the window
// gets the picture's aspect ratio (no black bars). Exit: Esc, H, right click or
// "Pokaż panele" button shown on the picture when the mouse moves.
func (u *ui) setViewOnly(on bool) {
	if on == u.viewOnly {
		return
	}
	u.saveGeometry() // of the mode we are leaving
	u.notMinimized() // the frame on/off re-maps the window
	u.viewOnly = on
	u.showBars(!on)
	if on {
		u.pictureControls.Hide()
	} else {
		u.pictureControls.Show()
	}
	u.win.SetPadded(!on)
	if on {
		u.normalSize = u.win.Canvas().Size()
		setDecorated(u.win, false)
		if !u.restoreGeometry(geomFrameless) {
			aspect := float32(9) / 16
			if f := u.video.frame; f != nil {
				aspect = float32(f.Bounds().Dy()) / float32(f.Bounds().Dx())
			}
			w := u.normalSize.Width
			u.win.Resize(fyne.NewSize(w, w*aspect))
		}
		if f := u.video.frame; f != nil {
			setAspectRatio(u.win, f.Bounds().Dx(), f.Bounds().Dy())
		} else {
			setAspectRatio(u.win, 16, 9)
		}
		u.showExitButton()
		return
	}
	u.exitView.Hide()
	if u.win.FullScreen() {
		u.win.SetFullScreen(false)
	}
	setDecorated(u.win, true)
	setAspectRatio(u.win, 0, 0)
	if !u.restoreGeometry(geomFramed) && !u.normalSize.IsZero() {
		u.win.Resize(u.normalSize)
	}
	u.normalSize = fyne.Size{}
}

func (u *ui) showBars(on bool) {
	for _, b := range u.bars {
		if on {
			b.Show()
		} else {
			b.Hide()
		}
	}
}

func (u *ui) showExitButton() {
	if !u.viewOnly {
		return
	}
	u.exitView.Show()
	u.hideOverlayAfter(3 * time.Second) // no mouse movement inside the window
}

// hideOverlayAfter hides the video-only buttons after d (restarted on every call).
func (u *ui) hideOverlayAfter(d time.Duration) {
	if u.hideTimer != nil {
		u.hideTimer.Stop()
	}
	u.hideTimer = time.AfterFunc(d, func() {
		fyne.Do(func() { u.exitView.Hide() })
	})
}

// cameraDrag turns the camera when the picture is dragged: like grabbing the view,
// dragging right turns the camera left. The camera moves while the mouse keeps dragging
// in that direction and stops on release (cameraDragEnd).
func (u *ui) cameraDrag(dx, dy float32) {
	if !u.caps.PTZ {
		return
	}
	if u.viewOnly {
		return // video-only mode has pan/tilt buttons instead
	}
	const threshold = 15 // px before the camera starts moving
	u.dragX += dx
	u.dragY += dy
	op := 0
	switch {
	case u.dragX >= threshold && u.dragX >= abs32(u.dragY):
		op = motorLeft
	case u.dragX <= -threshold && -u.dragX >= abs32(u.dragY):
		op = motorRight
	case u.dragY >= threshold:
		op = motorUp
	case u.dragY <= -threshold:
		op = motorDown
	}
	if op != 0 {
		u.mover.Press(op)
	}
}

func (u *ui) cameraDragEnd() {
	u.dragX, u.dragY = 0, 0
	u.release()
}

// release stops the camera turned by hand and reads where it stopped (the overview frame).
func (u *ui) release() {
	u.mover.Release()
	u.refreshPosition(u.camera.Selected, 1500*time.Millisecond)
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// setLanguage switches the app language (menu).
func (u *ui) setLanguage(code string) {
	{
		if code == lang {
			return
		}
		lang = code
		u.app.Preferences().SetString(prefLanguage, code)
		// rebuild UI with new texts; live video keeps playing (the new view gets its frames)
		wasGrid := u.grid
		if wasGrid {
			u.stopTiles()
			u.tiles, u.grid = nil, false
		}
		wasHistory := u.hist.on
		if wasHistory {
			u.player.stop()
			_ = setPlayback(u.current.Load().(string), 0, 0)
		}
		sound := u.sound.Checked
		frame := u.video.frame
		u.build()
		u.syncing = true
		u.sound.SetChecked(sound)
		u.syncing = false
		u.setupTray()
		if wasGrid {
			u.setGrid(true)
		} else if wasHistory || frame == nil {
			u.restart()
		} else {
			u.syncControls(u.camera.Selected)
			u.video.setFrame(frame)
			u.gotFrame = true
			u.status.SetText(fmt.Sprintf(T("Na żywo (%s)"), u.quality.Selected))
		}
		go u.loadCameraState()
		u.updateCaps()
		u.loadPoints()
	}
}

func (u *ui) showShortcuts() {
	text := T(`Strzałki – obrót kamery
Przeciąganie obrazu myszą – obrót kamery
+ / − (także na klawiaturze numerycznej), kółko myszy – zoom kamery (do 9×)
0 – zoom 1×
1, 2, 3 – punkty kamery (zapisane pozycje)
H – tylko obraz (ukrywa panele), prawy przycisk myszy lub Esc – powrót
F, F11, podwójne kliknięcie – pełny ekran
S – zdjęcie do ~/Obrazy
Zamknięcie okna chowa aplikację do zasobnika`)
	dialog.ShowInformation(T("Skróty klawiaturowe"), text+"\n\nXiaomi Cam "+version, u.win)
}

func (u *ui) screenshotAndQuit(path string) {
	time.Sleep(15 * time.Second)
	fyne.Do(func() {
		if f, err := os.Create(path); err == nil {
			_ = jpeg.Encode(f, u.win.Canvas().Capture(), nil)
			_ = f.Close()
		}
		u.app.Quit()
	})
}

// resizeKeepAspect resizes the frameless window by a dragged edge, keeping the picture
// aspect ratio, so there are no black bars.
func (u *ui) resizeKeepAspect(dx, dy float32) {
	aspect := float32(9) / 16
	if f := u.video.frame; f != nil {
		aspect = float32(f.Bounds().Dy()) / float32(f.Bounds().Dx())
	}
	size := u.win.Canvas().Size()
	w := size.Width + dx
	if dx == 0 || (dy != 0 && dy/aspect > dx) {
		w = size.Width + dy/aspect
	}
	w = max(w, 320)
	u.win.Resize(fyne.NewSize(w, w*aspect))
}

const (
	zoomStep = 0.5
	maxZoom  = 9.0 // hlCameraVideoMaxZoomScale in the Mi Home plugin
)

// zoomBy zooms in (delta > 0) or out around the centre of the picture.
func (u *ui) zoomBy(delta float64) { u.zoomAt(delta, nil) }

// zoomAt: the camera's own zoom first (sharp: the camera does it), then the digital zoom
// on top of it; zooming out undoes them in the reverse order. pos - keep this point of the
// picture under the mouse (wheel). Only the last camera value is sent after a short pause,
// so scrolling the wheel doesn't flood the camera with commands.
func (u *ui) zoomAt(delta float64, pos *fyne.Position) {
	if u.grid { // the tiles: the camera's own zoom only
		if u.caps.Zoom {
			u.setZoom(nextZoom(u.camZoom, delta, u.caps.Tele))
		}
		return
	}
	dz := u.video.dz.z
	if u.caps.Zoom && ((delta > 0 && u.camZoom < maxZoom) || (delta < 0 && dz <= 1.001)) {
		u.setZoom(nextZoom(u.camZoom, delta, u.caps.Tele))
		return
	}
	limit := maxDigital
	if u.caps.Zoom {
		limit = maxDigitalHybrid
	}
	if delta > 0 {
		dz *= digitalStep
	} else {
		dz /= digitalStep
	}
	if dz < 1.05 {
		dz = 1
	}
	u.video.setDigitalZoom(min(dz, limit), pos)
	u.showZoom(u.camZoom)
	u.decodeStable = 1 // a sharper decode tier soon (checkDecodeSize)
}

// setEnhance: the next decoder sharpens and denoises while the digital zoom is on.
func (u *ui) setEnhance() {
	u.decodeEnhanced = u.video.zoomed()
	u.player.enhance.Store(u.decodeEnhanced)
}

// zoomReset: back to the whole picture (camera zoom 1x, no digital zoom).
func (u *ui) zoomReset() {
	u.video.setDigitalZoom(1, nil)
	if u.caps.Zoom {
		u.setZoom(1)
	} else {
		u.showZoom(1)
	}
}

// setZoom sets the camera zoom; only the last value is sent after a short pause.
func (u *ui) setZoom(z float64) {
	u.camZoom = min(max(z, 1), maxZoom)
	u.showZoom(u.camZoom)
	if u.zoomTimer != nil && u.zoomTimer.Stop() {
		u.zoomDone() // the previous value wasn't sent
	}
	name, zoom := u.camera.Selected, u.camZoom
	if u.guard != nil {
		u.guard.touch(name)
	}
	setZoomState(name, zoom)
	done := u.busy(nil)
	u.zoomDone = done
	u.zoomTimer = time.AfterFunc(300*time.Millisecond, func() {
		err := cameraZoom(name, zoom)
		fyne.Do(func() {
			done()
			if err != nil {
				u.status.SetText("Zoom: " + err.Error())
			}
		})
	})
}

// showZoom updates the zoom label (the whole zoom: camera × digital) and the video-only
// zoom slider (the camera's zoom).
func (u *ui) showZoom(z float64) {
	if dz := u.video.dz.z; dz > 1.001 {
		u.zoomLabel.SetText(TJ("cyfrowy ", fmt.Sprintf("%.1f×", z*dz)))
	} else {
		u.zoomLabel.SetText(fmt.Sprintf("%.1f×", z))
	}
	if u.zoomSlider != nil {
		u.zoomSlider.SetValue(z)
	}
	u.showLens(z)
	u.showWide()
}

// decodeSize returns the size the video is decoded to: the width of the picture on the
// screen in pixels (rounded up to 64), limited by the stream size w x h. Decoding to the
// size shown instead of the full 2560x1440 saves most of CPU and memory (camera zoom is done
// by the camera, so no detail is lost); ffmpeg scales with a good filter and the GPU then
// shows the frame 1:1 (videoView.setScaleMode).
func (u *ui) decodeSize(w, h int) (int, int) {
	// zoomed in digitally: the crop has to be as sharp as the picture, so a larger frame
	_, _, sw, _ := u.video.shownRect()
	px := int(sw * u.win.Canvas().Scale() * float32(u.video.dz.z))
	if px <= 0 {
		px = 1280
	}
	dw := min((px+63)/64*64, w)
	return dw, dw * h / w &^ 1
}

// decodeOff reports whether a decoder of width have is too far from the width wanted: too
// small (a blurred picture) or more than 1.25x larger (CPU, and the GPU's bilinear scaling).
func decodeOff(have, want int) bool {
	return have < want || have*4 > want*5
}

// checkDecodeSize restarts live video when the window was resized to another tier
// (e.g. full screen); called every second.
func (u *ui) checkDecodeSize() {
	if u.grid {
		return // tiles have a fixed decode size
	}
	if !u.visible || u.hist.on || u.decodeW == 0 || u.camera.Selected == "" {
		return
	}
	q := currentQuality(u.camera.Selected)
	if w, _ := u.decodeSize(q.w, q.h); decodeOff(u.decodeW, w) || u.video.zoomed() != u.decodeEnhanced {
		u.decodeStable++
		if u.decodeStable >= 2 { // the same new size for 2 s - resizing has finished
			u.decodeStable = 0
			u.refresh()
		}
		return
	}
	u.decodeStable = 0
}

func ptzTip(op int) string {
	switch op {
	case motorLeft:
		return T("Obróć w lewo – przytrzymaj (klawisz ←)")
	case motorRight:
		return T("Obróć w prawo – przytrzymaj (klawisz →)")
	case motorUp:
		return T("Obróć w górę – przytrzymaj (klawisz ↑)")
	default:
		return T("Obróć w dół – przytrzymaj (klawisz ↓)")
	}
}
