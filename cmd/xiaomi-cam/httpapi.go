package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// Local HTTP API for scripts and AI (the MCP server uses it). Listens only on
// 127.0.0.1 (go2rtc API server). All endpoints take optional ?camera=<name>,
// default is the first camera.
//
//	GET  /api/xiaomi-cam/cameras
//	GET  /api/xiaomi-cam/snapshot?width=1280          image/jpeg
//	GET  /api/xiaomi-cam/state                       spotlight, brightness, siren, night mode, zoom, picture settings
//	POST /api/xiaomi-cam/picture?wdr=true&low_light_color=false  picture settings (models that have them)
//	POST /api/xiaomi-cam/move?direction=left&steps=5 left/right/up/down, 1 step = 0.1 s of movement
//	POST /api/xiaomi-cam/zoom?value=2.5              1.0-9.0 (camera zoom)
//	POST /api/xiaomi-cam/lens?name=tele              wide/tele lens of a dual-lens camera (CW700S: tele = zoom 2.8)
//	GET  /api/xiaomi-cam/position                    motor position {x, y, scale = zoom*10}
//	POST /api/xiaomi-cam/position/goto?x=&y=&zoom=   turn to motor coordinates (and zoom)
//	POST /api/xiaomi-cam/aim?x=0.2&y=-0.1&lens=tele  turn to a point of the picture (fractions of it from
//	                                                  its centre, -0.5..0.5); lens tele/wide/"" (the current)
//	GET  /api/xiaomi-cam/control                     manual control: away from the watch view, back in s;
//	                                                  moving - the app turns/zooms the camera (+4 s after)
//	POST /api/xiaomi-cam/control/return              back to the watch view now
//	Turning, zoom, lens, aiming and points by the API count as manual control: the camera goes
//	back to where it looked before after the minutes set in the app (Settings → General).
//	POST /api/xiaomi-cam/spotlight?on=true&brightness=80
//	POST /api/xiaomi-cam/siren?on=true
//	POST /api/xiaomi-cam/night-mode?mode=auto         auto/auto-color/smart-color/on/color/off
//	POST /api/xiaomi-cam/snapshot/save               saves JPEG to the snapshots folder
//	POST /api/xiaomi-cam/snapshot/enhanced           saves the enhanced photo (15 frames merged, PNG)
//	POST /api/xiaomi-cam/record/start?duration=60&mode=each&cameras=a,b  MP4 (H.265); mode: selected/each/grid
//	POST /api/xiaomi-cam/record/stop                 stops all recordings
//	GET  /api/xiaomi-cam/record                      recording status
//	GET  /api/xiaomi-cam/points                      camera points 1-3
//	POST /api/xiaomi-cam/points/goto?point=1         turn to a point (number or name)
//	POST /api/xiaomi-cam/points/save?point=1&name=auto  save the current position
//	GET  /api/xiaomi-cam/events?hours=24             camera events (motion, person, car, fence)
//	GET  /api/xiaomi-cam/onair                       the camera on air and the RTSP address for OBS
//	POST /api/xiaomi-cam/onair?camera=name           put a camera on air
//
// apiManual / apiGuard - manual control of a camera through the API (guard.go), set by the UI.
var (
	apiManual func(name string)
	apiGuard  *watchGuard
)

func manual(name string) {
	if apiManual != nil {
		apiManual(name)
	}
}

func initHTTPAPI(zoomChanged func(name string, zoom float64)) {
	handle := func(path, method string, f func(r *http.Request, name string) (any, error)) {
		api.HandleFunc("api/xiaomi-cam/"+path, func(w http.ResponseWriter, r *http.Request) {
			if r.Method != method {
				http.Error(w, method+" only", http.StatusMethodNotAllowed)
				return
			}
			name := r.URL.Query().Get("camera")
			if name == "" {
				if names := cameraNames(); len(names) > 0 {
					name = names[0]
				}
			}
			if !cameraExists(name) {
				http.Error(w, fmt.Sprintf("no camera %q", name), http.StatusNotFound)
				return
			}
			res, err := f(r, name)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if b, ok := res.([]byte); ok {
				w.Header().Set("Content-Type", "image/jpeg")
				_, _ = w.Write(b)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(res)
		})
	}

	api.HandleFunc("api/xiaomi-cam/onair", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
		case "POST":
			name := r.URL.Query().Get("camera")
			if !cameraExists(name) {
				http.Error(w, fmt.Sprintf("no camera %q", name), http.StatusNotFound)
				return
			}
			if err := xccore.SetOnAir(name); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		default:
			http.Error(w, "GET or POST", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"camera": xccore.OnAirCamera(), "rtsp": xccore.OnAirURL()})
	})

	api.HandleFunc("api/xiaomi-cam/cameras", func(w http.ResponseWriter, r *http.Request) {
		type camera struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		}
		cams := []camera{}
		for _, n := range cameraNames() {
			cams = append(cams, camera{n, cameraInfo(n)})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cams)
	})

	handle("snapshot", "GET", func(r *http.Request, name string) (any, error) {
		width, _ := strconv.Atoi(r.URL.Query().Get("width"))
		return snapshotJPEG(name, width)
	})

	handle("state", "GET", func(r *http.Request, name string) (any, error) {
		if c := ipCam(name); c != nil && !hasImou(name) {
			state := map[string]any{"camera": name, "type": kindName(c.Kind), "quality": currentQuality(name).name}
			if len(cameraNightModes(name)) > 0 {
				if v, err := getNightMode(name); err == nil {
					if i := nightModeByValue(v); i >= 0 {
						state["night_mode"] = nightModes[i].ID
					}
				}
			}
			return state, nil
		}
		props, err := getProps(name, propLight, propBrightness, propSiren, propNightShot, propWDR, propLowLightColor,
			propImageRollover)
		if err != nil {
			return nil, err
		}
		state := map[string]any{"camera": name, "zoom": currentZoom(name)}
		if tele := teleZoom(name); tele > 0 {
			state["lens"], state["tele_zoom"] = "wide", tele
			if onTele(currentZoom(name), tele) {
				state["lens"] = "tele"
			}
		}
		if c := ipCam(name); c != nil { // Imou: the light, siren and night mode in the cloud
			state = map[string]any{"camera": name, "type": kindName(c.Kind), "quality": currentQuality(name).name}
		}
		for _, p := range props {
			if p.Code != 0 {
				continue
			}
			switch {
			case p.Same(propWDR):
				state["wdr"] = p.Value
				continue
			case p.Same(propLowLightColor):
				state["low_light_color"] = p.Value
				continue
			case p.Same(propImageRollover):
				state["rotation"] = p.Value
				continue
			}
			switch p.Piid {
			case propLight.Piid:
				state["spotlight"] = p.Value
			case propBrightness.Piid:
				state["spotlight_brightness"] = p.Value
			case propSiren.Piid:
				state["siren"] = p.Value
			case propNightShot.Piid:
				if v, ok := p.Value.(float64); ok && p.Code == 0 {
					if i := nightModeByValue(int(v)); i >= 0 {
						state["night_mode"] = nightModes[i].ID
					}
				}
			}
		}
		return state, nil
	})

	handle("move", "POST", func(r *http.Request, name string) (any, error) {
		manual(name)
		ops := map[string]int{"left": motorLeft, "right": motorRight, "up": motorUp, "down": motorDown}
		dir := r.URL.Query().Get("direction")
		op, ok := ops[dir]
		if !ok {
			return nil, errors.New("direction must be left, right, up or down")
		}
		steps, _ := strconv.Atoi(r.URL.Query().Get("steps"))
		steps = min(max(steps, 1), 100)
		if err := ensureSession(name); err != nil {
			return nil, err
		}
		// the camera moves continuously until stopped: one step = 100 ms of movement
		var moveErr error
		m := newMover(func() string { return name }, func(err error) { moveErr = err })
		m.Pulse(op, time.Duration(steps)*100*time.Millisecond)
		time.Sleep(300 * time.Millisecond) // let the stop command go out
		if moveErr != nil {
			return nil, moveErr
		}
		return map[string]any{"moved": dir, "steps": steps}, nil
	})

	handle("zoom", "POST", func(r *http.Request, name string) (any, error) {
		manual(name)
		z, err := strconv.ParseFloat(r.URL.Query().Get("value"), 64)
		if err != nil || z < 1 || z > maxZoom {
			return nil, fmt.Errorf("value must be 1.0-%.1f", maxZoom)
		}
		if err = ensureSession(name); err != nil {
			return nil, err
		}
		if err = cameraZoom(name, z); err != nil {
			return nil, err
		}
		setZoomState(name, z)
		if zoomChanged != nil {
			zoomChanged(name, z)
		}
		return map[string]any{"zoom": z}, nil
	})

	// the lens of a dual-lens camera is the camera's zoom (lens.go)
	handle("lens", "POST", func(r *http.Request, name string) (any, error) {
		manual(name)
		tele := teleZoom(name)
		if tele == 0 {
			return nil, errors.New("the camera has one lens")
		}
		lens, z := r.URL.Query().Get("name"), 1.0
		switch lens {
		case "tele":
			z = tele
		case "wide":
		default:
			return nil, errors.New("name must be wide or tele")
		}
		if err := ensureSession(name); err != nil {
			return nil, err
		}
		if err := cameraZoom(name, z); err != nil {
			return nil, err
		}
		setZoomState(name, z)
		if zoomChanged != nil {
			zoomChanged(name, z)
		}
		return map[string]any{"lens": lens, "zoom": z}, nil
	})

	handle("position", "GET", func(r *http.Request, name string) (any, error) {
		return xccore.MotorPosition(name)
	})

	// turn to motor coordinates (from GET position); zoom 1.0-9.0, default: the current one
	handle("position/goto", "POST", func(r *http.Request, name string) (any, error) {
		manual(name)
		q := r.URL.Query()
		x, errX := strconv.Atoi(q.Get("x"))
		y, errY := strconv.Atoi(q.Get("y"))
		if errX != nil || errY != nil {
			return nil, errors.New("x and y must be motor coordinates (GET position)")
		}
		p := xccore.Position{X: x, Y: y}
		if z, err := strconv.ParseFloat(q.Get("zoom"), 64); err == nil {
			if z < 1 || z > maxZoom {
				return nil, fmt.Errorf("zoom must be 1.0-%.1f", maxZoom)
			}
			p.Scale = int(z*10 + 0.5)
		}
		if err := xccore.GotoPositionSteady(name, p); err != nil {
			return nil, err
		}
		if zoomChanged != nil {
			zoomChanged(name, currentZoom(name))
		}
		return xccore.MotorPosition(name)
	})

	handle("aim", "POST", func(r *http.Request, name string) (any, error) {
		q := r.URL.Query()
		fx, errX := strconv.ParseFloat(q.Get("x"), 64)
		fy, errY := strconv.ParseFloat(q.Get("y"), 64)
		if errX != nil || errY != nil {
			return nil, errors.New("x and y must be the point of the picture: fractions of it from its centre, -0.5..0.5")
		}
		manual(name)
		p, err := xccore.AimAt(name, fx, fy, q.Get("lens"))
		if err != nil {
			return nil, err
		}
		if zoomChanged != nil {
			zoomChanged(name, currentZoom(name))
		}
		return map[string]any{"position": p, "zoom": currentZoom(name)}, nil
	})

	handle("control", "GET", func(r *http.Request, name string) (any, error) {
		res := map[string]any{"camera": name, "return_after_min": watchReturnMinutes(), "away": false,
			"moving": xccore.Moving(name)} // turned or zoomed by the app now (or a few seconds ago)
		if apiGuard == nil {
			return res, nil
		}
		away, returning, until, home := apiGuard.state(name)
		res["away"] = away
		if away {
			res["returning"] = returning
			res["returns_in_s"] = int(max(time.Until(until), 0).Seconds())
			res["home"] = home
		}
		return res, nil
	})

	handle("control/return", "POST", func(r *http.Request, name string) (any, error) {
		if apiGuard == nil {
			return nil, errors.New("not ready")
		}
		if away, _, _, _ := apiGuard.state(name); !away {
			return map[string]any{"camera": name, "away": false}, nil
		}
		if err := apiGuard.goBack(name); err != nil {
			return nil, err
		}
		return xccore.MotorPosition(name)
	})

	handle("spotlight", "POST", func(r *http.Request, name string) (any, error) {
		q := r.URL.Query()
		if b := q.Get("brightness"); b != "" {
			v, err := strconv.Atoi(b)
			if err != nil || v < 0 || v > 100 {
				return nil, errors.New("brightness must be 0-100")
			}
			if err = setProp(name, propBrightness, v); err != nil {
				return nil, err
			}
		}
		if on := q.Get("on"); on != "" {
			if err := setProp(name, propLight, on == "true" || on == "1"); err != nil {
				return nil, err
			}
		}
		return map[string]any{"ok": true}, nil
	})

	handle("picture", "POST", func(r *http.Request, name string) (any, error) {
		q := r.URL.Query()
		for key, p := range map[string]miotProp{"wdr": propWDR, "low_light_color": propLowLightColor} {
			if v := q.Get(key); v != "" {
				if err := setProp(name, p, v == "true" || v == "1"); err != nil {
					return nil, fmt.Errorf("%s: %w", key, err)
				}
			}
		}
		return map[string]any{"ok": true}, nil
	})

	// the camera turns its own picture (?deg=0|90|180|270): a camera hung upside down
	handle("rotation", "POST", func(r *http.Request, name string) (any, error) {
		deg, err := strconv.Atoi(r.URL.Query().Get("deg"))
		if err != nil || deg < 0 || deg > 270 || deg%90 != 0 {
			return nil, fmt.Errorf("deg: 0, 90, 180 or 270")
		}
		if err = setProp(name, propImageRollover, deg); err != nil {
			return nil, err
		}
		return map[string]any{"rotation": deg}, nil
	})

	handle("night-mode", "POST", func(r *http.Request, name string) (any, error) {
		m, err := nightModeByID(r.URL.Query().Get("mode"))
		if err != nil {
			return nil, err
		}
		if err = setNightMode(name, m.Value); err != nil {
			return nil, err
		}
		return map[string]any{"night_mode": m.ID}, nil
	})

	handle("snapshot/enhanced", "POST", func(r *http.Request, name string) (any, error) {
		files, err := enhancedPhoto(name, nil)
		if err != nil {
			return nil, err
		}
		return map[string]any{"files": files}, nil
	})

	handle("snapshot/save", "POST", func(r *http.Request, name string) (any, error) {
		path, err := saveSnapshot(name)
		if err != nil {
			return nil, err
		}
		return map[string]any{"file": path}, nil
	})

	handle("record/start", "POST", func(r *http.Request, name string) (any, error) {
		secs, _ := strconv.Atoi(r.URL.Query().Get("duration"))
		mode := recordMode()
		switch m := r.URL.Query().Get("mode"); m {
		case "selected", "camera":
			mode = recordSelected
		case recordEach, recordGrid:
			mode = m
		}
		var cams []string // nil - cameras from the settings
		if c := r.URL.Query().Get("cameras"); c != "" {
			for _, n := range strings.Split(c, ",") {
				if n = strings.TrimSpace(n); cameraExists(n) {
					cams = append(cams, n)
				} else if n != "" {
					return nil, fmt.Errorf("no camera %q", n)
				}
			}
		}
		recs, err := startRecordings(mode, name, cams, time.Duration(secs)*time.Second)
		if len(recs) == 0 && err != nil {
			return nil, err
		}
		var files []string
		for _, rec := range recs {
			files = append(files, rec.path)
		}
		res := map[string]any{"recording": true, "files": files}
		if secs > 0 {
			res["duration_s"] = secs
		}
		if err != nil {
			res["errors"] = err.Error()
		}
		return res, nil
	})

	handle("record/stop", "POST", func(r *http.Request, name string) (any, error) {
		stopped := stopAll()
		if len(stopped) == 0 {
			return nil, errors.New("not recording")
		}
		var files []map[string]any
		for _, rec := range stopped {
			f := map[string]any{"file": rec.path, "duration_s": int(time.Since(rec.started).Seconds())}
			if rec.err != nil {
				f = map[string]any{"error": rec.err.Error()}
			}
			files = append(files, f)
		}
		return map[string]any{"recording": false, "files": files}, nil
	})

	handle("record", "GET", func(r *http.Request, name string) (any, error) {
		active := activeRecordings()
		if len(active) == 0 {
			return map[string]any{"recording": false}, nil
		}
		var files []map[string]any
		for _, rec := range active {
			files = append(files, map[string]any{"file": rec.path, "elapsed_s": int(time.Since(rec.started).Seconds())})
		}
		return map[string]any{"recording": true, "files": files}, nil
	})

	handle("points", "GET", func(r *http.Request, name string) (any, error) {
		list, err := getPresets(name)
		if err != nil {
			return nil, err
		}
		points := []map[string]any{}
		for i, p := range presetSlotsOf(list) {
			item := map[string]any{"point": i + 1, "defined": p != nil}
			if p != nil {
				item["name"] = p.Label()
			}
			points = append(points, item)
		}
		return points, nil
	})

	handle("points/goto", "POST", func(r *http.Request, name string) (any, error) {
		manual(name)
		p, err := findPoint(name, r.URL.Query().Get("point"))
		if err != nil {
			return nil, err
		}
		if err = gotoPresetAndWait(name, p.Idx); err != nil {
			return nil, err
		}
		pointReached(name, p)
		return map[string]any{"point": p.Idx, "name": p.Label(), "zoom": currentZoom(name)}, nil
	})

	handle("points/save", "POST", func(r *http.Request, name string) (any, error) {
		slot, _ := strconv.Atoi(r.URL.Query().Get("point"))
		label := r.URL.Query().Get("name")
		if err := savePresetSlot(name, slot, label); err != nil {
			return nil, err
		}
		if pointsChanged != nil {
			pointsChanged()
		}
		return map[string]any{"point": slot, "name": label}, nil
	})

	handle("events", "GET", func(r *http.Request, name string) (any, error) {
		if len(cameraEvents(name)) == 0 {
			return nil, fmt.Errorf("camera %q reports no events", name)
		}
		hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
		if hours <= 0 {
			hours = 24
		}
		hours = min(hours, 31*24)
		from := time.Now().Add(-time.Duration(hours) * time.Hour)
		// today from the camera (a short session when nobody watches it), older days kept
		_, err := readDayEvents(name, time.Now())
		type ev struct {
			Time string    `json:"time"`
			Kind eventKind `json:"kind"`
			Name string    `json:"name"`
		}
		out := []ev{}
		for _, e := range evStore.between(eventKey(name), from, time.Now().Add(time.Hour)) {
			out = append(out, ev{e.Time.Format(time.RFC3339), e.Kind, english[eventNames[e.Kind]]})
		}
		res := map[string]any{"camera": name, "events": out}
		if err != nil {
			res["warning"] = "today's events not read: " + err.Error()
		}
		return res, nil
	})

	handle("siren", "POST", func(r *http.Request, name string) (any, error) {
		on := r.URL.Query().Get("on")
		if err := setProp(name, propSiren, on == "true" || on == "1"); err != nil {
			return nil, err
		}
		return map[string]any{"siren": on == "true" || on == "1"}, nil
	})
}

// pointsChanged - points saved through the API (UI refresh)
var pointsChanged func()

// --- snapshot

// snapshotJPEG grabs one frame of live video. It shares the camera session with the
// app window (go2rtc has one producer per stream), so it works while the app shows video.
func snapshotJPEG(name string, width int) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	vf := "null"
	if width > 0 {
		vf = fmt.Sprintf("scale=%d:-2", width)
	}
	cmd := toolCommand(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-nostdin",
		"-rtsp_transport", "tcp", "-i", rtspURL(name),
		"-map", "0:v:0", "-frames:v", "1", "-vf", vf, "-q:v", "3", "-f", "image2", "-c:v", "mjpeg", "pipe:1")
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("snapshot: %v %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return out.Bytes(), nil
}

// --- session for commands

// Pan/tilt and zoom are sent over the camera session. When the app window doesn't show
// video (hidden to tray), a background consumer keeps the session open for a minute
// after the last command.

type keeper struct {
	cancel context.CancelFunc
	timer  *time.Timer
}

var (
	keepMu  sync.Mutex
	keepers = map[string]*keeper{}
)

func hasSession(name string) bool {
	s := streams.Get(name)
	return s != nil && len(s.Conns()) > 0
}

func ensureSession(name string) error {
	keepMu.Lock()
	k := keepers[name]
	if k != nil {
		k.timer.Reset(time.Minute)
	}
	keepMu.Unlock()

	if hasSession(name) {
		return nil
	}
	if k == nil {
		ctx, cancel := context.WithCancel(context.Background())
		cmd := toolCommand(ctx, "ffmpeg", "-hide_banner", "-loglevel", "quiet", "-nostdin",
			"-rtsp_transport", "tcp", "-i", rtspURL(name), "-map", "0:v:0", "-c", "copy", "-f", "null", "-")
		if err := cmd.Start(); err != nil {
			cancel()
			return err
		}
		go func() { _ = cmd.Wait() }()
		k = &keeper{cancel: cancel}
		k.timer = time.AfterFunc(time.Minute, func() {
			keepMu.Lock()
			delete(keepers, name)
			keepMu.Unlock()
			cancel()
		})
		keepMu.Lock()
		keepers[name] = k
		keepMu.Unlock()
	}
	for i := 0; i < 60; i++ {
		if hasSession(name) {
			time.Sleep(500 * time.Millisecond) // let the session start media
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return errors.New("can't connect to the camera")
}
