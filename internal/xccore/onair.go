package xccore

import (
	"net/url"
	"strings"

	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/streams"
)

// On air: one RTSP address for a broadcast app (OBS) that always shows the camera chosen
// here, so switching the broadcast camera never touches OBS. Video only: the camera
// microphone is never on air. It re-streams the camera's own stream, so the app and OBS
// share one camera session. The stream (and its go2rtc.yaml entry) is made when a camera
// is first put on air, not at start.

const OnAirStream = HiddenPrefix + "onair"

// OnAirURL - the address for OBS (Media Source).
func OnAirURL() string {
	return RTSPURL(OnAirStream)
}

func onAirSource(name string) string {
	return RTSPURL(name) + "?video"
}

// OnAirCamera returns the camera on air ("" - none).
func OnAirCamera() string {
	s := streams.Get(OnAirStream)
	if s == nil {
		return ""
	}
	for _, src := range s.Sources() {
		u, err := url.Parse(src)
		if err != nil {
			continue
		}
		name, err := url.PathUnescape(strings.TrimPrefix(u.EscapedPath(), "/"))
		if err == nil && name != "" {
			return name
		}
	}
	return ""
}

// SetOnAir puts a camera on air (saved in go2rtc.yaml). A running OBS connection switches
// to the new camera (the source of the stream is replaced).
func SetOnAir(name string) error {
	src := onAirSource(name)
	if s := streams.Get(OnAirStream); s != nil {
		s.SetSourceInPlace(src)
	} else if _, err := streams.New(OnAirStream, src); err != nil {
		return err
	}
	return app.PatchConfig([]string{"streams", OnAirStream}, []string{src})
}

// OffAirIfRemoved takes the broadcast address away when its camera was removed. Nothing is
// put on air by itself: the address exists once a camera is chosen (Settings or the API).
func OffAirIfRemoved() {
	cur := OnAirCamera()
	if cur == "" {
		return
	}
	for _, n := range CameraNames() {
		if n == cur {
			return
		}
	}
	streams.Delete(OnAirStream)
	_ = app.PatchConfig([]string{"streams", OnAirStream}, nil)
}

// RenameOnAir keeps the broadcast on a renamed camera.
func RenameOnAir(oldName, newName string) {
	if OnAirCamera() == oldName {
		_ = SetOnAir(newName)
	}
}
