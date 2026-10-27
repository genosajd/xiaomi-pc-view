# Xiaomi Cam (xiaomi-pc-view) – notes for Claude

Desktop viewer/controller for Xiaomi Mi Home cameras on Linux (user's camera: Xiaomi Outdoor
Camera CW700S, model `isa.camera.700sa`). Repo = fork of go2rtc v1.9.14 + app in
`cmd/xiaomi-cam`. GitHub: `git@github.com:genosajd/xiaomi-pc-view.git` (branch `main`, tags
`v1.0.0`…`v1.22.0`). User talks Polish; README/release notes in English.

## Build, install, release

```sh
make install VERSION=vX.Y.Z   # ~/.local/bin/xiaomi-cam + menu entry + hicolor icon
make deb     VERSION=vX.Y.Z   # dist/xiaomi-cam_X.Y.Z_amd64.deb
make release VERSION=vX.Y.Z   # dist/: .deb, plain binary, .tar.gz
go vet ./cmd/xiaomi-cam && go vet -tags devtest ./cmd/xiaomi-cam && go test ./cmd/xiaomi-cam/
```

Workflow per change: commit (message ends with `Co-Authored-By: Claude …` trailer), tag
`vX.Y.Z`, `make install` + `make release`, `git push origin main --tags`, relaunch the app:
`(setsid nohup ~/.local/bin/xiaomi-cam >/dev/null 2>&1 &)`.

GitHub releases: v1.0.0–v1.33.0 published; from v1.23.0 also the APKs (`make apk`: universal, arm64-v8a, armeabi-v7a) (gh logged in as genosajd). For a new version:
`gh release create vX.Y.Z --title "Xiaomi Cam vX.Y.Z" --notes-file <notes> --latest dist/<3 files>`;
notes must list the changes since the previous tag (English) and the 3 downloads.

## Windows version (from v1.24.0)

`make windows VERSION=vX.Y.Z` → `dist/xiaomi-cam-X.Y.Z-windows-setup.exe` (cross build with
llvm-mingw in `~/Windows`, go-winres icon/manifest, ffmpeg BtbN 8.1 GPL shared, Mesa
26.1.8 in `softgl\` with a copy of the exe, `glprobe\xc-glprobe.exe`, Inno Setup 6.7.3 in the
Wine prefix `~/Windows/wine`). Windows code: `wm_windows.go` (move/resize via
WM_NCLBUTTONDOWN, wallpaper: Win11 24H2 = layered child of Progman below SHELLDLL_DefView;
not shown in the VM with Mesa/GDI, untested with a GPU), `gl_windows.go` (no OpenGL 2.1 →
start `softgl\xiaomi-cam.exe` with GALLIUM_DRIVER=softpipe; llvmpipe exits 1, copying Mesa
at runtime failed – Defender), `autostart_windows.go` (HKCU Run), `proc_windows.go`,
`tools.go` (ffmpeg from the app folder, no console windows). `XC_STARTDEBUG=1` prints start
stages to stderr. Test VM: libvirt `win11` (Windows 11 24H2 PL, no GPU, user `genos`; the user
logs in, never type the password); drive it with virsh screenshot + QMP send-key /
input-send-event (usb tablet), files via a temporary `python3 -m http.server --bind
192.168.122.1` + `curl` in cmd (PowerShell is blocked there). Clean up after tests (uninstall,
`%APPDATA%\xiaomi-cam-viewer`, `%APPDATA%\fyne\pl.lawinski.xiaomi-cam`, test files) and shut
the VM down. Release: add the setup.exe to the GitHub release.

## Self-tests with the real camera

Build with `-tags devtest`; `cmd/xiaomi-cam/devtest_*.go` hooks run when env `XC_*` is set
(e.g. `XC_TEST`, `XC_PTZTEST`, `XC_GRIDTEST`, `XC_AITEST`, `XC_SEAMLESSTEST`, `XC_BUSYTEST`,
`XC_NIGHTTEST`, `XC_WALLTEST` + `XC_WALLPOS`/`XC_WALLSECS`/`XC_LOWERMS`, `XC_IPTEST` + `XC_SIMHOST`, `XC_IPUITEST`, `XC_ADDCAM="name|url;…"` adds IP cameras to the real config). XTest mouse input doesn't reach the window (GNOME Wayland,
XWayland), so click behaviour is tested with the Fyne test driver (`tooltip_test.go`). Real screenshots
(whole desktop, no prompt): xdg-desktop-portal Screenshot with interactive=false via python gi. Real mouse clicks on
the desktop: a virtual absolute pointer via /dev/uinput (python evdev, the user has an ACL) –
only on a monitor area without the user's windows. IP cameras without a real one: `cmd/xc-camsim` (go2rtc with ffmpeg testsrc: RTSP with a
login, ONVIF, MJPEG); run it as `xc-camsim -rtsp :28554 -http 10.0.3.1:21984` (its API runs
commands: never on the LAN; RTSP needs a login only from non-loopback, so test via
10.0.3.1) and the test with `XC_SIMHOST=10.0.3.1 XC_IPTEST=<dir>`; back up and restore
`~/.config/xiaomi-cam-viewer/go2rtc.yaml` around it. The app is
single-instance (ports 127.0.0.1:11984 API, :18554 RTSP), so stop the user's running copy
first with an **anchored** pattern (unanchored pkill kills your own shell):
`pkill -f "^/home/genosajd/(.local/bin|xiaomi-pc-view)/xiaomi-cam( --tray)?$"`. Back up and restore
`~/.config/fyne/pl.lawinski.xiaomi-cam/preferences.json` around tests (window geometry,
language, layout). Relaunch the user's app afterwards. Clean up test files in ~/Videos,
~/Pictures and test camera points.

## Architecture (cmd/xiaomi-cam)

- `main.go` UI (Fyne 2.8), `backend.go` embedded go2rtc (api, streams, rtsp, xiaomi), config
  `~/.config/xiaomi-cam-viewer/go2rtc.yaml` (Mi token, 0600)
- `video.go` ffmpeg → RGBA frames, player with seamless `switchTo` (no camera reconnect)
- `control.go` mover (PTZ), custom widgets with pointer cursor + tooltips (`tooltip.go`),
  GLFW access via reflection (frame, minimize, cursor leave), `wm_x11.go` (_NET_WM_MOVERESIZE),
  desktop window type + empty XShape input for `wallpaper.go`; no _NET_WM_STATE_BELOW, it
  is mutter's BOTTOM layer above the desktop icons (DING); `keepBelow` re-lowers the
  window under DING every 10 ms from its own X connection, because DING lowers itself on
  every window map and every click on the desktop, and the icons blinked), `busy.go` spinners on buttons /
  status / video, `nightmode.go`. Tooltips are a canvas overlay that forwards clicks to the
  owner (a widget.PopUp swallowed the first click); never use PopUp for passive overlays
- `ipcam.go`/`ipcamui.go` IP cameras (not Xiaomi): any go2rtc source (onvif://, rtsp://,
  http://, tapo://, dvrip://); the stream has the active source (main/sub), the app keeps
  main, sub, sizes and codecs under `xiaomi_cam: cameras:` in go2rtc.yaml (yaml.Patch only
  takes 2-level paths). `cameraSource()` is Xiaomi only (""), use `cameraExists()` /
  `streamSource()`; `cameraQualities()` gives the quality list of any camera. Probing: a
  hidden `__xc_probe*` stream + ffprobe. `getProps`/`setProp`/`motor` return `errNotXiaomi`
- The WM reports a short minimize when it re-maps the window (frame on/off, wallpaper mode):
  `onMinimize` hides to tray only if still minimized after 0.7 s, and `notMinimized()` guards
  2 s (`remapping`). Hooks `XC_VIEWTEST` (video only on/off, minimize) and `XC_MOVETEST`
  (uinput drag of the move handle; the injected drag doesn't start a WM move here)
- Look (v1.22.0): `apptheme.go` palette/sizes/fonts/icons in one place (Geist in `fonts/`,
  Lucide in `icons/` made by `tools/icons.sh`: strokes → fills, Fyne recolors only fills;
  add an icon: run the script with its Lucide name, map it in `themeIcons` or `lucide(name)`).
  `overlay.go` controls on the picture (pills, hide after `controlsIdle`, not under the mouse;
  status chip `statusLabel`/`statusLayout`, `chipLayout` for truncated name chips),
  `segmented.go` quality control. App icon `packaging/xiaomi-cam.svg` → png via inkscape
  (also `cmd/xiaomi-cam/icon.png`). `XC_SHOTS=<dir>` screenshots main/light/menu/dialogs/grid
  (disables shortcuts: the test window can take the focus while the user types).
- `bars.go` ☰ menu (video only, full screen, cameras, language, help) and the light panel
  (spotlight, siren, night mode) – keep the bars narrow (window min ~660 px, `XC_LAYOUTTEST`
  prints the min sizes); grid tiles have a small video min size (`videoView.small`)
- `timeline.go` the history time axis (scale, known recordings from `miss.RecordingsBetween(did)`,
  playback position = file start + played time, wheel zoom, hover → `showHistoryPreview`),
  `thumbs.go` one thumbnail per minute (192x108 JPEG, `~/.cache/xiaomi-cam/thumbs/<did>/<unix
  minute>.jpg`, 30 days) saved from the shown picture (`captureThumb`: live = now, history =
  position). Not done: scanning a range for thumbnails (it would use the one camera session).
  `XC_TLTEST=<dir>` screenshots of the timeline and the preview. Event marks come from
  `evStore` (events.go); the min width is 120 px so the history bar fits the window min.
- Never call glfw `Restore()` (it can spin forever under XWayland): use `wmRestore`. A hang:
  read the newest ~/.cache/xiaomi-cam/hang-*.txt (goroutine 1 = the UI thread).
- `grid.go` multi-camera grid, `caps.go` hide unsupported features, `pointsui.go`/`presets.go`
  points 1–3, `history.go` SD playback, `recorder.go` MP4 (selected/each/grid),
  `ai.go`/`aiui.go` AI tasks (Claude SDK, OpenAI, own LM Studio/Ollama server, cron),
  `notify.go`/`notifyui.go` e-mail, Slack, ntfy, Telegram, `httpapi.go` + `mcp.go`
  (MCP server `xiaomi-cam mcp`, registered in the user's Claude Code), `autostart.go`,
  `settings.go`, `i18n.go` (Polish source strings, `T()` → English)
- go2rtc changes: `pkg/xiaomi/miss/cs2/server.go` (CS2 via Xiaomi servers, `p2p=server`),
  session close 0xF0, `miss` Command/Request/StartPlayback, motor, zoom

## Camera protocol facts (isa.camera.700sa)

- Remote video: CS2 servers 34.34.47.37 / 34.90.51.72 / 34.147.123.47:32100 (EU), the camera
  never answers LAN search. One camera session at a time.
- SD card file list (v1.25.0, `pkg/xiaomi/miss/rdt.go`): RDT = CS2 channel 1; every message
  is size (BE u32) + data encrypted with the session key (crypto.Encode/Decode), like
  commands without the code. Request cmd 6: 24 bytes LE (`6`, channel at offset 16 = 0);
  answer [cmd LE][size LE][status LE][data] split into ~2 KB messages, 8 bytes per file
  (start u32 LE, duration u8 (0 = 60), motion, saved, deleting). The user's card: ~17,300
  files, 12 days, ~0.5 s. Found via github.com/vickers-rz/go2rtc-c700-cloud-relay (docs
  only, no license: don't copy code). `cmd/xc-rdtprobe` lists the files and today's events
  with its own session (stop the app first). Never probe unknown MISS command codes
  blindly: 0x116 with binary data made the motor answer "motorReach".
- Events (v1.26.0): RDT cmd 11 = [11][len][JSON {"mac":"F1F2F3F4F5F6","time":"YYYYMMDD",
  "channel":"0"} padded to 4] (time "" answers nothing: ask per day); answer pages (same
  12-byte header) [?][total][index][32 B] + 32 B per event (start u64 LE, type u32 LE),
  last page index >= total; ~0.03 s a day. Codes (Mi Home plugin): 0 motion, 1 person, 2/3
  cat/dog, 4/5 face, 7 baby cry, 8 AI, 17 person near car, 19 car, 20/21 fence in/out (CW400
  hlc8: 12/14 car). Detection MIoT: 5.1 on, 5.2 interval min, 5.3 sensitivity 1-3, 13.4
  person, 13.5 motion, 13.1 JSON list [{id 1 type 2 car | id 2 type 1 fence, switch, event
  bits 1/2, pos, left_top/right_bottom in 640x360 (0 on fw >= _0060)}] (write one entry, code
  1 = being applied), 6.20 JSON which events start the sound+light alarm, 2.8/2.10 motion/
  person tracking, 10.1 patrol (car/fence exclude patrol and tracking). 2.7 recording on
  Hualai: 0 always, 1 motion, 2 off (spec names wrong). The user's camera had detection off
  (5.1/13.4/13.5 false; restored after tests). Cloud: the camera API host
  `{region}.business.smartcamera.api.io.mi.com` takes GET with RC4 params signed with the
  whole path (`Cloud.Get`, xmtool `camera:/path`); `/common/app/get/eventlist` is empty
  without a subscription (`/miot/camera/app/v1/vip/status`: NONE_VIP, free alerts ended
  2025-03), `/user/get_user_device_data` has no camera events either.
- SD playback (0x10D/0x10E): the answer is `{"id","status":"filefound|filenotfound","starttime",
  "duration"}`; files are ~60 s (60–62, sometimes shorter), one after another; the camera
  answers requests one after another (~0.2–0.3 s each, remote). `miss.StartPlayback` tries
  the starts predicted from known files first (cache `miss.Recordings`, saved by the app to
  `~/.cache/xiaomi-cam/recordings.json`), then the 70 s window in batches of 4. The producer
  keeps the packets read while probing (from the key frame) and sends them 800 ms after start
  (an RTSP consumer drops packets before its SETUP): the picture ~3 s sooner (live and SD).
  ffmpeg `-analyzeduration 0.5 s`, `-timeout 30 s`. `XC_HISTTIME=1` times the history
  steps (`miss.Debugf`, `histDebugf`). Idea not done: a temp cache of watched minutes (the
  user asked; one camera session at a time means no prefetch).
- MISS: motor 0x112 `{"operation":1-4}` starts continuous move (repeat every 200 ms while
  held), stop `{"operation":-1001}`; zoom 530 `{"value":zoom*10}` (1–9×); playback 0x10D needs
  the exact ~61 s file start (search second by second, offset 0).
- MIoT: spotlight 6.16, brightness 6.11, siren 6.10, night-shot 2.3 (0 off, 1 IR on, 2 auto,
  3 auto full color, 4 full color on, 5 auto smart full color; user's default 5), motor position 6.12, points 9.1 (list,
  base64 names) / 9.2 (go to idx), SD status 4.1; missing property → code -704040003.
- Mi Home RN plugin for the camera: cloud `/v2/plugin/fetch_plugin` (api_version ≥ 10100),
  dev tool `cmd/xmtool` calls any cloud API with the app's token.

## In progress (update after each step; for the next session)

State 2026-10-27: **v1.33.0** = Android: camera sound (speaker button / key M, pref `sound`, volume
only, no reconnect; live URL `RTSPURLAudio` = `?video&audio&opus_headers=1`, the grid stays
video only; on a parse/decode/audio error the player falls back to video only) and the home
screen widget `Widget.kt` (resizable, picture = `Mobile.KeyFrame` → go2rtc api/frame.mp4 →
MediaMetadataRetriever → `files/widget/<camera>.jpg` ≤ 1080 px; config activity: camera +
refresh by hand/15/30/60 min; JobScheduler `WidgetJob`: one-shot expedited per widget, periodic
15 min for the due ones; tap opens the app on the camera via `Widgets.EXTRA_CAMERA`).
go2rtc fix: Media3 1.5 RtpOpusReader needs the Ogg OpusHead + OpusTags as the first two RTP
packets ("ID Header missing"): `pkg/rtsp/opus.go`, only with `?opus_headers=1`. Tested on the
xc_car emulator with `xc-camsim` stream `/opus` (Opus decoder 48 kHz 2 ch, volume 0 dB / -inf,
widget add/resize/refresh/tap); not on the real camera (the PC app was watching it) nor the phone.
Earlier: **v1.32.0 released** = merged PRs #7–#10 (daniel24ka, Windows + two CW700S) with our fixes: #8 picture rotation (MIoT 2.2 `PropImageRollover`, ☰ "Obróć obraz o 180°", `POST rotation`), #9 telephoto lens (`Caps.Tele`/`TeleZoom` 700sa = 2.8, Wide | Tele segment `lens.go`, zoom steps stop at 2.8, `POST lens`, MCP `set_lens`), #10 aiming (`internal/xccore/aim.go`: absolute positions through temporary points 4–6 named "·" written to 9.1 and 9.2, `GotoPositionSteady` overshoot +12/+6 against the backlash, motor scale 8.4 / −21.8 px per unit, tele centre +22/+12 px; `aimui.go` Ctrl+click, `widemap.go` wide overview, `guard.go` back to the watch view, `moving.go` `xccore.Moving`; API position/aim/control, MCP `aim_camera`/`get_camera_control`), #7 on-air switch without dropping OBS. **Our fixes** (not in the PRs): the in-place source switch is `SetSourceInPlace` used only by `onair.go` (the camera's own `SetSource` live ↔ SD playback still stops the session first: one camera session at a time, v1.24.1); the RTP timeline is rebased only after `MoveNode` (`Node.rejoin`), never on a gap in one source (recordings keep real stalls); back to the watch view **off by default** (pref `watch_return_min` 0); the countdown ticker only while a camera is away; the points list is restored from a fresh read (Mi Home edits during a turn survive); position polled every 500 ms; the API marks manual control only after validation. Not tested on the real camera here (the author tested on two CW700S on Windows); aiming constants are his measurements.
Earlier: **v1.31.0** = merged PRs #3–#6 (daniel24ka, follow-ups to the #2 review): phone page same-origin only (`phoneAllowed`: Host local or *.ts.net, Origin = Host, Sec-Fetch-Site same-origin/none) and off by default (pref `phone_page`, Settings → General → Phone); `__xc_onair` only from Settings/API (`OffAirIfRemoved` replaces `EnsureOnAir`); PC archive switch (pref `archive`, off by default); libplacebo without fit_mode (Vulkan path now works on the local ffmpeg 8.0.1), GPU checks in the background at start (`probeGPU`, a player waits max 3 s).
**v1.30.0** = merged PR #2 (daniel24ka, Windows 11 + C200 user, OBS
broadcast): Hebrew UI (`i18n_he.go`, `bidi.go` visual order, `TJ`/`TM`/`TN`), camera model table
`internal/xccore/models.go` (700sa ids mapped per model; no entry = video only; C200 mapped;
qualities per model, non-EU regions without p2p=server), on-air RTSP `__xc_onair` for OBS
(`onair.go`; EnsureOnAir writes it to go2rtc.yaml at start), phone page `phone.go` on
127.0.0.1:11985 (allowlist proxy to the API; same-origin check since v1.31.0), portable `config` folder next to the
exe, digital zoom `digizoom.go` (after the camera zoom; decode width = view width rounded to
64), GPU paths `enhance.go`,
enhanced photo `photo.go` (key E), PC archive `archive.go` (external "cam-archive" service on
:8114, not public), producer dial without the lock (`internal/streams/producer.go`). Our fix:
`GetProps` matches the cloud answer by siid/piid – the cloud answers missing properties first
(6.23 on the CW700S), the PR paired by index and hid the spotlight.
**v1.29.0** = merged PR #1 (feature/onvif-controls by a teammate,
kwlodarski: ONVIF PTZ/points/IR cut/motion events + Imou Open API light/siren/night/points, see
Open ideas → IP cameras phase 2) + fix: HLS/MJPEG URLs and non-SOAP answers (CDN 404) =
`errNoONVIF` (before: the public webcams were probed on :80 every minute forever). Review
leftovers (low, not done): editing a camera keeps the running PullPoint loop with the old
host/login until it fails; points stay when Imou is switched off.
**v1.28.0** (Mi login: QR code login `qr=start`/`qr=wait` in
`/api/xiaomi` (`longPolling/loginUrl`, long poll `lp` ~20 s per call, 300 s expiry; tested up to
the QR shown, scan not tested: the user's account); the e-mail code looped on the car head unit
(a new code after a correct one, 24 h lock risk): now one cookie jar per login (`Cloud.lc`),
verify with the ticket in the form body + context + trust=true, result/check → Auth2/end (Tips
page) → sts, passToken/userId from the jar; a wrong code = `VerifyError` (400), the step stays,
never a new code; Android `LoginSession` keeps the step across activity recreation. Real 2FA
not tested (no codes without the user: the lock). **Never trigger Mi logins with the user's
account in tests.**) **v1.27.5** (fix: grid recording said "Saved" but no file: ffmpeg opens the inputs one after another, ~22 s for 9 cameras with the default probe, and creates the file only then; stop before that killed it. Now short probe (~7 s), wall clock timestamps + fps=25, status "Connecting to the cameras…" until `recording.writing()`, errors with ffmpeg stderr tail instead of "Saved"). **v1.27.4** (fix: the window hung ("not responding") on show from the tray: glfw Restore spins forever waiting for a VisibilityNotify, now `wmRestore` (XMapWindow + _NET_ACTIVE_WINDOW); `vblank_mode=0` (SwapBuffers blocked seconds while minimized under XWayland); hang watchdog `hang.go` writes goroutine stacks to ~/.cache/xiaomi-cam/hang-*.txt when the UI doesn't answer 5 s; `XC_RESTORETEST=<n>`, `XC_HANGTEST`). **v1.27.3** (HLS source: starts at the last two segments and paces packets by timestamps, `mpegts.Producer.Pace`; before, the whole playlist played fast at the start and each 6 s segment came in a burst). **v1.27.2** (fix: late frames of the grid tiles after the UI stopped
  called fyne.Do: `appStopped`; 4 Polish test cameras added). **v1.27.1** (the window starts with the grid: Settings → General →
  View at start, prefs `start_view` ""/"last", `last_camera`; fix: leaving the grid dropped
  the area editor layer, now `videoAreaObjects`). **v1.27.0** (`areaeditor.go`: the car area / virtual fence drawn on
  the live picture, saved to 13.1 like the Mi Home FencingArea page (switch on, pos as read,
  never 13.2 "activate" or 6.19 PTZ calibration: they move the motor); verified: the camera
  draws the same frame (13.6), motor 6.12 unchanged; `XC_AREATEST=<dir>` (puts the fence
  back)). **v1.26.0** (camera events: `events.go`/`eventsui.go`, core
  `internal/xccore/events.go`; timeline marks, filter, prev/next event, ☰ Event detection,
  ☰ Events list, Settings → Events watcher with actions (short RDT session without video
  when nobody watches, ~0.5 s), `list_events` MCP / `GET events`; `XC_EVENTTEST=<dir>`).
  Not done: events on Android. **v1.25.0** (SD card file list over RDT: calendar days, full timeline,
  see Camera protocol facts). **v1.24.3** (own `datepicker.go` replaces Fyne DateEntry: above the
  button, app language; `XC_DATETEST`). **v1.24.2** (timeline: drag moves the range, click picks; `browsed` stops
  following the playback). **v1.24.1** (fix: history showed live video – `streams.Producer.SetSource`
  now stops a running connection when the URL changes; v1.24.0 Windows, v1.23.1 Mi login code).
  Earlier: **v1.23.0 released** (desktop + first Android APKs, debug-signed: the user
  does not want a release key). Android app progress:
- Toolchain installed (no sudo): `~/Android/jdk` (Temurin 17), `~/Android/sdk` (platform 35,
  build-tools 35, NDK 27.3.13750724, emulator, images android-30 android-tv x86 + default
  x86_64), `~/Android/gradle-8.11.1`, `~/go/bin/gomobile`. Env for builds: JAVA_HOME,
  ANDROID_HOME, ANDROID_NDK_HOME (see Makefile `apk`).
- Test phone on USB: Redmi Note 11 Pro 5G (veux, Android 13 / HyperOS 1.0, arm64,
  1080x2400), adb `3f391f9ec38e`; USB install on; "USB debugging (security settings)" NOT on
  yet, so `adb shell input` fails (asked the user to enable it).
- Done: shared core `internal/xccore` (go2rtc start, cameras, MIoT, caps, points, night
  modes, IP camera storage, mover, Mi login, qualities); desktop uses it via aliases in
  `cmd/xiaomi-cam/core.go`. `mobile/` = gomobile API (JSON strings) → `android/app/libs/
  xccore.aar` (`gomobile bind -target=android/arm,arm64,386,amd64 -androidapi 26 -javapkg
  pl.lawinski.xiaomicam.core`, Java class `pl.lawinski.xiaomicam.core.mobile.Mobile`).
- `android/` (Gradle 8.11.1 wrapper, AGP 8.7.3, Kotlin 2.0.21, Compose, Media3 1.5.1): Core.kt
  (JSON wrappers), Player.kt (RTSP over TCP `?video` – Media3 can't read go2rtc's Opus track;
  decoder fallback: the phone's HW H.265 stops at 1920x1088, 2.5K plays on c2.android.hevc ~30%
  CPU), Live.kt (layouts: portrait = video on top + panel; landscape = overlay, zoom upright at
  the right; wide (≥360 dp beside 16:9) = car radio panel), Grid.kt, Setup.kt (Mi login,
  devices, IP camera, import go2rtc.yaml), Strings.kt (PL/EN), MainActivity.kt (menu and camera
  list are overlays over LIVE/GRID so the video keeps playing). No siren on Android (the
  user doesn't want it). Build: `make aar` + `cd android && ./gradlew assembleDebug`;
  test on the phone: `adb install -r app/build/outputs/apk/debug/app-arm64-v8a-debug.apk`
  (HyperOS asks to confirm each USB install on the phone), config via `run-as
  pl.lawinski.xiaomicam` (files/go2rtc.yaml, shared_prefs/xiaomi-cam.xml).
- go2rtc fixes for Media3: H.265 RTP without aggregation packets (pkg/h265/rtp.go),
  `reqTrackID` finds trackID= in the path even with a query (pkg/rtsp/server.go).
- Not done yet: TV/car emulator
  tests, wide car layout check, prefer the SW decoder directly when HW can't (saves ~0.5 s).
- Car radio: 14.9", 2560x720 (32:9): wide layout = 16:9 video left + touch controls right;
  emulator AVD "xc_car" 2560x720. TV AVD "xc_tv" (1080p).
- v1.22.0 history timeline (scale, zoom, recordings, playback marker) + thumbnail preview.
  **Open question to the user** (no answer yet): add a "Wczytaj miniatury" button that plays
  every Nth file of the shown range to take thumbnails (~2.5 s each, the one camera session is
  busy meanwhile).
- v1.21.0 SD history ~5x faster (predicted file starts, cache, key frame kept, ffmpeg 30 s).
  The user's idea of a temp cache of watched video: explained (helps only re-watching; no
  prefetch possible), not done; the user didn't insist.
- v1.20.x camera points: rename (✎, position stays), name under the number (10 px).
- v1.20.0 new look (variant A of the mockup https://claude.ai/artifact/B1ccjwrZwjCkYLtYvDsnEK),
  new app icon, zoom follows camera points.
- **Android app (APK) – postponed by the user ("android później")**: see Open ideas below for
  the agreed plan (native Kotlin + Media3 + Go core via gomobile, same repo). Before starting:
  ask to install JDK/SDK/NDK (~8–10 GB in ~/Android, no sudo); no Shield access for tests yet.
Notes for testing: the user's app usually runs as `xiaomi-cam --tray` (autostart) – stop it
with the anchored pkill (with `( --tray)?`), relaunch with `--tray`. The test window takes
the focus: hooks disable keyboard shortcuts. Never take full-screen screenshots where the
user's windows are (a password manager was captured once); crop to the test window only.
Point 1 of the camera is named "Home" by the user. Public test cameras (WhatsUpCams HLS):
Pula, Dubrownik – Stradun, Pag – plaża Gajac, Bale – Istria; Polish (webcamera.pl, 1080p H.264
HLS, no token; the page has the URL ROT13-coded in `data-src`): Kraków – Wawel, Gdańsk – Stare
Miasto, Sopot – Molo, Zakopane – Krupówki (9 cameras: grid 3x3, ~2 cores of 16). More found:
Kraków Zakole Wisły (krakow2), Kasprowy Wierch, Białystok, Zamość, Łeba, Karpacz.

## Open ideas / not done

- **Android app (planned, on hold – the user said "wait", 2026-10-15)** for an NVIDIA Shield (TV)
  and an Android car head unit (not Android Auto: video apps are not allowed there). Plan
  agreed in principle: same repo, `android/` (Kotlin, Jetpack Compose, Media3/ExoPlayer with
  hardware H.265/H.264 decoding of the local RTSP stream, D-pad/remote navigation, Leanback
  launcher entry, look of variant A) + a Go `mobile/` package built with gomobile into an .aar
  (embedded go2rtc with the Xiaomi CS2/MISS changes, login, PTZ, zoom, points, MIoT, IP
  cameras). First version: login or importing cameras from the PC (file/QR), live view,
  quality, grid, PTZ, zoom, points, light/siren/night mode; no tray, wallpaper, frameless
  window, MCP, AI tasks or recording. Needs (not installed yet, ask before): JDK 17, Android
  SDK + NDK, gomobile, Android TV emulator image (~8–10 GB in ~/Android, no sudo). No Shield
  access for tests yet (would need network ADB debugging and its IP).

- IP cameras phase 2 (branch `feature/onvif-controls`, 2026-10-18, built and tested on Windows
  with an Imou Cruiser IPC-S7C-3M0WE, fw 2.810 2025-12): `internal/xccore/onvifcam.go` (ONVIF
  detection, also for RTSP/HTTP cameras on port 80 with the same login; PTZ ContinuousMove
  with a 1 s timeout repeated by the Mover; points = positions kept in `IPCamera.Points`
  (Pos * 10000) + AbsoluteMove, only when a no-op AbsoluteMove to the current position works;
  IR cut filter → night modes auto/on/off), `onvifevents.go` (PullPoint per camera from
  `StartONVIFEvents`, motion rising edges → EventMotion in memory, `eventKey` "ip:<name>" +
  `evStore.merge` in the app). Imou facts: no ONVIF presets (MaximumNumberOfPresets 0), lists
  AbsolutePanTiltPositionSpace but AbsoluteMove = "not implemented" and GetStatus is always
  0.8/0.8, no IrCutFilter in Imaging, Dahua CGI on :80 doesn't answer (ONVIF does), RTSP
  `/cam/realmonitor?channel=1&subtype=0|1` with admin + the safety code. PullMessages needs
  WS-Addressing Action+To ("Resource Unknown" without); few subscriptions ("Subscribe Creation
  Failed" until old ones expire: subscribe for 120 s, renew every 60 s, Unsubscribe on exit).
  PTZ moves don't raise MotionAlarm. Tools: `cmd/xc-onvifprobe`, `XC_ONVIFCAM=<name> go test
  -run TestONVIFLive|TestONVIFPullRaw ./internal/xccore`. Imou spotlight/siren/night/presets/SD
  come from the Imou Open API (below). ONVIF IR cut filter untested (no camera).
- Imou Open API (`internal/xccore/imoucam.go`, 2026-10-18, same branch): `IPCamera.ImouApp/
  ImouSecret/ImouDC/ImouDevice` (form: "Sterowanie przez chmurę Imou", Get = device list);
  POST https://openapi-<dc>.easy4ip.com/openapi/<method> (dc fk = EU), sign = Base64(HMAC-
  SHA256("time:T,nonce:N,appSecret:S", hex(sha256(S)))) (`TestImouSign` = docs test case),
  accessToken 3 days (TK1002 renew). The developer app of the user's Imou Life account (same
  login on open.imoulife.com) sees its 7 devices (listDeviceDetailsByPage), nothing bound;
  Cruiser = `C41D4AEPCG84966` "Podwórko" (abilities WhiteLight, Siren, NVM, CollectionPoint,
  PT/PTZ, accessType PaaS). Light/siren = setDeviceCameraStatus whiteLight/siren (also
  read), served as MIoT props 6.16/6.10 by `GetProps`/`SetProp` (no brightness: code
  -704040003, `Caps.Brightness`); night = getNightVisionMode modes Intelligent/FullColor/
  Infrared/Off → app values 5/3/2/0; points = collection points by name (`Preset.Coll`):
  setCollection saves the current position, turnCollection, modifyCollection,
  deleteCollection; 21201 "Unknown error" right after another point change → `pointCall`
  retries. Verified on the Cruiser (`XC_IMOU="app|secret|serial" XC_IMOURTSP=<rtsp>
  XC_FFMPEG=dist/ffmpeg.exe go test -run TestImouLive ./internal/xccore`): night 5→2→5,
  point add/rename/delete, turn right 2 s + back to the point (picture diff 71.7 → 1.5).
  White light verified (`XC_IMOULIGHT=1`: on, off; the cloud reports a switch ~1 s late, so
  `imouSetProp` waits 1.5 s before the UI reads it back). Siren: setDeviceCameraStatus siren
  = DV1026 on the Cruiser (reading it works), so `setSiren` falls back to the things data
  model: listDeviceDetailsByIds → productId (Cruiser QLDuttGT), getProductModel → services
  SirenStart 25500 (input clientLocalTime 25501, "2006-01-02 15:04:05"), SirenStop 22200,
  GetSirenStatus 25700; iotDeviceControl {productId, deviceId, ref, content}. Verified by
  the user in the app (Syrena on/off, 2026-10-18; the auto-mode classifier blocks Claude
  from sounding it). The IoT model also has whiteLightStart/Stop (countdown), PtzMoveFour,
  collection services, GetLocalRecords. Cost: 30,000 requests + 2 GB flow free a month; never use cloud
  live/playback (bindDeviceLive HLS, RTMP) by default. SD playback works on the LAN without
  the cloud: `rtsp://admin:<code>@<ip>/cam/playback?channel=1&subtype=0&starttime=
  YYYY_MM_DD_HH_MM_SS&endtime=…` (404 = no recording in the window); list via the cloud
  `queryLocalRecords` (max 30 per call) – not done. Pan/tilt over the cloud (controlMovePTZ
  op 0-3 up/down/left/right + duration ms) is not used (ONVIF does it).
- The user has four public webcams added for testing (WhatsUpCams HLS, Croatia): "Pula (Chorwacja)",
  "Dubrownik – Stradun (Chorwacja)", "Pag – plaża Gajac (Chorwacja)", "Bale – Istria (Chorwacja)"
  (5 cameras with the Xiaomi one: grid 3 columns works). US DOT HLS feeds (Caltrans, Iowa) are geo-blocked here.
- Real tests of Telegram, e-mail SMTP, Slack, LM Studio/Ollama,
  Claude provider (no keys); a second real camera for the grid.
- CS2 server list hardcoded for EU (`&servers=` param for other regions).
