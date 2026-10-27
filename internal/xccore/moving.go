package xccore

import (
	"sync"
	"time"
)

// The camera is being turned or zoomed by the app: its picture shifts and the motor is heard
// on its own microphone - a program watching the camera (a motion or event detector) can
// pause meanwhile (GET /api/xiaomi-cam/control "moving"). The first pixels of a turn are too
// small for a detector's own "the whole picture moved" check (the gears' backlash: a 10-step
// turn can start with a 5 px shift), and the motor's clunk sounds like a knock.

// movingSettle - after the motor stops: the picture settles (focus, exposure, the lens).
const movingSettle = 4 * time.Second

var movingUntil sync.Map // camera name -> time.Time

// markMoving: the camera moves for at most d from now (a later mark replaces it).
func markMoving(camera string, d time.Duration) {
	movingUntil.Store(camera, time.Now().Add(d))
}

// Moving reports whether the app is turning or zooming the camera (or did a moment ago).
func Moving(camera string) bool {
	v, ok := movingUntil.Load(camera)
	return ok && time.Now().Before(v.(time.Time))
}
