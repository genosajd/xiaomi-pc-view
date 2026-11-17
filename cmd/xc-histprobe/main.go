// xc-histprobe - developer tool: the events of a day and pictures of the SD card recordings
// of a camera through the local API of the core (api/xc/events, api/xc/history.mp4), with
// its own go2rtc: stop the app first.
//
//	xc-histprobe <camera> <day YYYY-MM-DD> [<HH:MM:SS> ...]   pictures → ./hist-<HHMMSS>.mp4
package main

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/AlexxIT/go2rtc/internal/xccore"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("usage: xc-histprobe <camera> <day> [<time> ...]")
		os.Exit(2)
	}
	args := os.Args // Start replaces them with the go2rtc ones
	dir, _ := os.UserConfigDir()
	if err := xccore.Start(filepath.Join(dir, "xiaomi-cam-viewer")); err != nil {
		panic(err)
	}
	time.Sleep(500 * time.Millisecond)
	name, day := args[1], args[2]
	get := func(path string, q url.Values) (*http.Response, []byte) {
		t0 := time.Now()
		r, err := http.Get("http://127.0.0.1:11984/" + path + "?" + q.Encode())
		if err != nil {
			panic(err)
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(r.Body)
		fmt.Printf("%s %v: HTTP %d, %d bytes, %v\n", path, q, r.StatusCode, len(b), time.Since(t0).Round(time.Millisecond))
		return r, b
	}
	_, b := get("api/xc/events", url.Values{"src": {name}, "day": {day}})
	if len(b) > 1500 {
		b = append(b[:1500], "…"...)
	}
	fmt.Println(string(b))
	for _, s := range args[3:] {
		t, err := time.ParseInLocation("2006-01-02 15:04:05", day+" "+s, time.Local)
		if err != nil {
			panic(err)
		}
		r, b := get("api/xc/history.mp4", url.Values{"src": {name}, "time": {fmt.Sprint(t.Unix())}})
		if r.StatusCode != 200 {
			fmt.Println(string(b))
			continue
		}
		var shown int64
		fmt.Sscan(r.Header.Get("X-Frame-Time"), &shown)
		fmt.Printf("asked %s, picture of %s\n", s, time.Unix(shown, 0).Format("15:04:05"))
		_ = os.WriteFile("hist-"+t.Format("150405")+".mp4", b, 0o600)
	}
}
