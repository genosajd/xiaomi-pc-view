// Package mobile is the Go core of the Android app (built with gomobile bind into an .aar):
// the embedded go2rtc with the Xiaomi changes (internal/xccore) and the camera controls.
// The app plays the local RTSP stream (RTSPURL) with Media3; lists and states are JSON.
// Only gomobile types here: string, bool, int, float64, error.
package mobile

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

var (
	startOnce sync.Once
	startErr  error
	dataDir   string

	camMu   sync.Mutex
	current string // the camera the mover drives
	mover   = xccore.NewMover(func() string {
		camMu.Lock()
		defer camMu.Unlock()
		return current
	}, nil)
)

// Start runs go2rtc with the config in dir (the app's files dir). Once per process.
func Start(dir string) error {
	startOnce.Do(func() {
		dataDir = dir
		startErr = xccore.Start(dir)
	})
	return startErr
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// --- cameras

type camera struct {
	Name   string `json:"name"`
	Info   string `json:"info"`
	Xiaomi bool   `json:"xiaomi"`
}

// Cameras returns [{"name","info","xiaomi"}].
func Cameras() string {
	list := []camera{}
	for _, n := range xccore.CameraNames() {
		c := camera{Name: n}
		if src := xccore.CameraSource(n); src != "" {
			c.Xiaomi, c.Info = true, xccore.SourceParam(src, "model")
		} else if ip := xccore.IPCam(n); ip != nil {
			if u, err := url.Parse(ip.Main); err == nil {
				c.Info = strings.ToUpper(ip.Kind) + " " + u.Hostname()
			}
		}
		list = append(list, c)
	}
	return toJSON(list)
}

// RTSPURL - the local RTSP stream of the camera for the player: video only (the grid).
func RTSPURL(name string) string { return xccore.RTSPURL(name) + "?video" }

// RTSPURLAudio - the stream with the sound of the camera: Opus tracks start with the Ogg
// headers that Media3 needs ("ID Header missing" without them).
func RTSPURLAudio(name string) string {
	return xccore.RTSPURL(name) + "?video&audio&opus_headers=1"
}

// KeyFrame saves one key frame of the camera as a short MP4 to path (the widget picture:
// the app decodes it with MediaMetadataRetriever; no ffmpeg on Android). It opens the
// camera session when nobody watches (a few seconds) and closes it after.
func KeyFrame(name, path string) error {
	b, code, err := xccore.APICall("GET", "/api/frame.mp4", url.Values{"src": {name}}, nil)
	if err != nil {
		return err
	}
	if code != 200 || len(b) == 0 {
		return errors.New(strings.TrimSpace(string(b)))
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Caps returns {"PTZ","Zoom","Light","Siren","Points","History","NightMode"} (cloud
// request, cached).
func Caps(name string) string { return toJSON(xccore.CameraCaps(name)) }

func RemoveCamera(name string) error { return xccore.RemoveCamera(name) }

// RenameCamera gives the camera a new name (the caller stops its video first).
func RenameCamera(name, newName string) error {
	newName = strings.TrimSpace(newName)
	if newName == "" || newName == name {
		return nil
	}
	if xccore.CameraExists(newName) {
		return errors.New("a camera with this name exists")
	}
	if c := xccore.IPCam(name); c != nil {
		if err := xccore.AddIPCamera(newName, c); err != nil {
			return err
		}
	} else if err := xccore.AddCamera(newName, xccore.StreamSource(name)); err != nil {
		return err
	}
	return xccore.RemoveCamera(name)
}

// AddIPCamera adds an IP camera by its address (rtsp://, http(s)://, onvif://, tapo://,
// dvrip://), login and password in the URL.
func AddIPCamera(name, src string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(src))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errors.New("wrong camera address")
	}
	kind := u.Scheme
	if kind == "https" || kind == "rtsps" {
		kind = strings.TrimSuffix(kind, "s")
	}
	if strings.TrimSpace(name) == "" {
		name = u.Hostname()
	}
	name = xccore.UniqueName(strings.TrimSpace(name))
	return name, xccore.AddIPCamera(name, &xccore.IPCamera{Kind: kind, Main: u.String()})
}

// --- quality

// Qualities returns {"names":[...],"sizes":[[w,h],...],"codecs":[...],"selected":i}
// (size 0x0, codec "": not known; the app checks if its decoder can play them).
func Qualities(name string) string {
	var names, codecs []string
	var sizes [][2]int
	sel := 0
	if c := xccore.IPCam(name); c != nil {
		names, sizes, codecs = []string{"Main"}, [][2]int{{c.W, c.H}}, []string{c.Codec}
		if c.Sub != "" {
			names = append(names, "Sub")
			sizes = append(sizes, [2]int{c.SubW, c.SubH})
			codecs = append(codecs, c.SubCodec)
			sel = min(max(c.Quality, 0), 1)
		}
	} else {
		for _, q := range xccore.CameraQualities(name) {
			names = append(names, q.Name)
			sizes = append(sizes, [2]int{q.W, q.H})
			codecs = append(codecs, "hevc") // isa.camera.700sa: H.265 in every quality
		}
		sel = xccore.XiaomiQuality(name)
	}
	return toJSON(map[string]any{"names": names, "sizes": sizes, "codecs": codecs, "selected": sel})
}

// SetQuality selects quality i of Qualities. The caller stops the video first.
func SetQuality(name string, i int) error {
	if xccore.IPCam(name) != nil {
		return xccore.SetIPQuality(name, i)
	}
	qs := xccore.CameraQualities(name)
	if i < 0 || i >= len(qs) {
		return errors.New("no such quality")
	}
	return xccore.SetSourceParams(name, map[string]string{"subtype": qs[i].Subtype})
}

// --- pan/tilt and zoom

// MotorPress starts moving the camera (1 left, 2 right, 3 up, 4 down) until MotorRelease.
func MotorPress(name string, op int) {
	camMu.Lock()
	current = name
	camMu.Unlock()
	mover.Press(op)
}

func MotorRelease() { mover.Release() }

// Zoom sets the camera zoom 1.0-9.0.
func Zoom(name string, z float64) error {
	z = min(max(z, 1), 9)
	if err := xccore.CameraZoom(name, z); err != nil {
		return err
	}
	xccore.SetZoomState(name, z)
	return nil
}

func CurrentZoom(name string) float64 { return xccore.CurrentZoom(name) }

// --- points 1-3

type point struct {
	Slot  int    `json:"slot"`
	Name  string `json:"name"`
	Saved bool   `json:"saved"`
}

// Points returns [{"slot","name","saved"}] for points 1-3.
func Points(name string) (string, error) {
	list, err := xccore.GetPresets(name)
	if err != nil {
		return "", err
	}
	var out []point
	for i, p := range xccore.PresetSlotsOf(list) {
		pt := point{Slot: i + 1}
		if p != nil {
			pt.Name, pt.Saved = p.Label(), true
		}
		out = append(out, pt)
	}
	return toJSON(out), nil
}

// GotoPoint turns the camera to point slot (1-3).
func GotoPoint(name string, slot int) error {
	list, err := xccore.GetPresets(name)
	if err != nil {
		return err
	}
	p := xccore.PresetSlotsOf(list)[min(max(slot, 1), xccore.PresetSlots)-1]
	if p == nil {
		return errors.New("no such point")
	}
	if err = xccore.GotoPreset(name, p.Idx); err != nil {
		return err
	}
	xccore.PointReached(name, *p)
	return nil
}

// SavePoint stores the current position as point slot (1-3).
func SavePoint(name string, slot int, label string) error {
	return xccore.SavePresetSlot(name, slot, label)
}

// --- spotlight, night mode (no siren: the user doesn't want it in the Android app)

// State returns {"light":bool,"brightness":int,"night":value} (MIoT).
func State(name string) (string, error) {
	props, err := xccore.GetProps(name, xccore.PropLight, xccore.PropBrightness, xccore.PropNightShot)
	if err != nil {
		return "", err
	}
	state := map[string]any{}
	for _, p := range props {
		if p.Code != 0 {
			continue
		}
		switch {
		case p.Same(xccore.PropLight):
			state["light"] = p.Value
		case p.Same(xccore.PropBrightness):
			state["brightness"] = p.Value
		case p.Same(xccore.PropNightShot):
			state["night"] = p.Value
		}
	}
	return toJSON(state), nil
}

func SetLight(name string, on bool) error { return xccore.SetProp(name, xccore.PropLight, on) }

// SetNightMode sets the night vision mode (a value of NightModes).
func SetNightMode(name string, value int) error {
	return xccore.SetProp(name, xccore.PropNightShot, value)
}

// NightModes returns [{"Value","ID","Name"}] in the order of the list (Polish names).
func NightModes() string { return toJSON(xccore.NightModes) }

// --- Mi account login

func Accounts() string {
	a := xccore.Accounts()
	if a == nil {
		a = []string{}
	}
	return toJSON(a)
}

// Regions returns [{"Code","Name"}].
func Regions() string { return toJSON(xccore.Regions) }

// Login sends the user name and password; the answer {"ok","captcha","verify_phone",
// "verify_email"} says what Xiaomi asks for next (LoginCode).
func Login(user, password string) (string, error) {
	r, err := xccore.Login(url.Values{"username": {strings.TrimSpace(user)}, "password": {password}})
	if err != nil {
		return "", err
	}
	return toJSON(r), nil
}

// LoginCode sends the code from the captcha picture (key "captcha") or from the SMS /
// e-mail (key "verify").
func LoginCode(key, code string) (string, error) {
	r, err := xccore.Login(url.Values{key: {strings.TrimSpace(code)}})
	if err != nil {
		return "", err
	}
	return toJSON(r), nil
}

// LoginQR starts a QR code login: {"qr" (base64 PNG), "qr_url"}; then LoginQRWait.
func LoginQR() (string, error) {
	r, err := xccore.Login(url.Values{"qr": {"start"}})
	if err != nil {
		return "", err
	}
	return toJSON(r), nil
}

// LoginQRWait waits ~20 s for the scan: {"ok"} or {"qr_wait": true} (call again).
func LoginQRWait() (string, error) {
	r, err := xccore.Login(url.Values{"qr": {"wait"}})
	if err != nil {
		return "", err
	}
	return toJSON(r), nil
}

// Devices returns [{"name","info","url"}] of the account in the region ([] - none).
func Devices(account, region string) (string, error) {
	list, err := xccore.Devices(account, region)
	if errors.Is(err, xccore.ErrNoDevices) {
		return "[]", nil
	}
	if err != nil {
		return "", err
	}
	return toJSON(list), nil
}

// AddXiaomi adds a camera of Devices; returns its name.
func AddXiaomi(name, deviceURL string) (string, error) {
	return xccore.AddXiaomiCamera(name, deviceURL)
}

// --- import from the PC app

// ImportConfig replaces the config with go2rtc.yaml of the PC app (Mi account and
// cameras). The app restarts its process afterwards (go2rtc reads the config at start).
func ImportConfig(yaml string) error {
	if !strings.Contains(yaml, "streams:") && !strings.Contains(yaml, "xiaomi:") {
		return errors.New("this is not a Xiaomi Cam config (go2rtc.yaml)")
	}
	if dataDir == "" {
		return errors.New("not started")
	}
	return os.WriteFile(filepath.Join(dataDir, "go2rtc.yaml"), []byte(yaml), 0o600)
}
