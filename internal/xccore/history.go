package xccore

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/streams"
	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/AlexxIT/go2rtc/pkg/h264"
	"github.com/AlexxIT/go2rtc/pkg/h265"
	"github.com/AlexxIT/go2rtc/pkg/mp4"
	"github.com/AlexxIT/go2rtc/pkg/xiaomi/miss"
	"github.com/pion/rtp"
)

// The history of a camera for other programs on this machine (the AI assistant of the car
// launcher Kokpit F11 asks "was a car at the gate at 14:30?"), on the local go2rtc API:
//
//	GET api/xc/events?src=<camera>&day=YYYY-MM-DD   the detections of that day (SD card)
//	GET api/xc/history.mp4?src=<camera>&time=<unix> one picture of the recording at that time
//
// The picture is a key frame in a short MP4, like api/frame.mp4; the header X-Frame-Time
// has its real time (unix seconds): the nearest key frame at or after the asked time.

func initHistoryAPI() {
	api.HandleFunc("api/xc/events", handlerEvents)
	api.HandleFunc("api/xc/history.mp4", handlerHistoryFrame)
}

type eventJSON struct {
	Time int64     `json:"time"` // unix seconds
	Kind EventKind `json:"kind"`
}

type detectionJSON struct {
	On     bool `json:"on"`     // alerts on (off: the camera saves no events)
	Person bool `json:"person"` // person detection
	Motion bool `json:"motion"` // motion detection
	Car    bool `json:"car"`    // car detection area
	Fence  bool `json:"fence"`  // virtual fence
}

func handlerEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	name := q.Get("src")
	day, err := time.ParseInLocation("2006-01-02", q.Get("day"), time.Local)
	if err != nil {
		http.Error(w, "day must be YYYY-MM-DD", http.StatusBadRequest)
		return
	}
	if streams.Get(name) == nil {
		http.Error(w, api.StreamNotFound, http.StatusNotFound)
		return
	}
	events, err := SDEvents(name, day)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	out := struct {
		Detection *detectionJSON `json:"detection"` // null: not read (cloud error, other camera)
		Events    []eventJSON    `json:"events"`
	}{Events: []eventJSON{}}
	for _, e := range events {
		out.Events = append(out.Events, eventJSON{e.Time.Unix(), e.Kind})
	}
	if d, err := GetDetection(name); err == nil {
		out.Detection = &detectionJSON{d.On, d.Person, d.Motion, d.Car.Switch, d.Fence.Switch}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func handlerHistoryFrame(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	at, err := strconv.ParseInt(q.Get("time"), 10, 64)
	if err != nil || at <= 0 {
		http.Error(w, "time must be unix seconds", http.StatusBadRequest)
		return
	}
	b, shown, err := HistoryFrame(q.Get("src"), time.Unix(at, 0))
	if err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, errNoCamera) {
			code = http.StatusNotFound
		}
		http.Error(w, err.Error(), code)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "video/mp4")
	h.Set("Content-Length", strconv.Itoa(len(b)))
	h.Set("X-Frame-Time", strconv.FormatInt(shown.Unix(), 10))
	_, _ = w.Write(b)
}

var (
	errNoCamera = errors.New("no such Xiaomi camera")
	histMu      sync.Mutex // one history picture at a time
)

const (
	histSpeed   = 16               // the camera plays the file this fast up to the asked time
	histTimeout = 45 * time.Second // connecting, finding the file and playing up to the time
)

// HistoryFrame returns one picture (a key frame in a short MP4) of the SD card recording
// of a Xiaomi camera at the time at, and the time of that picture.
//
// The camera plays a recording only from the start of its ~1 min file, so the file is
// played fast and the key frames are counted by their timestamps up to the asked time.
// The camera takes one session: its stream switches to the recording and back to live
// video, so whoever watches the camera sees a break of a few seconds.
func HistoryFrame(name string, at time.Time) (mp4File []byte, shown time.Time, err error) {
	src := CameraSource(name)
	if src == "" {
		return nil, time.Time{}, errNoCamera
	}
	if at.After(time.Now().Add(-90 * time.Second)) {
		return nil, time.Time{}, errors.New("this minute is still being recorded: ask for a live picture")
	}
	histMu.Lock()
	defer histMu.Unlock()

	stream := streams.Get(name)
	if stream == nil {
		return nil, time.Time{}, errNoCamera
	}
	// the recording without sound: near the end of a file the fast playback ends before
	// an audio packet comes, and the probe of the source would fail ("no audio")
	u, err := url.Parse(src)
	if err != nil {
		return nil, time.Time{}, err
	}
	q := u.Query()
	q.Set("playback", strconv.FormatInt(at.Unix(), 10))
	q.Set("speed", strconv.Itoa(histSpeed))
	q.Set("audio", "0")
	u.RawQuery = q.Encode()
	stream.SetSource(u.String())
	defer stream.SetSource(src) // back to what it was (live video)

	g := newFrameGrab()
	if err = stream.AddConsumer(g); err != nil {
		return nil, time.Time{}, err
	}
	defer stream.RemoveConsumer(g)

	// the file found by the playback request: its start is the time of the first picture
	start := at.Unix()
	if files := miss.RecordingsBetween(CameraDid(src), at.Unix(), at.Unix()+1); len(files) > 0 {
		start = files[0].Start
	}
	want := float64(at.Unix() - start)

	deadline := time.After(histTimeout)
	var best *grabbed
	for {
		select {
		case f := <-g.frames:
			best = &f
			if f.offset >= want-0.5 {
				return f.data, time.Unix(start, 0).Add(time.Duration(f.offset * float64(time.Second))), nil
			}
		case <-time.After(6 * time.Second):
			// the file ended before the asked time (a gap in the recordings): the last picture
			if best != nil {
				return best.data, time.Unix(start, 0).Add(time.Duration(best.offset * float64(time.Second))), nil
			}
		case <-deadline:
			if best != nil {
				return best.data, time.Unix(start, 0).Add(time.Duration(best.offset * float64(time.Second))), nil
			}
			return nil, time.Time{}, fmt.Errorf("no picture from the recording in %v", histTimeout)
		}
	}
}

// grabbed - a key frame as a complete MP4 and its time from the first frame (seconds of
// the recording, from the packet timestamps of the camera)
type grabbed struct {
	data   []byte
	offset float64
}

// frameGrab - a consumer that turns every video key frame into a one-frame MP4
type frameGrab struct {
	core.Connection
	muxer  *mp4.Muxer
	frames chan grabbed
	first  uint32
	have   bool
}

func newFrameGrab() *frameGrab {
	g := &frameGrab{
		Connection: core.Connection{ID: core.NewID(), FormatName: "mp4"},
		muxer:      &mp4.Muxer{},
		frames:     make(chan grabbed, 64),
	}
	g.Medias = []*core.Media{{
		Kind:      core.KindVideo,
		Direction: core.DirectionSendonly,
		Codecs:    []*core.Codec{{Name: core.CodecH264}, {Name: core.CodecH265}},
	}}
	return g
}

func (g *frameGrab) AddTrack(media *core.Media, _ *core.Codec, track *core.Receiver) error {
	g.muxer.AddTrack(track.Codec)
	init, err := g.muxer.GetInit()
	if err != nil {
		return err
	}
	isKey := h264.IsKeyframe
	if track.Codec.Name == core.CodecH265 {
		isKey = h265.IsKeyframe
	}
	sender := core.NewSender(media, track.Codec)
	sender.Handler = func(packet *rtp.Packet) {
		if !isKey(packet.Payload) {
			return
		}
		if !g.have {
			g.have, g.first = true, packet.Timestamp
		}
		offset := float64(packet.Timestamp-g.first) / 90000 // uint32: right across a wrap
		g.muxer.Reset()
		b := g.muxer.GetPayload(0, packet)
		data := make([]byte, 0, len(init)+len(b))
		data = append(append(data, init...), b...)
		g.Send += len(data)
		select {
		case g.frames <- grabbed{data, offset}:
		default: // nobody reads any more
		}
	}
	if track.Codec.IsRTP() {
		if track.Codec.Name == core.CodecH265 {
			sender.Handler = h265.RTPDepay(track.Codec, sender.Handler)
		} else {
			sender.Handler = h264.RTPDepay(track.Codec, sender.Handler)
		}
	} else if track.Codec.Name == core.CodecH264 {
		sender.Handler = h264.RepairAVCC(track.Codec, sender.Handler)
	}
	sender.HandleRTP(track)
	g.Senders = append(g.Senders, sender)
	return nil
}
