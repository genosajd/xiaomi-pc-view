package xccore

// Camera models. The app speaks the MIoT property ids and values of isa.camera.700sa
// (backend.go Prop*, nightmode.go, presets.go); a ModelSpec maps them to another model's
// spec, so supporting another model is one entry in modelSpecs. GetProps / SetProp
// translate ids and values; a property the model doesn't have answers codeNoProp without
// asking the cloud, so its control is hidden (caps.go) and never written. Models without a
// spec get the video only.

type propID struct{ Siid, Piid int }

type ModelSpec struct {
	// Props maps the 700sa properties to the model's; nil - the model uses the 700sa ids
	// (CW400 / CW700S family).
	Props map[propID]propID
	// Values maps the 700sa values of a property to the model's (missing value: the model
	// has no such setting).
	Values map[propID]map[int]int
	// PanTilt - pan/tilt with the MISS motor command (0x112, operation 1-4, stop -1001)
	// on a model without the motor position property (no camera points then).
	PanTilt bool
}

var hualaiSpec = &ModelSpec{} // the 700sa ids as they are

var modelSpecs = map[string]*ModelSpec{
	"isa.camera.700sa":  hualaiSpec, // Outdoor Camera CW700S
	"isa.camera.hlzoom": hualaiSpec, // CW400 Pro
	"isa.camera.hlc8":   hualaiSpec, // CW400

	// Mi 360° Home Security Camera 2K / C200
	// https://home.miot-spec.com/spec/chuangmi.camera.046c04 – no spotlight, siren, zoom or
	// camera points; night-shot 2.3 is 0 on, 1 off, 2 auto (infrared).
	"chuangmi.camera.046c04": {
		Props: map[propID]propID{
			{2, 3}: {2, 3}, // night-shot
			{2, 5}: {2, 5}, // wdr-mode
			{2, 6}: {2, 6}, // glimmer-full-color
			{4, 1}: {4, 1}, // memory card status
		},
		Values: map[propID]map[int]int{
			{2, 3}: {2: 2, 1: 0, 0: 1}, // auto, always infrared, off
		},
		PanTilt: true,
	},
}

// Picture settings the 700sa doesn't have. The ids are the C200's (chuangmi.camera.046c04
// 2.5 / 2.6); only a model whose spec maps them has them (mapProp), so a 700sa id that may
// mean something else on another model is never written.
var (
	PropWDR           = MiotProp{Siid: 2, Piid: 5} // wide dynamic range: the bright sky and the shade both visible
	PropLowLightColor = MiotProp{Siid: 2, Piid: 6} // glimmer-full-color: colour in low light without the spotlight
)

var modelOnly = map[propID]bool{{2, 5}: true, {2, 6}: true}

// SpecForModel returns the spec of a camera model (nil - not supported: video only).
func SpecForModel(model string) *ModelSpec {
	return modelSpecs[model]
}

func specOf(src string) *ModelSpec {
	return SpecForModel(SourceParam(src, "model"))
}

// CanTurnPicture reports whether the camera turns its own picture (PropImageRollover).
func CanTurnPicture(name string) bool {
	src := CameraSource(name)
	if src == "" {
		return false
	}
	_, ok := specOf(src).mapProp(PropImageRollover)
	return ok
}

// mapProp returns the model's property for a 700sa property (false: the model has none).
func (s *ModelSpec) mapProp(p MiotProp) (MiotProp, bool) {
	if s == nil {
		return p, false
	}
	if s.Props == nil {
		return p, !modelOnly[propID{p.Siid, p.Piid}]
	}
	to, ok := s.Props[propID{p.Siid, p.Piid}]
	if !ok {
		return p, false
	}
	p.Siid, p.Piid = to.Siid, to.Piid
	return p, true
}

// toModel / fromModel translate a value of a 700sa property (ok false: no such value)
func (s *ModelSpec) toModel(p MiotProp, v any) (any, bool) {
	m := s.values(p)
	if m == nil {
		return v, true
	}
	i, isInt := intValue(v)
	if !isInt {
		return v, true
	}
	to, ok := m[i]
	return to, ok
}

func (s *ModelSpec) fromModel(p MiotProp, v any) any {
	m := s.values(p)
	if m == nil {
		return v
	}
	if i, ok := intValue(v); ok {
		for canon, model := range m {
			if model == i {
				return float64(canon) // the cloud answers numbers as JSON floats
			}
		}
	}
	return v
}

func (s *ModelSpec) values(p MiotProp) map[int]int {
	if s == nil || s.Values == nil {
		return nil
	}
	return s.Values[propID{p.Siid, p.Piid}]
}

// hasValue reports whether the model has the 700sa value v of property p.
func (s *ModelSpec) hasValue(p MiotProp, v int) bool {
	if _, ok := s.mapProp(p); !ok {
		return false
	}
	_, ok := s.toModel(p, v)
	return ok
}

func intValue(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case float64:
		return int(x), true
	}
	return 0, false
}
