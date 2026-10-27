# Xiaomi Cam – Xiaomi Mi Home and IP camera viewer for Linux and Windows

A desktop app for Ubuntu/Linux and Windows to watch and control **Xiaomi Mi Home** cameras, which
officially work only in the Mi Home phone app. Tested with the **Xiaomi Outdoor Camera
CW700S** (`isa.camera.700sa`). It also shows **IP cameras** of other brands (ONVIF, RTSP,
HTTP/MJPEG, TP-Link Tapo, DVRIP/XMeye), together with Xiaomi cameras.

One binary file with an embedded [go2rtc](https://github.com/AlexxIT/go2rtc). It also has an
**MCP server** for AI assistants and a **local HTTP API** for scripts.

The **Windows** version has the same features (see [Windows](#windows)). There is also an
**Android app** (phones, Android TV / NVIDIA Shield, Android car radios):
see [Android](#android).

## Features

- Live video up to **2.5K** (2560×1440), quality 2.5K / 720p / SD.
- **Two lenses of the CW700S** (Wide | Tele next to the zoom): the camera switches to its
  telephoto sensor at zoom 2.8× (about 3× the detail of the wide lens cropped); the zoom steps
  stop there on the way. **Aim with a click**: Ctrl+click on the picture (or the aim button and
  a click) turns the camera so that the point comes to the centre of the telephoto lens; on the
  telephoto lens the wide picture stays in the corner with a frame of where the lens looks, and a
  click on it aims there. **Back to watching** (Settings → General, off by default): a camera
  turned or zoomed by hand goes back where it stood after N idle minutes, so a detector or a
  broadcast watching from a fixed view is never left blind. **Turn the picture 180°** (☰ menu):
  a camera hung upside down turns its own picture, so every viewer and recording is upright.
- **IP cameras** (Cameras → Add camera → IP camera):
  - **ONVIF** – the camera gives its stream addresses itself; **Search network** finds ONVIF
    cameras in the local network (WS-Discovery).
  - **RTSP** – address, login, password and the stream paths; ready paths for Hikvision /
    Annke / Ezviz, Dahua / Amcrest / Imou, Reolink, Tapo / VIGI, Uniview, Axis, Foscam, or a
    whole `rtsp://…` address.
  - **HTTP** (MJPEG, HLS `.m3u8` or H.264 over HTTP), **TP-Link Tapo** (own protocol, camera account
    password) and **DVRIP** (XMeye / Sofia cameras and recorders).
  - **Edit** (✎ in the camera list) opens the same form with the saved settings: name, type,
    address, login, password, stream paths.
  - **Test** checks the connection and the picture size before adding. Quality: **main** or
    **sub stream** (when the camera has two). Picture, sound, snapshots, MP4 recording
    (H.264/H.265 copied, MJPEG encoded to H.264), the grid, AI tasks, notifications, HTTP API
    and MCP work with IP cameras too.
  - **ONVIF controls** (also for a camera added as RTSP / HTTP: the app asks ONVIF on port 80
    with the same login): **pan/tilt**, **camera points 1–3** (positions kept by the app, for
    cameras with absolute PTZ positioning), **night mode** (the IR cut filter: auto / infrared /
    off) and **motion events** (a PullPoint subscription while the app runs: the events list,
    notifications, e-mail / ntfy / Telegram, snapshot, recording). Only what the camera
    answers is shown; e.g. Imou (Cruiser) has pan/tilt and motion events, but no positions and
    no IR cut filter over ONVIF.
  - **Imou cloud controls** (optional, for Imou cameras): the **white light**, the **siren**,
    the **night vision mode** (smart / full color / infrared / off) and **camera points 1–3**
    (collection points kept in the camera, also seen in the Imou Life app) over the
    [Imou Open API](https://open.imoulife.com). The video stays local (RTSP); the cloud only
    sends the commands (free: 30,000 requests a month, a few per action). See
    [Imou cameras](#imou-cameras) below. Zoom and SD history are Xiaomi only.
- **Remote connection from any network**, like Mi Home, without port forwarding or VPN.
  There is also a local (LAN) mode.
- **Pan/tilt**: buttons, arrow keys, or dragging the picture with the mouse.
- **Camera points 1–3**: save the current direction and zoom as a point (e.g. 1 = car,
  2 = driveway) and turn the camera back to it with one click or key 1–3. Points are stored
  in the camera, so Mi Home sees them too. Rename a point (✎ in the points window) without
  changing its position.
- **Native camera zoom up to 9×**: the camera zooms itself, so details stay sharp and the
  date/time printed by the camera keeps its size. Use the mouse wheel, `+`/`−`, or the
  slider in video-only mode.
- **SD card history**: date (a calendar in the app language), timeline of the day, ±1/±10 min, speed 1–16×, and back to live.
  A step takes about 2.5 s: when history opens, the app reads the list of the files on the
  SD card (~0.5 s, the RDT channel like Mi Home), so going to a time needs one request to
  the camera. The calendar enables only the days with recordings.
  The **timeline** has a scale (hours, minor ticks), shows the recordings on the SD card and the
  playback position; drag it (or scroll sideways on a touchpad) to move to another time, click
  to play from there, the mouse wheel zooms it (15 min – 24 h). Hovering shows the time and a
  **thumbnail** of that minute: thumbnails are kept (once a minute, ~5 KB) from the live
  picture and from the history you watched, for 30 days in `~/.cache/xiaomi-cam/thumbs`.
- **Camera events** (cameras with AI detection, e.g. the CW700S / CW400 Pro): motion, person,
  car, a person near a car, entering and leaving the virtual fence.
  - ☰ → **Event detection**: turn detection on, sensitivity, time between alarms, the event
    kinds, which events start the **sound and light alarm** (spotlight + siren), and motion /
    person tracking. Changes go to the camera right away, like in Mi Home.
  - **Car detection area** and **virtual fence**: drag a rectangle on the live picture
    (corners resize, the middle moves, outside draws a new one) and save; the camera watches
    it in the direction it faces (nothing turns the camera) and can draw it on the picture.
  - The **timeline** marks the recordings with an event in the colors of Mi Home; the
    preview names the event; buttons jump to the **previous / next event**, and a filter
    shows only chosen kinds. The camera saves the kind with its SD card recordings, and the
    app reads them over the RDT channel (command 11, ~0.03 s a day, past days only once).
  - ☰ → **Events**: the latest events of all cameras; a click plays the recording.
  - Settings → **Events**: check the cameras every 1–30 minutes and react to the chosen
    kinds: desktop notification, e-mail / ntfy / Telegram with a picture, Slack, save a
    snapshot, record a video. While you watch the camera this uses the open connection;
    otherwise a short connection without video (~0.5 s). It needs no Xiaomi cloud
    subscription: without one the cloud keeps no alerts.
- **Spotlight** with brightness, and **siren** (light bulb button on the picture, together
  with the night mode).
- **Night mode** (infrared): auto infrared, auto / smart full color (with the spotlight),
  always infrared, always full color, or off.
- **Local recording to MP4**: H.265 video copied as it comes from the camera (no
  re-encoding, almost no CPU), AAC audio. Fragmented MP4, so a file stays playable even if
  the app is closed while recording. Recording works with the window hidden.
- **Recording mode** (Settings → General): the selected camera, **each camera to its own
  file**, or **all cameras in one grid video** (choose which cameras with checkboxes) (H.265, cells 960×540, sound from the selected
  camera; this mode re-encodes and uses the CPU). The record button starts/stops all of them.
- Full resolution (2560×1440) snapshots.
- **Settings**: folders for recordings (default: your Videos folder) and snapshots (default:
  your Pictures folder).
- Camera audio.
- **Video-only mode**: no panels and no window frame. The window keeps the picture's aspect
  ratio and can be resized by any edge or moved with the ✥ button. Pan/tilt buttons and a
  zoom slider with a scale appear when you move the mouse.
- **Several cameras**: switch between one camera and a **grid** of all cameras (button in the
  top bar, shown with 2+ cameras). In the grid click a camera to select it – all controls
  (pan/tilt, zoom, points, spotlight, quality, snapshot, recording, sound) work on the
  selected one; double click opens it alone. The app starts with the grid (Settings → General
  → View at start: the grid or the last view); the last selected camera stays selected.
- Full screen mode.
- **Modern dark look**: graphite theme with one amber accent, the Geist font and Lucide icons.
  The picture fills the window. Pan/tilt, camera points, zoom, light and sound lie on the
  picture in translucent "pills" that show when you move the mouse and hide after 3 s. The
  status ("Live", recording, messages) is a small chip in the top right corner. The top bar has
  the camera list, quality (2.5K | 720p | SD), connection, grid, history, **Record**, snapshot,
  settings and the ☰ menu (video only, full screen, cameras, language, shortcuts). The grid
  shows cameras as rounded cards with their names. The window can be about 680 px narrow.
- **System tray**: closing or minimizing the window hides the app and stops streaming.
- Window size and position are remembered, separately with and without the frame.
- Only one instance runs; starting the app again shows the window that is already open.
- **AI tasks**: Claude or OpenAI checks the camera picture on a schedule (CRON) for what you
  describe in a prompt, and sends a system notification when it is detected.
- **Only what the camera can do**: controls of features the selected camera doesn't have
  (pan/tilt, points, zoom, spotlight, siren, SD card history) are hidden; detected from the
  camera's MIoT properties.
- **Tooltips** with a description on every button and control.
- **Progress indicators**: a button whose action takes a moment (snapshot, recording start/stop,
  camera points, notification tests, …) shows a spinner and ignores repeated clicks; a spinner
  next to the status line while the camera is changing a setting (spotlight, siren, zoom), and
  over the picture while connecting to the camera or searching for a recording.
- **Wallpaper mode** (tray menu): the window stays where it is, with its size and look, but
  lies under all windows like the desktop background (above the wallpaper, below the desktop
  icons), on all workspaces, and clicks go through it. Remembered after a restart; "Show" in the tray menu brings the normal window
  back (X11 / XWayland).
- Polish / English / Hebrew UI. Hebrew is shown right to left: Fyne lays text out left to
  right only, so `bidi.go` gives it the text in display order.
- Mi Home login with a QR code scanned by the phone (no password, no e-mail code), or with
  the password plus captcha and 2FA code support.

## Requirements

- Linux x86-64 (tested on Ubuntu 26.04, GNOME, X11/XWayland).
- `ffmpeg` for H.265 decoding: `sudo apt install ffmpeg`.
- On GNOME the tray icon needs the AppIndicator extension (enabled by default in Ubuntu).

## Install

From [Releases](../../releases) choose one of:

- **`xiaomi-cam_<version>_amd64.deb`** – Ubuntu/Debian package (recommended): installs the app
  to the menu with its icon and pulls in `ffmpeg`.

  ```sh
  sudo apt install ./xiaomi-cam_*_amd64.deb
  ```

  Remove with `sudo apt remove xiaomi-cam`.
- **`xiaomi-cam-<version>-linux-amd64`** – the plain program, run without installing
  (needs `ffmpeg`):

  ```sh
  chmod +x xiaomi-cam-*-linux-amd64 && ./xiaomi-cam-*-linux-amd64
  ```
- **`xiaomi-cam-<version>-linux-amd64.tar.gz`** – the program with the icon and the desktop
  file, for a manual install to `~/.local`:

  ```sh
  tar xzf xiaomi-cam-*-linux-amd64.tar.gz
  install -Dm755 xiaomi-cam/xiaomi-cam ~/.local/bin/xiaomi-cam
  install -Dm644 xiaomi-cam/xiaomi-cam.desktop ~/.local/share/applications/xiaomi-cam.desktop
  install -Dm644 xiaomi-cam/xiaomi-cam.svg ~/.local/share/icons/hicolor/scalable/apps/xiaomi-cam.svg
  ```

To keep the app in the Ubuntu dock, right click its icon → *Add to Favorites*. To start it at
login hidden in the tray, enable *Start at login* in Settings → General.

Or build from source (Go ≥ 1.25, a C compiler, X11/GL headers):

```sh
sudo apt install golang gcc libgl1-mesa-dev xorg-dev libxkbcommon-dev libwayland-dev
make install        # builds and installs to ~/.local
make deb            # builds dist/xiaomi-cam_<version>_amd64.deb
make release        # .deb, plain binary and .tar.gz in dist/
```

### Windows

Download **`xiaomi-cam-<version>-windows-setup.exe`** from [Releases](../../releases) and run
it (Windows 10/11, 64-bit). It installs for the current user without administrator rights:
the Start menu entry, a desktop icon, optionally start at login (in the tray), and ffmpeg
inside the app folder – nothing else to install. Remove it in Settings → Apps. The installer
is not signed: Windows SmartScreen may ask "Run anyway".

Settings and cameras: `%APPDATA%\xiaomi-cam-viewer\go2rtc.yaml` (the same file as on Linux –
it can be copied between them). Recordings and snapshots go to Videos and Pictures.

**Portable config:** when a `config` folder exists next to `xiaomi-cam.exe`, the app keeps
`go2rtc.yaml` there instead. Use it when the app is started by a process manager (pm2, a
service wrapper) or from a packaged (MSIX) app: such parents can see another `%APPDATA%` than
yours, and the app would start without cameras.

**Firewall:** cameras connected in the LAN (`cs2` without `p2p=server`, `tutk`) answer from
another UDP port, so Windows Firewall drops the answers until the app is allowed (inbound,
private network, local subnet), e.g. in an administrator PowerShell:
`New-NetFirewallRule -DisplayName "Xiaomi Cam" -Direction Inbound -Program "<path>\xiaomi-cam.exe" -Action Allow -Profile Private -RemoteAddress LocalSubnet`

On a PC without a graphics driver (a virtual machine, Remote Desktop) the app uses the
software OpenGL of Mesa (installed with it) – slower, but it works. The wallpaper mode on
Windows 11 24H2 puts the window under the desktop icons like Lively Wallpaper does; in a
virtual machine with the software OpenGL it was not shown (not tested on a PC with a GPU).

## First run

1. Start **Xiaomi Cam** from the applications menu.
2. Log in with your Mi Home account: **Sign in with a QR code** (scan it with a phone signed
   in to Xiaomi: Mi Home → + → Scan, and confirm on the phone; no e-mail code), or with the
   e-mail and password. The password goes only to the embedded go2rtc and to Xiaomi servers. Only a token is stored, in `~/.config/xiaomi-cam-viewer/go2rtc.yaml`
   (mode 600).
3. Choose the region of your account (e.g. **Europe**) and the cameras.

For an IP camera choose **IP camera** in the first step, then the type (ONVIF, RTSP, …),
the address, login and password. ONVIF usually has to be enabled in the camera settings
(often with its own account). The camera settings, with the password, are stored in the same
config file.

### Imou cameras

Imou (Dahua) cameras work over RTSP: type **RTSP**, producer **Dahua / Amcrest / Imou**, login
`admin` and the safety code from the camera label as the password. Pan/tilt and motion events
come over ONVIF. For the light, the siren, the night mode and camera points:

1. Log in at [open.imoulife.com](https://open.imoulife.com) with the **Imou Life** account
   (the same e-mail and password; the cameras of the app are seen by the API, nothing has to
   be bound).
2. **My App → Application Information**: type **PC Client**, any name, save. Copy the
   **AppId** and **AppSecret**.
3. In the camera form tick **Control over the Imou cloud**, paste the AppId and AppSecret,
   choose the data center (Europe for EU accounts), press **Get** and pick the camera.

The free plan has 30,000 API requests a month (a camera switch reads 3–4 values, each
command is 1–2 requests). The app never uses the cloud video (live HLS / RTMP or SD
playback): it counts as paid media flow beyond 2 GB a month.

## Android

The Android app (`android/`, Kotlin + Jetpack Compose + Media3) has the same camera core as
the desktop app (Go, built with gomobile), so it talks to the camera directly, from any
network – no PC needed. One APK for phones, Android TV (NVIDIA Shield, with the remote) and
Android car radios (a wide layout for 2560x720 screens). Android 8.0 or newer.

From [Releases](../../releases) download **`xiaomi-cam-<version>-universal.apk`** (or the
smaller `arm64-v8a` one for most phones and the Shield, `armeabi-v7a` for older 32-bit
devices) and open it on the device (allow installing from this source). The APK is signed
with a debug key: Android asks to confirm the install.

- **Add cameras:** sign in to Mi Home in the app (best with the QR code: on a car head unit
  scan it with the phone, no e-mail codes), or **Import from the PC**: pick the
  `~/.config/xiaomi-cam-viewer/go2rtc.yaml` file of the desktop app (the Mi account and all
  cameras, also IP cameras), or add an IP camera by its address.
- **Live view:** quality 2.5K / 720p / SD (2.5K uses the software decoder on phones whose
  hardware H.265 decoder stops at 1080p), pan/tilt (buttons, drag the picture, the remote's
  arrows), zoom, points 1–3 (keys 1–3), spotlight, night mode.
- **Grid** of all cameras; tap a tile to open it.
- Layouts: an upright phone (picture on top, controls below), landscape (controls on the
  picture, hidden after 5 s), car radio (picture left, big controls right).
- The video stops when the app is in the background (the camera takes one session at a
  time, so the PC app can connect meanwhile).

Not in the Android app: the siren, SD card history, camera events, recording, AI tasks, notifications,
the MCP server and the HTTP API.

Build: `make apk VERSION=vX.Y.Z` (needs JDK 17, Android SDK + NDK and gomobile, see
`Makefile`).

## Keyboard and mouse

| Key / mouse | Action |
|---|---|
| arrows, pan/tilt buttons (hold) | pan/tilt the camera |
| drag the picture | pan/tilt (not in video-only mode) |
| `+` / `−`, mouse wheel | camera zoom |
| `0` | zoom 1× |
| `1`, `2`, `3` | turn to camera point 1–3 |
| Ctrl+click on the picture | aim the camera at that point (telephoto lens) |
| `H` | video-only mode; `Esc` or right click to go back |
| `F`, `F11`, double click | full screen |
| `S` | snapshot |
| red dot button | start / stop recording |

## AI tasks (Settings → AI)

Let an AI model watch the camera for you. Add your **Claude** (Anthropic) and/or **OpenAI**
API key in *Settings → AI*. Then add tasks:

- **Prompt**: what to look for, e.g. *"Is there a delivery van at the gate?"* or *"Is there a
  person on the property?"*.
- **Schedule**: every 10/30 seconds, every 1/5/15/30 minutes, every hour, or custom: a
  **CRON** expression (`minute hour day month weekday`, e.g. `*/10 7-22 * * *` = every 10 min
  from 7:00 to 22:59) or an interval (`@every 20s`, `@every 2m`). A check that is still running
  is not started again. For frequent checks the camera session stays open (closed a minute
  after the last check). Every 10 s is about 8600 paid requests a day.
- **Model**: Claude (default `claude-opus-5-5`), OpenAI (default `gpt-5-mini`), or **your own
  server** in the network with an OpenAI-compatible API – **LM Studio** (`http://IP:1234`) or
  **Ollama** (`http://IP:11434`). Enter the address, an optional key, and pick the model with
  *Get models*; it must be a vision model (e.g. Qwen2.5-VL, Gemma 3, LLaVA). For LM Studio
  enable *Serve on Local Network* in the server settings; for Ollama set
  `OLLAMA_HOST=0.0.0.0`.
- **Camera point**: optionally turn the camera to point 1–3 before each check (e.g. look at
  the car, then ask about it).
- **Picture**: 1280 px by default, or **full resolution** (2560×1440) for small details such as
  car brands or number plates (the request costs more).
- **When detected**: system notification, **push to the phone** (ntfy, with the snapshot),
  **Telegram** (snapshot with a caption), **e-mail** (with the snapshot attached), **Slack**
  message, save a full resolution snapshot, and/or **record a video** of a chosen length
  (seconds, MP4 to the recordings folder). *Don't repeat for N minutes* prevents repeated
  actions. If an action fails (e.g. wrong SMTP password), the error is shown in the task list.

**Notifications (Settings → Notifications):**

- **Phone push (ntfy)**: install the [ntfy](https://ntfy.sh) app (Android/iOS), subscribe to
  a topic and enter the same topic in Xiaomi Cam (*Generate random* creates a long one – on the
  public ntfy.sh server anyone who knows the topic can read it). Optional own server and
  access token, priority (normal / high / urgent). The notification shows the snapshot.
- **Telegram**: create a bot with [@BotFather](https://t.me/BotFather) (`/newbot`), paste the
  token, send any message to your bot and click *Detect chat ID*. Notifications come as a
  picture with a caption.
- **E-mail**: SMTP server and port, encryption (STARTTLS / SSL/TLS / none), user, password,
  sender and recipients (comma separated). For Gmail use `smtp.gmail.com`, port 587, STARTTLS
  and an [app password](https://myaccount.google.com/apppasswords).
- **Slack**: an [Incoming Webhook](https://api.slack.com/messaging/webhooks) URL (Slack app →
  Incoming Webhooks → Add New Webhook to Workspace → choose a channel). Webhooks can't upload
  files, so Slack gets text only.
- *Send test* checks each channel.
- Each task can be switched on/off, run immediately (▶) to test it, edited or removed. The
  list shows the time and result of the last check.

Every check takes a snapshot (1280 px) and sends one request with the picture to the chosen
model, which answers `{"detected": true/false, "description": "..."}`. Mind the API cost for
frequent schedules. Tasks run while the app is running, also hidden in the tray. The keys and
tasks are stored in `~/.config/xiaomi-cam-viewer/ai.json` (mode 600).

## AI control (MCP)

`xiaomi-cam mcp` is a [Model Context Protocol](https://modelcontextprotocol.io) server over
stdio for AI assistants: Claude Code, Claude Desktop and others. If the app isn't running, it
starts it hidden in the tray.

**Claude Code:**

```sh
claude mcp add --scope user xiaomi-cam -- ~/.local/bin/xiaomi-cam mcp
```

**Claude Desktop** (`~/.config/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "xiaomi-cam": { "command": "/home/USER/.local/bin/xiaomi-cam", "args": ["mcp"] }
  }
}
```

**Tools.** All tools take an optional `camera` argument (a name from `list_cameras`); the
default is the first camera.

| Tool | Arguments | Result |
|---|---|---|
| `list_cameras` | – | camera names and models |
| `get_snapshot` | `width` (default 1280, max 2560) | live JPEG image (returned to the model as an image) |
| `get_camera_state` | – | zoom, spotlight on/off and brightness, siren |
| `move_camera` | `direction` (`left`/`right`/`up`/`down`), `steps` (default 10, max 100; 1 step = 0.1 s of movement) | pan/tilt |
| `set_zoom` | `zoom` (1.0–9.0) | camera zoom |
| `set_lens` | `lens` (`wide`/`tele`) | the lens of a dual-lens camera (CW700S) |
| `aim_camera` | `x`, `y` (-0.5..0.5 from the centre of the picture), `lens` | turns the camera so that the point of the picture comes to the centre |
| `get_camera_control` | – | whether the camera is away from its watch view and when it goes back |
| `set_spotlight` | `on` (bool), `brightness` (0–100) | spotlight |
| `set_siren` | `on` (bool) | alarm siren |
| `list_points` | – | camera points 1–3 and their names |
| `goto_point` | `point` (1–3 or name) | turns the camera to the point and waits until it stops |
| `save_point` | `point` (1–3), `name` | saves the current position (direction and zoom) as the point |
| `save_snapshot` | – | saves a 2560×1440 JPEG to the snapshots folder, returns the path |
| `start_recording` | `duration_seconds` (optional, auto stop), `mode` (`selected`/`each`/`grid`) | starts MP4 recording(s), returns the paths |
| `stop_recording` | – | finishes all recordings, returns the paths and durations |
| `recording_status` | – | recording or not, file path, elapsed seconds |
| `list_events` | `hours` (default 24) | camera events (motion, person, car, a person near a car, fence) with their times |

Example prompts: *"Show me what the camera sees"*, *"Turn the camera left and zoom in on the
gate"*, *"Is there a car parked in front of the house?"*.

## HTTP API

The running app serves a REST API on **`127.0.0.1:11984`**. It is local only and has no
authentication. All endpoints take an optional `camera=<name>`; the default is the first
camera. Errors return a non-200 status with a text message.

| Method | Path | Parameters | Response |
|---|---|---|---|
| GET | `/api/xiaomi-cam/cameras` | – | `[{"name": "...", "model": "..."}]` |
| GET | `/api/xiaomi-cam/snapshot` | `width` (optional) | `image/jpeg` of live video |
| GET | `/api/xiaomi-cam/state` | – | `{"zoom", "spotlight", "spotlight_brightness", "siren"}` |
| POST | `/api/xiaomi-cam/move` | `direction` = `left`/`right`/`up`/`down`, `steps` (1–100, 1 step = 0.1 s of movement) | `{"moved", "steps"}` |
| POST | `/api/xiaomi-cam/zoom` | `value` (1.0–9.0) | `{"zoom"}` |
| POST | `/api/xiaomi-cam/lens` | `name` = `wide`/`tele` | `{"lens", "zoom"}` – the lens of a dual-lens camera; `GET state` shows `lens` and `tele_zoom` |
| POST | `/api/xiaomi-cam/rotation` | `deg` = 0/90/180/270 | `{"rotation"}` – the camera turns its own picture; `GET state` shows `rotation` |
| GET | `/api/xiaomi-cam/position` | – | `{"x", "y", "scale"}` – motor position and zoom×10 |
| POST | `/api/xiaomi-cam/position/goto` | `x`, `y`, `zoom` (optional) | turns to motor coordinates (steady: the same pixels every time) |
| POST | `/api/xiaomi-cam/aim` | `x`, `y` (-0.5..0.5 from the centre of the picture), `lens` (`tele`/`wide`/empty) | `{"position", "zoom"}` – the point comes to the centre of the lens |
| GET | `/api/xiaomi-cam/control` | – | `{"away", "returns_in_s", "home", "moving", "return_after_min"}` – manual control and the way back to the watch view; `moving`: the app turns or zooms the camera now (+4 s) |
| POST | `/api/xiaomi-cam/control/return` | – | back to the watch view now |
| POST | `/api/xiaomi-cam/spotlight` | `on` (`true`/`false`), `brightness` (0–100) | `{"ok": true}` |
| POST | `/api/xiaomi-cam/siren` | `on` (`true`/`false`) | `{"siren"}` |
| POST | `/api/xiaomi-cam/snapshot/save` | – | `{"file"}` – JPEG saved to the snapshots folder |
| POST | `/api/xiaomi-cam/record/start` | `duration` (seconds, optional), `mode` = `selected`/`each`/`grid` (default from settings) | `{"recording", "files"}` |
| POST | `/api/xiaomi-cam/record/stop` | – | stops all recordings: `{"recording", "files": [{"file", "duration_s"}]}` |
| GET | `/api/xiaomi-cam/record` | – | `{"recording", "files": [{"file", "elapsed_s"}]}` |
| GET | `/api/xiaomi-cam/points` | – | `[{"point", "defined", "name"}]` |
| POST | `/api/xiaomi-cam/points/goto` | `point` (1–3 or name) | `{"point", "name"}` after the camera stops |
| POST | `/api/xiaomi-cam/points/save` | `point` (1–3), `name` | `{"point", "name"}` |
| GET | `/api/xiaomi-cam/events` | `hours` (default 24, max 744) | `{"camera", "events": [{"time", "kind", "name"}]}` |
| GET | `/api/xiaomi-cam/onair` | – | `{"camera", "rtsp"}` – the camera on air and the address for OBS |
| POST | `/api/xiaomi-cam/onair` | `camera` | puts the camera on air (OBS switches without touching the scene) |

```sh
# snapshot
curl -o camera.jpg "http://127.0.0.1:11984/api/xiaomi-cam/snapshot?width=1920"
# pan left by 20 steps, zoom 3x
curl -X POST "http://127.0.0.1:11984/api/xiaomi-cam/move?direction=left&steps=20"
curl -X POST "http://127.0.0.1:11984/api/xiaomi-cam/zoom?value=3"
# record 60 seconds to the recordings folder
curl -X POST "http://127.0.0.1:11984/api/xiaomi-cam/record/start?duration=60"
# spotlight on at 80 %
curl -X POST "http://127.0.0.1:11984/api/xiaomi-cam/spotlight?on=true&brightness=80"
```

Pan/tilt and zoom use the camera session. When the window is hidden in the tray, the API
opens a session itself and closes it a minute after the last command. If the app is showing
history, the snapshot comes from the recording being played.

The embedded go2rtc also serves the camera stream as RTSP:
`rtsp://127.0.0.1:18554/<camera name>`.

### Phone page

`http://127.0.0.1:11985` is a small page for a phone browser: live video of the cameras
(MSE, so H.265 plays where the phone decodes it; HLS on iPhones; a refreshing picture
otherwise), pan/tilt (hold the arrows), night mode, camera points, a snapshot and "Put on
air". The page follows the app language (Hebrew, English, Polish). It is off until switched
on in Settings → General → Phone.

It listens on localhost only and answers only its own page: the Host must be this computer or
its Tailscale name, and a browser request from another site (Origin / Sec-Fetch-Site, also
another localhost port) gets 403. To open it from the phone anywhere without opening router ports,
publish it to your own devices with [Tailscale](https://tailscale.com): install it on the PC
and the phone with the same account, then on the PC `tailscale serve --bg 11985` – the phone
opens `https://<pc-name>.<tailnet>.ts.net`. Only the page goes out: an allowlist in front of
the local go2rtc API lets through the cameras' video and those controls; the config (the Mi
token), the other API calls and streams of any other source answer 403.

### On air (OBS)

`rtsp://127.0.0.1:18554/__xc_onair` always shows the camera chosen in Settings → General →
On air (or `POST /api/xiaomi-cam/onair?camera=…`); the address exists once a camera is
chosen there, and goes away when that camera is removed. Add it once in OBS as a Media Source
(untick "Local File"); switching the broadcast camera later never touches the OBS scene. It
is video only (the camera microphone is never on air) and re-streams the camera's own
stream, so the app and OBS share one camera session. Recommended Media Source settings:
FFmpeg options `rtsp_transport=tcp fflags=nobuffer timeout=5000000`, network buffering 0 MB
(a 1 Mbps camera stream has seconds of delay in 1 MB), "Show nothing when playback ends" on.
Without the `timeout` option OBS keeps waiting on a dead connection after an app restart.
Switching the camera on air keeps the viewers' sessions: OBS sees the old camera until the new
one answers, on one continuous timeline.

## How it works

Xiaomi cameras don't offer RTSP. The video comes over Xiaomi's **MISS** protocol on **CS2**
P2P, the same way as in Mi Home:

1. The Xiaomi cloud returns the encryption keys and the camera's P2P id.
2. The app asks the Xiaomi CS2 servers to connect it with the camera. The servers send each
   side the address of the other, and both punch through NAT (UDP hole punching).
3. The video goes straight from the camera to the computer, encrypted. go2rtc serves it as
   RTSP on `127.0.0.1`, and ffmpeg decodes it.

This "server" mode was added to go2rtc in this repository, because some cameras (e.g. the
CW700S) don't answer the LAN search that upstream go2rtc uses.

Other controls:

| Function | How it is done |
|---|---|
| Pan/tilt | MISS command `0x112` `{"operation": 1-4}` starts moving, `{"operation": -1001}` stops |
| Points | MIoT `6.12` current position, `9.1` saved positions, `9.2` turn to a position |
| Zoom | MISS command `530` `{"value": zoom×10}`, found in the camera's Mi Home plugin |
| Spotlight, siren | Xiaomi cloud (MIoT properties) |
| History | MISS `PLAYBACK_REQ` `0x10D` |
| SD card files, events | RDT (CS2 channel 1): command `6` file list, `11` event types of a day |
| Event detection | MIoT `5.1`–`5.3`, `13.1` (car area, virtual fence: a rectangle in 640×360 of the frame), `13.4`, `13.5`, `13.6` (areas on the picture), `6.20` (sound and light alarm) |

For history, the camera needs the exact start of a recorded file (files are about 61 s). The
app searches for it second by second and plays from the start of the file that contains the
chosen time.

## Limitations

- CS2 server addresses are set for the **Europe** region. Cameras of other regions are added
  with the LAN connection (no `p2p=server`); for remote access add `&servers=ip1,ip2` to the
  camera source in the config.
- Picture qualities follow the model: 2.5K / 720p / SD for the CW400 / CW700S family, HD / SD
  for other (older) cameras, e.g. the C200 (`chuangmi.camera.046c04`, tested).
- Camera models are listed in `internal/xccore/models.go`: the app speaks the MIoT ids and
  values of `isa.camera.700sa`, and a model entry maps them to its own spec (ids, values,
  pan/tilt over the MISS motor command). A property the model doesn't have is answered as
  missing without asking the cloud, so its control is hidden and never written. Mapped: the
  CW400 / CW700S family and the C200 (`chuangmi.camera.046c04`: pan/tilt, night mode auto /
  infrared / off, SD status – it has no spotlight, siren, zoom or camera points). Other models
  get the video only until they are mapped (see https://home.miot-spec.com/spec/<model>).
  `cmd/xc-xprobe` prints the real connection error of a camera (stop the app first).
- Cameras with the `cs2` P2P protocol are supported (most newer Xiaomi cameras). Spotlight,
  siren and zoom follow the `isa.camera.700sa` specification.
- IP cameras: no zoom and no SD card history. Light, siren and points without ONVIF positions
  only for Imou cameras with the Imou cloud settings.

## License and credits

MIT. Based on [go2rtc](https://github.com/AlexxIT/go2rtc) by Alexey Khit (MIT); the original
README is in [README.go2rtc.md](README.go2rtc.md). The pan/tilt motor command is based on
AlexxIT/go2rtc#2531, and the command channel fix on AlexxIT/go2rtc#2489.
