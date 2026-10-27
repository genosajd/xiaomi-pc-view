package xccore

import (
	"errors"
	"sync"
)

// Camera capabilities: controls of features the selected camera doesn't have are hidden.
// Detected by reading the MIoT properties of the features (code 0 - the camera has it,
// -704040003 - no such property); zoom of isa.camera.700sa works over P2P (cmd 530), so
// known zoom models are listed.

type Caps struct {
	PTZ, Zoom, Light, Siren, Points, History, NightMode bool
	Brightness                                          bool    // of the light
	WDR, LowLightColor                                  bool    // picture settings (models.go)
	Tele                                                float64 // camera zoom of the telephoto lens (0: one lens)
}

// AllCaps - the cloud didn't answer: don't hide the usual controls (the picture settings
// stay hidden: only some models have them).
var AllCaps = Caps{PTZ: true, Zoom: true, Light: true, Siren: true, Points: true, History: true, NightMode: true, Brightness: true}

var zoomModels = map[string]bool{
	"isa.camera.700sa":  true, // Outdoor Camera CW700S (CW400 Pro OverSea)
	"isa.camera.hlzoom": true, // CW400 Pro
}

// Dual-lens cameras: the camera zoom (MISS 530) at which the camera switches from the
// wide-angle to the telephoto lens. Below it the zoom crops the wide lens (blurrier with
// every step); from it on the picture comes from the telephoto sensor in full detail. The
// camera sends one lens at a time (videoquality2 / channel=2 gives no media), and the zoom
// is the camera's state: every viewer of the camera sees the lens chosen.
// Measured on the CW700S (2026-10-26): 2.7 = the wide lens cropped (centred), 2.8 = the
// other lens (the picture jumps ~50 px: parallax) with ~3x the detail of 2.7.
var teleModels = map[string]float64{
	"isa.camera.700sa": 2.8, // Outdoor Camera CW700S
}

// TeleZoom returns the camera zoom of the telephoto lens of a dual-lens camera (0: one lens).
func TeleZoom(name string) float64 {
	src := CameraSource(name)
	if src == "" {
		return 0
	}
	return teleModels[SourceParam(src, "model")]
}

var CapsCache sync.Map // camera name -> Caps

// CameraCaps returns (cached) capabilities of a camera.
func CameraCaps(name string) Caps {
	if v, ok := CapsCache.Load(name); ok {
		return v.(Caps)
	}
	var c Caps
	src := CameraSource(name)
	if src == "" {
		return ipCaps(name) // an IP camera: the controls it has over ONVIF and the Imou cloud
	}
	model := SourceParam(src, "model")
	spec := SpecForModel(model)
	if spec == nil {
		// no spec mapped (models.go): the property ids may be other features on this
		// model, so it gets the video only
		CapsCache.Store(name, c)
		return c
	}
	c.Zoom = zoomModels[model]
	c.Tele = teleModels[model]
	props, err := GetProps(name, PropLight, PropSiren, PropMotorLocation, PropPresets, PropSDStatus, PropZoom, PropNightShot,
		PropWDR, PropLowLightColor)
	if err != nil {
		all := AllCaps // the cloud didn't answer: don't hide anything, try again next time
		all.Tele = c.Tele
		return all
	}
	for _, p := range props {
		ok := p.Code == 0
		switch {
		case p.Same(PropLight):
			c.Light, c.Brightness = ok, ok
		case p.Same(PropSiren):
			c.Siren = ok
		case p.Same(PropMotorLocation):
			c.PTZ = ok || spec.PanTilt
		case p.Same(PropPresets):
			c.Points = ok
		case p.Same(PropSDStatus):
			c.History = ok
		case p.Same(PropZoom):
			c.Zoom = c.Zoom || ok
		case p.Same(PropNightShot):
			c.NightMode = ok
		case p.Same(PropWDR):
			c.WDR = ok
		case p.Same(PropLowLightColor):
			c.LowLightColor = ok
		}
	}
	CapsCache.Store(name, c)
	return c
}

// ipCaps - the controls of an IP camera over ONVIF and the Imou cloud (what didn't answer
// is asked again next time).
func ipCaps(name string) Caps {
	var c Caps
	oc, oerr := onvifOf(name)
	if oerr == nil {
		c.PTZ = oc.PTZ
		c.Points = oc.PTZ && oc.Absolute
		c.NightMode = len(oc.nightModes()) > 0
	}
	ic, ierr := imouOf(name)
	if ierr == nil {
		c.Light, c.Siren = ic.Light, ic.Siren
		c.Points = c.Points || ic.Points
		c.NightMode = c.NightMode || len(ic.nightModes()) > 0
	}
	if (oerr == nil || errors.Is(oerr, errNoONVIF)) && (ierr == nil || errors.Is(ierr, errNoImou)) {
		CapsCache.Store(name, c)
	}
	return c
}
