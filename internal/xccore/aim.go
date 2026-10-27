package xccore

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"
)

// Absolute camera positions. The camera has no command to turn to a position, but it turns
// to a stored point (9.2) and zooms to the point's scale: gotoPath stores a temporary
// point at a free index, turns to it and removes it again (the user's points stay as they
// were; Mi Home shows the temporary one for a few seconds at most).

// Position - the motor position (6.12) and the zoom*10 of a camera.
type Position struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Scale int `json:"scale"`
}

// MotorPosition returns where the camera looks now.
func MotorPosition(camera string) (Position, error) {
	if CameraSource(camera) == "" {
		return Position{}, ErrNotXiaomi
	}
	loc, err := motorPosition(camera)
	return Position{loc.X, loc.Y, loc.Scale}, err
}

var aimName = base64.StdEncoding.EncodeToString([]byte("·"))

var errStoppedShort = errors.New("the camera stopped")

var moveLocks sync.Map // camera name -> *sync.Mutex: one turn at a time

// GotoPosition turns the camera to p (scale = zoom*10, 0: the current zoom) and waits
// until it stands there (max 20 s).
func GotoPosition(camera string, p Position) error {
	return gotoPath(camera, []Position{p})
}

// GotoPositionSteady turns to p always ending with a move towards smaller coordinates
// (first past p when p lies further): the gears' backlash then sits on the same side every
// time, and the picture comes back to the same pixels (measured on the CW700S: within 2 px
// at 2560; the same coordinates reached from the other side are ~40-70 px apart).
func GotoPositionSteady(camera string, p Position) error {
	cur, err := MotorPosition(camera)
	if err != nil {
		return err
	}
	m := aimModelOf(camera)
	if p.X > cur.X || p.Y > cur.Y {
		over := p
		if p.X > cur.X {
			over.X += m.BackX
		}
		if p.Y > cur.Y {
			over.Y += m.BackY
		}
		return gotoPath(camera, []Position{over, p})
	}
	return gotoPath(camera, []Position{p})
}

// gotoPath turns the camera to each position in turn through temporary points. A position
// past the end of the motor's range (the camera stops short) is fine on the way; only the
// last one has to be reached. One turn at a time per camera (an aim and the way back would
// write the points over each other).
func gotoPath(camera string, path []Position) error {
	if CameraSource(camera) == "" {
		return ErrNotXiaomi
	}
	lock, _ := moveLocks.LoadOrStore(camera, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	unlock := true
	defer func() {
		if unlock {
			mu.Unlock()
		}
	}()
	for i := range path {
		if path[i].Scale == 0 {
			path[i].Scale = int(CurrentZoom(camera)*10 + 0.5)
		}
	}
	markMoving(camera, time.Minute)
	defer markMoving(camera, movingSettle)
	list, err := GetPresets(camera)
	if err != nil {
		return err
	}
	used := map[int]bool{}
	var keep []Preset
	for _, q := range list {
		if q.Name == aimName { // left over by an earlier turn that didn't finish
			continue
		}
		used[q.Idx] = true
		keep = append(keep, q)
	}
	var free []int
	for i := MaxPresets; i > PresetSlots; i-- { // above the app's points 1-3
		if !used[i] {
			free = append(free, i)
		}
	}
	if len(free) == 0 {
		return fmt.Errorf("all %d camera positions are in use (Mi Home): free one to aim", MaxPresets)
	}
	// each step its own temporary point, all in one write: every cloud call before the motor
	// starts adds ~0.7 s (measured: a turn starts ~2.2 s after the command, the picture shows
	// it ~0.8-1.3 s later). Too few free indexes: one point, written again before each step.
	write := func(steps []Position, idxs []int) error {
		tmp := append([]Preset(nil), keep...)
		for k, p := range steps {
			tmp = append(tmp, Preset{Location: len(tmp) + 1, Idx: idxs[k], Pos: [2]int{p.X, p.Y}, Scale: p.Scale, Name: aimName})
		}
		sort.Slice(tmp, func(i, j int) bool { return tmp[i].Idx < tmp[j].Idx })
		return putPresetsAsIs(camera, tmp)
	}
	together := len(free) >= len(path)
	if together {
		err = write(path, free[:len(path)])
	}
	for i, p := range path {
		idx := free[0]
		if together {
			idx = free[i]
		} else {
			err = write([]Position{p}, free[:1])
		}
		if err == nil {
			err = SetProp(camera, PropPresetActive, idx)
		}
		if err == nil {
			err = waitPosition(camera, p, 20*time.Second)
		}
		if err != nil && i < len(path)-1 && errors.Is(err, errStoppedShort) {
			err = nil // past the end of the range on the way: the next step comes from there
		}
		if err != nil {
			break
		}
	}
	// the user's list back, even when turning failed - after the answer (the camera stands
	// already; the next turn waits for it on the lock, and a leftover point is dropped then)
	unlock = false
	go func() {
		defer mu.Unlock()
		for i := 0; i < 3; i++ {
			if putPresetsAsIs(camera, keep) == nil {
				return
			}
			time.Sleep(time.Second)
		}
	}()
	if err == nil {
		SetZoomState(camera, float64(path[len(path)-1].Scale)/10)
	}
	return err
}

// putPresetsAsIs writes the list without renumbering the locations (PutPresets numbers
// them 1..n, which would move the user's points in Mi Home's order).
func putPresetsAsIs(camera string, list []Preset) error {
	b := []byte("[]")
	if len(list) > 0 {
		b, _ = json.Marshal(list)
	}
	return SetProp(camera, PropPresets, string(b))
}

// waitPosition waits until the camera reports p (or stands still elsewhere: the motor
// can't reach it, e.g. past the end of the pan range).
func waitPosition(camera string, p Position, max time.Duration) error {
	var last Position
	t0, changed := time.Now(), time.Now()
	for time.Since(t0) < max {
		// each read is a cloud call (~0.5 s): asked often, the arrival is seen sooner
		time.Sleep(300 * time.Millisecond)
		now, err := MotorPosition(camera)
		if err != nil {
			continue
		}
		if now.X == p.X && now.Y == p.Y {
			return nil
		}
		if now != last {
			last, changed = now, time.Now()
		} else if time.Since(changed) > 3*time.Second && time.Since(t0) > 5*time.Second {
			// standing elsewhere (the motor starts ~1-2 s after the command: not before 5 s)
			return fmt.Errorf("%w at %d,%d (asked %d,%d)", errStoppedShort, now.X, now.Y, p.X, p.Y)
		}
	}
	return errors.New("the camera didn't reach the position in time")
}

// --- aiming: a point of the picture to the centre of a lens

// aimModel - how a camera model's picture moves with its motor, measured on the CW700S
// (2026-10-26, a 2560x1440 picture at 1x, the picture not turned, each position reached
// the steady way): x 8.24-8.53 px per motor unit over -40..+40 (linear), y -21.7..-22.0
// (+y: the picture moves up), back to the start within 1.5 px three times; the backlash
// is ~5-8 units in x and ~3 in y. The telephoto lens's centre lies at +22,+12 px of the
// wide picture's centre (far objects; parallax adds a little for near ones).
type aimModel struct {
	PxX, PxY     float64 // picture shift per motor unit, px of a 2560x1440 picture at 1x
	BackX, BackY int     // overshoot before the last move (more than the backlash)
	TeleX, TeleY float64 // the telephoto lens's centre in the wide picture, px from its centre
}

var aimModels = map[string]aimModel{
	"isa.camera.700sa": {PxX: 8.4, PxY: -21.8, BackX: 12, BackY: 6, TeleX: 22, TeleY: 12},
}

const aimW, aimH = 2560.0, 1440.0 // the picture the model is measured in

func aimModelOf(camera string) aimModel {
	if m, ok := aimModels[SourceParam(CameraSource(camera), "model")]; ok {
		return m
	}
	return aimModel{BackX: 12, BackY: 6}
}

// CanAim reports whether the camera can be aimed at a point of its picture.
func CanAim(camera string) bool {
	src := CameraSource(camera)
	_, ok := aimModels[SourceParam(src, "model")]
	return src != "" && ok
}

// PictureTurn - the camera's own turn of its picture (2.2: 0/90/180/270).
func PictureTurn(camera string) int {
	if !CanTurnPicture(camera) {
		return 0
	}
	props, err := GetProps(camera, PropImageRollover)
	if err != nil || len(props) == 0 || props[0].Code != 0 {
		return 0
	}
	v, _ := intValue(props[0].Value)
	return v
}

// aimTarget: where to turn (motor position, zoom) so that the point (fx, fy) of the
// picture as shown now (fractions of it from its centre, -0.5..0.5) comes to the centre of
// the lens at zoom zf. cur - the position now, z - the zoom now, tele - the zoom of the
// telephoto lens (0: none), turn - the picture's turn (0 or 180).
func aimTarget(m aimModel, cur Position, z, tele, zf float64, turn int, fx, fy float64) Position {
	s := 1.0
	if turn == 180 { // the picture upside down: everything in it moves the other way
		s = -1
	}
	onTele := func(z float64) bool { return tele > 0 && z >= tele-0.01 }
	// the point in the wide picture at 1x, px from its centre
	px, py := fx*aimW/z, fy*aimH/z
	if onTele(z) {
		px, py = px+s*m.TeleX, py+s*m.TeleY
	}
	// it has to come to the centre of the lens it ends on
	var cx, cy float64
	if onTele(zf) {
		cx, cy = s*m.TeleX, s*m.TeleY
	}
	return Position{
		X:     cur.X + int(math.Round((cx-px)/(s*m.PxX))),
		Y:     max(0, cur.Y+int(math.Round((cy-py)/(s*m.PxY)))),
		Scale: int(zf*10 + 0.5),
	}
}

// AimAt turns the camera so that the point (fx, fy) of its picture as it is now
// (fractions of the picture from its centre, -0.5..0.5) comes to the centre: on the
// telephoto lens (lens "tele"), the wide one ("wide") or the lens it is on ("").
func AimAt(camera string, fx, fy float64, lens string) (Position, error) {
	cur, err := aimFrom(camera)
	if err != nil {
		return Position{}, err
	}
	z := CurrentZoom(camera)
	if cur.Scale >= 10 {
		z = float64(cur.Scale) / 10
	}
	return AimFrom(camera, cur, z, fx, fy, lens)
}

func aimFrom(camera string) (Position, error) {
	if !CanAim(camera) {
		return Position{}, errors.New("this camera can't be aimed at a point")
	}
	return MotorPosition(camera)
}

// AimFrom - AimAt for a picture taken at position from with zoom z (an overview kept
// while the camera looks elsewhere).
func AimFrom(camera string, from Position, z, fx, fy float64, lens string) (Position, error) {
	if !CanAim(camera) {
		return Position{}, errors.New("this camera can't be aimed at a point")
	}
	if math.Abs(fx) > 0.5 || math.Abs(fy) > 0.5 {
		return Position{}, errors.New("the point must be inside the picture (-0.5..0.5 from its centre)")
	}
	tele := TeleZoom(camera)
	zf := CurrentZoom(camera)
	switch lens {
	case "tele":
		if tele > 0 && zf < tele {
			zf = tele
		}
	case "wide":
		zf = 1
	case "":
	default:
		return Position{}, errors.New("lens must be tele, wide or empty")
	}
	p := aimTarget(aimModelOf(camera), from, z, tele, zf, PictureTurn(camera), fx, fy)
	return p, GotoPositionSteady(camera, p)
}

// WideFrame returns where the picture of the camera at position now (zoom z) lies on a
// wide picture taken at position from: its centre and size as fractions of that picture
// (centre from its centre; outside -0.5..0.5: not on it). turn - PictureTurn.
func WideFrame(camera string, from, now Position, z float64, turn int) (cx, cy, w, h float64) {
	s := 1.0
	if turn == 180 {
		s = -1
	}
	cx, cy = widePoint(aimModelOf(camera), from, now, z, TeleZoom(camera), s)
	return cx, cy, 1 / max(z, 1), 1 / max(z, 1)
}

// widePoint: the centre of the picture at position now (zoom z) on the wide picture taken
// at position from, as fractions of that picture from its centre.
func widePoint(m aimModel, from, now Position, z, tele, s float64) (fx, fy float64) {
	// the picture moved by (now - from) units: its centre lies the other way on the old one
	px := -float64(now.X-from.X) * s * m.PxX
	py := -float64(now.Y-from.Y) * s * m.PxY
	if tele > 0 && z >= tele-0.01 {
		px, py = px+s*m.TeleX, py+s*m.TeleY
	}
	return px / aimW, py / aimH
}
