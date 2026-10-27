// xc-camsim - developer tool: a simulated IP camera for testing Xiaomi Cam without a real
// one. It is go2rtc with a test picture (ffmpeg testsrc): RTSP server with a login, ONVIF
// device service, and MJPEG over HTTP.
//
//	xc-camsim [-rtsp :28554] [-http :21984] [-user admin] [-pass secret]
//
// RTSP: rtsp://admin:secret@127.0.0.1:28554/main (1920x1080 + AAC audio), /sub (640x360)
// and /opus (main with Opus audio, like Xiaomi cameras)
// ONVIF: onvif://admin:secret@127.0.0.1:21984 (profiles main, sub)
// MJPEG: http://127.0.0.1:21984/api/stream.mjpeg?src=mjpeg
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/AlexxIT/go2rtc/internal/api"
	"github.com/AlexxIT/go2rtc/internal/app"
	"github.com/AlexxIT/go2rtc/internal/exec"
	"github.com/AlexxIT/go2rtc/internal/ffmpeg"
	"github.com/AlexxIT/go2rtc/internal/mjpeg"
	"github.com/AlexxIT/go2rtc/internal/onvif"
	"github.com/AlexxIT/go2rtc/internal/rtsp"
	"github.com/AlexxIT/go2rtc/internal/streams"
)

const source = "exec:ffmpeg -hide_banner -re -f lavfi -i testsrc2=size=%s:rate=15 -f lavfi -i sine=frequency=440 " +
	"-c:v libx264 -preset ultrafast -tune zerolatency -g 30 -pix_fmt yuv420p -c:a aac -b:a 64k -f rtsp {output}"

func main() {
	rtspAddr := flag.String("rtsp", ":28554", "RTSP listen address")
	httpAddr := flag.String("http", ":21984", "HTTP (API, ONVIF, MJPEG) listen address")
	user := flag.String("user", "admin", "RTSP login")
	pass := flag.String("pass", "secret", "RTSP password")
	flag.Parse()

	cfg := fmt.Sprintf(`api:
  listen: %q
rtsp:
  listen: %q
  username: %q
  password: %q
log:
  level: warn
streams:
  main: %q
  sub: %q
  mjpeg: "ffmpeg:main#video=mjpeg"
  opus: "ffmpeg:main#video=copy#audio=opus"
`, *httpAddr, *rtspAddr, *user, *pass, fmt.Sprintf(source, "1920x1080"), fmt.Sprintf(source, "640x360"))
	dir, err := os.MkdirTemp("", "xc-camsim")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "go2rtc.yaml")
	if err = os.WriteFile(path, []byte(cfg), 0o600); err != nil {
		panic(err)
	}
	os.Args = []string{os.Args[0], "-config", path}
	app.Init()
	api.Init()
	streams.Init()
	rtsp.Init()
	exec.Init()
	ffmpeg.Init()
	mjpeg.Init()
	onvif.Init()
	fmt.Printf("xc-camsim: rtsp://%s:%s@127.0.0.1%s/main, onvif http%s, mjpeg http://127.0.0.1%s/api/stream.mjpeg?src=mjpeg\n",
		*user, *pass, *rtspAddr, *httpAddr, *httpAddr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
}
