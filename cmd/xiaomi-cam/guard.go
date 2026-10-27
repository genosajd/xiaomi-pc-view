package main

import (
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

// Back to the watch view: a camera other programs watch from a fixed view (a detector,
// recorders, a broadcast) goes back where it looked before, on the lens it was on, a few
// minutes after the last manual control (turning, zoom, lens, aiming, a point; also through
// the API and AI) – a look around never leaves the watching blind. The way back is
// xccore.GotoPositionSteady: the same pixels within ~2 px when the view was reached that way
// too, else up to the gears' backlash (~50 px at 2560), which a program comparing pictures
// can follow.

const prefWatchReturn = "watch_return_min" // minutes, 0: off

var watchReturnChoices = []int{0, 1, 3, 5, 10, 30}

func watchReturnMinutes() int {
	if appPrefs == nil {
		return 3
	}
	return appPrefs.IntWithFallback(prefWatchReturn, 3)
}

type watchGuard struct {
	mu   sync.Mutex
	rest map[string]restPos    // where each camera stands while nobody controls it
	away map[string]*awayState // cameras under manual control: where they go back to
	// changed - a camera went away, its time moved or it came back (err: the way back failed)
	changed func(name string, err error)
}

type restPos struct {
	pos xccore.Position
	at  time.Time
}

type awayState struct {
	home      xccore.Position
	homeKnown bool
	until     time.Time
	timer     *time.Timer
	returning bool
	again     bool // touched while going back: away again once there
}

func newWatchGuard(changed func(string, error)) *watchGuard {
	return &watchGuard{rest: map[string]restPos{}, away: map[string]*awayState{}, changed: changed}
}

// touch - the camera is being controlled by hand now (from any goroutine).
func (g *watchGuard) touch(name string) {
	n := watchReturnMinutes()
	if n <= 0 || name == "" || !xccore.CanAim(name) {
		return
	}
	g.mu.Lock()
	a := g.away[name]
	if a != nil && a.returning {
		a.again = true
		g.mu.Unlock()
		return
	}
	if a == nil {
		a = &awayState{}
		if r, ok := g.rest[name]; ok && time.Since(r.at) < 3*restEvery {
			a.home, a.homeKnown = r.pos, true
		} else {
			go g.readHome(name, a) // no recent rest position: now, before the camera gets far
		}
		g.away[name] = a
	}
	g.arm(name, a, time.Duration(n)*time.Minute)
	g.mu.Unlock()
	g.notify(name, nil)
}

// arm (re)starts the countdown of a camera (g.mu held).
func (g *watchGuard) arm(name string, a *awayState, d time.Duration) {
	a.until = time.Now().Add(d)
	if a.timer != nil {
		a.timer.Stop()
	}
	a.timer = time.AfterFunc(d, func() { _ = g.goBack(name) })
}

func (g *watchGuard) readHome(name string, a *awayState) {
	p, err := xccore.MotorPosition(name)
	if err != nil {
		return
	}
	g.mu.Lock()
	if !a.homeKnown {
		a.home, a.homeKnown = p, true
	}
	g.mu.Unlock()
}

// goBack turns the camera back to where it stood before the manual control.
func (g *watchGuard) goBack(name string) error {
	g.mu.Lock()
	a := g.away[name]
	if a == nil || a.returning {
		g.mu.Unlock()
		return nil
	}
	if !a.homeKnown {
		delete(g.away, name) // never knew where it stood: nothing to go back to
		g.mu.Unlock()
		g.notify(name, nil)
		return nil
	}
	a.returning, a.again = true, false
	if a.timer != nil {
		a.timer.Stop()
	}
	home := a.home
	g.mu.Unlock()
	g.notify(name, nil)

	err := xccore.GotoPositionSteady(name, home)

	g.mu.Lock()
	a.returning = false
	switch {
	case err != nil: // try again in a minute
		g.arm(name, a, time.Minute)
	case a.again: // controlled again on the way: a new countdown from the view
		a.again = false
		g.rest[name] = restPos{home, time.Now()}
		g.arm(name, a, time.Duration(max(watchReturnMinutes(), 1))*time.Minute)
	default:
		delete(g.away, name)
		g.rest[name] = restPos{home, time.Now()}
	}
	g.mu.Unlock()
	g.notify(name, err)
	return err
}

// state of a camera: away (under manual control), when it goes back, where to.
func (g *watchGuard) state(name string) (away, returning bool, until time.Time, home xccore.Position) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if a := g.away[name]; a != nil {
		return true, a.returning, a.until, a.home
	}
	return false, false, time.Time{}, xccore.Position{}
}

func (g *watchGuard) notify(name string, err error) {
	if g.changed != nil {
		g.changed(name, err)
	}
}

const restEvery = 90 * time.Second

// watchRest notes where each aimable camera stands while nobody controls it, so a manual
// control knows the way back from its very first move (reading the position then would
// come after the motor has started).
func (g *watchGuard) watchRest(names func() []string) {
	time.Sleep(5 * time.Second)
	for {
		for _, name := range names() {
			if watchReturnMinutes() <= 0 || !xccore.CanAim(name) {
				continue
			}
			g.mu.Lock()
			_, away := g.away[name]
			g.mu.Unlock()
			if away {
				continue
			}
			p, err := xccore.MotorPosition(name)
			if err != nil {
				continue
			}
			g.mu.Lock()
			if _, away = g.away[name]; !away {
				g.rest[name] = restPos{p, time.Now()}
			}
			g.mu.Unlock()
		}
		time.Sleep(restEvery)
	}
}
