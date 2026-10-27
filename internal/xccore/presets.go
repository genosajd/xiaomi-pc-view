package xccore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Preset positions ("often viewed positions" in Mi Home), stored in the camera, so they are
// shared with the Mi Home app (IP cameras: ONVIF positions in the app config, IPCamera.Points):
//   6.12 - current motor position {"x":637,"y":63,"scale":10}
//   9.1  - list [{"location":1,"idx":1,"pos":[x,y],"scale":10,"name":"<base64>"}], up to 6
//   9.2  - write idx: the camera turns to that position

var (
	PropMotorLocation = MiotProp{Siid: 6, Piid: 12}
	PropPresets       = MiotProp{Siid: 9, Piid: 1}
	PropPresetActive  = MiotProp{Siid: 9, Piid: 2}
)

const MaxPresets = 6

type Preset struct {
	Location int    `json:"location"`
	Idx      int    `json:"idx"`
	Pos      [2]int `json:"pos"`
	Scale    int    `json:"scale,omitempty"`
	Name     string `json:"name"`           // base64 in the camera
	Coll     string `json:"coll,omitempty"` // Imou: the collection point in the camera
}

// Label - decoded name
func (p Preset) Label() string {
	if b, err := base64.StdEncoding.DecodeString(p.Name); err == nil && len(b) > 0 {
		return string(b)
	}
	return fmt.Sprintf("%d", p.Idx)
}

func GetPresets(camera string) ([]Preset, error) {
	if ip := IPCam(camera); ip != nil {
		list := append([]Preset(nil), ip.Points...)
		sort.Slice(list, func(i, j int) bool { return list[i].Location < list[j].Location })
		return list, nil
	}
	props, err := GetProps(camera, PropPresets)
	if err != nil {
		return nil, err
	}
	if len(props) == 0 {
		return nil, errors.New("no answer from the camera")
	}
	s, _ := props[0].Value.(string)
	var list []Preset
	if strings.TrimSpace(s) != "" {
		if err = json.Unmarshal([]byte(s), &list); err != nil {
			return nil, fmt.Errorf("presets: %w", err)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Location < list[j].Location })
	return list, nil
}

func PutPresets(camera string, list []Preset) error {
	for i := range list {
		list[i].Location = i + 1
	}
	if ip := IPCam(camera); ip != nil {
		ip.Points = list
		return SaveIPCamera(camera, ip)
	}
	b, _ := json.Marshal(list)
	if len(list) == 0 {
		b = []byte("[]")
	}
	return SetProp(camera, PropPresets, string(b))
}

func DeletePreset(camera string, idx int) error {
	list, err := GetPresets(camera)
	if err != nil {
		return err
	}
	out := list[:0]
	for _, p := range list {
		if p.Idx != idx {
			out = append(out, p)
		} else if ic := imouPoints(camera); ic != nil {
			if err := ic.deletePoint(p); err != nil {
				return err
			}
		}
	}
	return PutPresets(camera, out)
}

// GotoPreset turns the camera to a stored position.
func GotoPreset(camera string, idx int) error {
	markMoving(camera, 15*time.Second) // a point can be half a turn away (GotoPresetAndWait ends it sooner)
	if IPCam(camera) != nil {
		_, err := gotoIPPoint(camera, idx)
		return err
	}
	return SetProp(camera, PropPresetActive, idx)
}

// gotoIPPoint turns an IP camera to its point idx (Imou: the collection point; the ONVIF
// services are nil then).
func gotoIPPoint(camera string, idx int) (*onvifCam, error) {
	list, _ := GetPresets(camera)
	for _, p := range list {
		if p.Idx != idx {
			continue
		}
		if ic := imouPoints(camera); ic != nil {
			return nil, ic.gotoPoint(p)
		}
		c, err := ipControl(camera)
		if err != nil {
			return nil, err
		}
		return c, c.moveTo(float64(p.Pos[0])/pointScale, float64(p.Pos[1])/pointScale)
	}
	return nil, fmt.Errorf("no point %d", idx)
}

// PointReached: the camera turned to the point p, and zoomed to its zoom (scale = zoom*10):
// the zoom shown in the app follows.
func PointReached(camera string, p Preset) {
	if p.Scale >= 10 {
		SetZoomState(camera, float64(p.Scale)/10)
	}
}

// GotoPresetAndWait turns to a position and waits until the camera stops (the motor
// position doesn't change), max 15 s.
func GotoPresetAndWait(camera string, idx int) error {
	if IPCam(camera) != nil {
		c, err := gotoIPPoint(camera, idx)
		if err != nil {
			return err
		}
		if c == nil {
			time.Sleep(imouTurnTime) // Imou: no position to watch
		} else {
			c.waitStill()
		}
		return nil
	}
	if err := GotoPreset(camera, idx); err != nil {
		return err
	}
	defer markMoving(camera, movingSettle)
	var last string
	for i := 0; i < 15; i++ {
		time.Sleep(time.Second)
		props, err := GetProps(camera, PropMotorLocation)
		if err != nil || len(props) == 0 {
			continue
		}
		s, _ := props[0].Value.(string)
		if s == last && i >= 1 {
			return nil
		}
		last = s
	}
	return nil
}

// UI, API and MCP use 3 points; point n is the Preset with idx n.
const PresetSlots = 3

// SavePresetSlot stores the current position as point slot (1-3) with a name.
func SavePresetSlot(camera string, slot int, name string) error {
	if slot < 1 || slot > PresetSlots {
		return fmt.Errorf("point must be 1-%d", PresetSlots)
	}
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("%d", slot)
	}
	if ic := imouPoints(camera); ic != nil {
		return ic.savePoint(camera, slot, strings.TrimSpace(name))
	}
	loc, err := motorPosition(camera)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("%d", slot)
	}
	list, err := GetPresets(camera)
	if err != nil {
		return err
	}
	p := Preset{Idx: slot, Pos: [2]int{loc.X, loc.Y}, Scale: loc.Scale, Name: base64.StdEncoding.EncodeToString([]byte(name))}
	out := []Preset{}
	for _, q := range list {
		if q.Idx != slot {
			out = append(out, q)
		}
	}
	if len(out) >= MaxPresets {
		return fmt.Errorf("the camera keeps up to %d positions", MaxPresets)
	}
	out = append(out, p)
	sort.Slice(out, func(i, j int) bool { return out[i].Idx < out[j].Idx })
	return PutPresets(camera, out)
}

type motorLocation struct{ X, Y, Scale int }

// motorPosition - the current position (Xiaomi: MIoT 6.12, IP cameras: ONVIF * pointScale)
func motorPosition(camera string) (loc motorLocation, err error) {
	if IPCam(camera) != nil {
		c, err := ipControl(camera)
		if err != nil {
			return loc, err
		}
		x, y, err := c.position()
		if err != nil {
			return loc, err
		}
		p := pointOf(x, y)
		return motorLocation{X: p[0], Y: p[1]}, nil
	}
	props, err := GetProps(camera, PropMotorLocation)
	if err != nil {
		return loc, err
	}
	if len(props) == 0 {
		return loc, errors.New("no position from the camera")
	}
	s, _ := props[0].Value.(string)
	if err = json.Unmarshal([]byte(s), &loc); err != nil {
		return loc, fmt.Errorf("position: %w (%s)", err, s)
	}
	return loc, nil
}

// RenamePresetSlot changes the name of point slot (1-3); its position stays.
func RenamePresetSlot(camera string, slot int, name string) error {
	list, err := GetPresets(camera)
	if err != nil {
		return err
	}
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("%d", slot)
	}
	name = strings.TrimSpace(name)
	for i := range list {
		if list[i].Idx == slot {
			if ic := imouPoints(camera); ic != nil {
				coll, err := ic.renamePoint(list[i], name)
				if err != nil {
					return err
				}
				list[i].Coll = coll
			}
			list[i].Name = base64.StdEncoding.EncodeToString([]byte(name))
			return PutPresets(camera, list)
		}
	}
	return fmt.Errorf("no point %d", slot)
}

// PresetSlotsOf returns points 1-3 (nil - not defined).
func PresetSlotsOf(list []Preset) [PresetSlots]*Preset {
	var slots [PresetSlots]*Preset
	for i := range list {
		if n := list[i].Idx; n >= 1 && n <= PresetSlots {
			slots[n-1] = &list[i]
		}
	}
	return slots
}

// FindPoint - point by number 1-3 or by name
func FindPoint(camera, key string) (Preset, error) {
	list, err := GetPresets(camera)
	if err != nil {
		return Preset{}, err
	}
	key = strings.TrimSpace(key)
	slots := PresetSlotsOf(list)
	var n int
	if _, err = fmt.Sscan(key, &n); err == nil && n >= 1 && n <= PresetSlots && slots[n-1] != nil {
		return *slots[n-1], nil
	}
	for _, p := range slots {
		if p != nil && strings.EqualFold(p.Label(), key) {
			return *p, nil
		}
	}
	return Preset{}, fmt.Errorf("no point %q", key)
}
