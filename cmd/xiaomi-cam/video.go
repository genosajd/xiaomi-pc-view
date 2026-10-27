package main

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// --- player: go2rtc RTSP -> ffmpeg -> raw RGBA frames (+ audio to PulseAudio/PipeWire)

type player struct {
	gen atomic.Int64 // generation whose frames are shown; frames of other playbacks are dropped

	mu      sync.Mutex
	seq     int64
	cur     *playback // shown
	pending *playback // seamless switch: replaces cur when it delivers its first frame

	onFrame  func(img *image.RGBA, gen int64)
	onStatus func(text string)
	onEnd    func(gen int64, err error) // a file played to its end or failed (startFile)

	enhance atomic.Bool // the next playback sharpens and denoises (digital zoom: enhance.go)
}

type playback struct {
	gen     int64
	cancel  context.CancelFunc
	done    chan struct{}
	enhance bool
	file    *fileOpts // a recording (archive.go), nil: the live stream
}

// fileOpts - where and how fast a recording plays.
type fileOpts struct {
	seek float64 // seconds from the file start
	rate float64 // 1: real time
}

// fileFPS - the frames a second shown from a recording (played faster, the rest are dropped
// by ffmpeg: 16x of a 20 fps camera would be 320 frames a second).
const fileFPS = 20

func (pb *playback) stop() {
	pb.cancel()
	<-pb.done
}

// start plays url with frames of given size; it reconnects until stop is called.
// The previous playback is stopped first, so the camera session is closed (needed when the
// stream source changes, e.g. quality).
func (p *player) start(url string, w, h int, audio bool) {
	p.stop()
	p.begin(url, w, h, audio, false, nil)
}

// startFile plays a recording (an MP4 over HTTP) once, from seek seconds, rate times faster
// than real time; at its end onEnd is called instead of reconnecting.
func (p *player) startFile(url string, seek, rate float64, w, h int) {
	p.stop()
	p.begin(url, w, h, false, false, &fileOpts{seek: seek, rate: max(rate, 1)})
}

// switchTo starts a new playback while the current one keeps running and replaces it
// when the new one delivers the first frame. Both read the same go2rtc stream, so the
// camera session stays open: no reconnect, no black screen (sound on/off, decode size).
func (p *player) switchTo(url string, w, h int, audio bool) {
	p.mu.Lock()
	running := p.cur != nil
	p.mu.Unlock()
	p.begin(url, w, h, audio, running, nil)
}

func (p *player) begin(url string, w, h int, audio, seamless bool, file *fileOpts) {
	ctx, cancel := context.WithCancel(context.Background())
	p.mu.Lock()
	p.seq++
	pb := &playback{gen: p.seq, cancel: cancel, done: make(chan struct{}), enhance: p.enhance.Load(), file: file}
	oldPending := p.pending
	p.pending = nil
	if seamless {
		p.pending = pb
	} else {
		p.cur = pb
		p.gen.Store(pb.gen)
	}
	p.mu.Unlock()
	if oldPending != nil {
		go oldPending.stop()
	}

	status := func(text string) {
		if p.gen.Load() == pb.gen { // a pending playback doesn't cover the picture with "connecting"
			p.onStatus(text)
		}
	}
	go func() {
		defer close(pb.done)
		for ctx.Err() == nil {
			if file != nil {
				status(T("Wczytuję nagranie…"))
			} else {
				status(T("Łączenie z kamerą…"))
			}
			err := p.run(ctx, url, w, h, audio, pb)
			if ctx.Err() != nil {
				return
			}
			if file != nil { // played once: the caller goes on (the next part, an error)
				if p.onEnd != nil {
					p.onEnd(pb.gen, err)
				}
				return
			}
			status(fmt.Sprintf(T("Rozłączono (%v) – ponawiam za 3 s…"), err))
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
			}
		}
	}()
}

// frame is called by the reader of pb; the first frame of a pending playback promotes it.
func (p *player) frame(pb *playback, img *image.RGBA) {
	if p.gen.Load() != pb.gen {
		p.mu.Lock()
		if p.pending != pb {
			p.mu.Unlock()
			return // replaced or stopped
		}
		old := p.cur
		p.cur, p.pending = pb, nil
		p.gen.Store(pb.gen)
		p.mu.Unlock()
		if old != nil {
			go old.stop()
		}
	}
	p.onFrame(img, pb.gen)
}

func (p *player) stop() {
	p.mu.Lock()
	cur, pending := p.cur, p.pending
	p.cur, p.pending = nil, nil
	p.mu.Unlock()
	p.gen.Add(1 << 32) // no frames of these playbacks are shown any more
	for _, pb := range []*playback{cur, pending} {
		if pb != nil {
			pb.stop()
		}
	}
}

func (p *player) run(ctx context.Context, url string, w, h int, audio bool, pb *playback) error {
	input, filter, gpu := videoArgs(w, h, pb.enhance)
	args := []string{
		"-hide_banner", "-loglevel", "error", "-nostdin",
		"-fflags", "nobuffer", "-flags", "low_delay",
		"-threads", "2", // decoder threads: default is one per CPU core, each with own frame buffers
		"-filter_threads", "1",
		// a short stream analysis: the picture ~1-2 s sooner (the codecs are in the RTSP SDP);
		// 30 s timeout: finding an SD card recording can take a while
		"-analyzeduration", "500000", "-probesize", "500000",
	}
	args = append(args, input...)
	if f := pb.file; f != nil {
		// read at the playback speed from the position; faster: fewer frames shown
		args = append(args, "-readrate", fmt.Sprintf("%g", f.rate), "-ss", fmt.Sprintf("%.3f", f.seek), "-i", url)
		// a constant frame rate out (faster: frames dropped): each frame is rate/fileFPS
		// seconds of the recording, so the frames shown count the position exactly
		filter = fmt.Sprintf("setpts=PTS/%g,fps=%d,", f.rate, fileFPS) + filter
	} else {
		args = append(args, "-rtsp_transport", "tcp", "-timeout", "30000000", "-i", url)
	}
	args = append(args,
		"-map", "0:v:0", "-vf", filter,
		"-pix_fmt", "rgba", "-f", "rawvideo", "pipe:1",
	)
	if audio {
		args = append(args, "-map", "0:a:0?", "-f", "pulse", "-name", "Xiaomi Cam", "default")
	}
	cmd := toolCommand(ctx, "ffmpeg", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	started := time.Now()
	if err = cmd.Start(); err != nil {
		return fmt.Errorf(T(ffmpegMissing), err)
	}

	// ring of buffers, so the frame being drawn is not overwritten
	const n = 3
	var frames [n]*image.RGBA
	for i := range frames {
		frames[i] = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	r := stdout // frames are read whole with io.ReadFull, no extra buffer
	first := true
	for i := 0; ; i = (i + 1) % n {
		if _, err = io.ReadFull(r, frames[i].Pix); err != nil {
			break
		}
		if first {
			first = false
			histDebug("ffmpeg: first frame %v after start", time.Since(started))
		}
		p.frame(pb, frames[i])
	}
	_ = cmd.Wait()
	if first && ctx.Err() == nil && gpuError(stderr.String()) {
		for _, g := range gpu {
			g.failed() // the GPU didn't give a picture: the CPU from the next try
		}
	}
	if pb.file != nil && !first {
		return nil // a recording played to its end (stderr: only the decoder's warnings)
	}
	if msg := strings.TrimSpace(stderr.String()); msg != "" {
		if lines := strings.Split(msg, "\n"); len(lines) > 0 {
			return fmt.Errorf("%s", lines[len(lines)-1])
		}
	}
	return err
}

// --- video view widget: mouse wheel - camera zoom, drag - pan/tilt, edges - window resize

type videoView struct {
	widget.BaseWidget

	img    *canvas.Image
	bg     *canvas.Rectangle
	status *canvas.Text
	// spinner above the status text: connecting, searching for a recording
	spinner *spinner

	frame *image.RGBA
	small bool // a grid tile: smaller minimum size
	dz    digitalZoom

	onDoubleTap     func()
	onTap           func() // click: select the tile in the grid
	onSecondary     func()
	onMouseMove     func()
	onCameraDrag    func(dx, dy float32)             // dragging the picture moves the camera
	onCameraDragEnd func()                           // mouse released: stop the camera
	onScroll        func(up bool, pos fyne.Position) // mouse wheel - zoom (camera, then digital)
	// a click to aim the camera at (fractions of the picture from its centre): true - done
	// (Ctrl+click or the aim mode, aimui.go)
	onAimTap  func(fx, fy float64) bool
	onWideTap func(fx, fy float64) // a click on the overview of the wide lens
	aimMode   bool                 // the next click aims: crosshair cursor
	ctrlDown  bool                 // the click started with Ctrl held

	// the wide picture kept while the camera is on its telephoto lens, with the lens's frame
	wideImg            *canvas.Image
	wideFrame, wideBox *canvas.Rectangle
	wideRect           [4]float64 // the frame: centre (from the picture's centre) and size, fractions
	wideOn             bool

	// window resize by edges (window without frame)
	canResize   func() bool
	onEdgePress func(dir int) bool   // window manager resize, wm* direction; false - not available
	onResize    func(dx, dy float32) // fallback: resize by our own drag (right/bottom edges)
	edge        int                  // wm* direction under the mouse, -1 - none
	dragging    bool                 // drag in progress
	dragEdge    int                  // edge grabbed when the drag started
	wmGrab      bool                 // the window manager is resizing, ignore drags
}

const edgeSize = 12 // px from the window edge that grab the border

// edgeAt returns the wm* resize direction for a position, or -1.
func (v *videoView) edgeAt(pos fyne.Position) int {
	if v.canResize == nil || !v.canResize() {
		return -1
	}
	size := v.Size()
	left, right := pos.X < edgeSize, pos.X >= size.Width-edgeSize
	top, bottom := pos.Y < edgeSize, pos.Y >= size.Height-edgeSize
	switch {
	case top && left:
		return wmTopLeft
	case top && right:
		return wmTopRight
	case bottom && left:
		return wmBottomLeft
	case bottom && right:
		return wmBottomRight
	case top:
		return wmTop
	case bottom:
		return wmBottom
	case left:
		return wmLeft
	case right:
		return wmRight
	}
	return -1
}

// Cursor shows resize arrows over the edges of a window without frame.
func (v *videoView) Cursor() desktop.Cursor {
	if v.aimMode && v.edge < 0 {
		return desktop.CrosshairCursor
	}
	switch v.edge {
	case wmLeft, wmRight:
		return desktop.HResizeCursor
	case wmTop, wmBottom:
		return desktop.VResizeCursor
	case wmTopLeft, wmBottomRight:
		return desktop.NWSEResizeCursor
	case wmTopRight, wmBottomLeft:
		return desktop.NESWResizeCursor
	}
	return desktop.DefaultCursor
}

func (v *videoView) MouseDown(e *desktop.MouseEvent) {
	v.wmGrab = false
	v.ctrlDown = e.Modifier&fyne.KeyModifierControl != 0
	if e.Button != desktop.MouseButtonPrimary {
		return
	}
	if dir := v.edgeAt(e.Position); dir >= 0 && v.onEdgePress != nil && v.onEdgePress(dir) {
		v.wmGrab = true
	}
}

func (v *videoView) MouseUp(*desktop.MouseEvent) {}

func newVideoView() *videoView {
	v := &videoView{edge: -1}
	v.img = canvas.NewImageFromImage(nil)
	v.img.FillMode = canvas.ImageFillContain
	v.img.ScaleMode = canvas.ImageScaleSmooth
	v.bg = canvas.NewRectangle(color.Black)
	v.status = canvas.NewText("", color.Gray{Y: 200})
	v.status.Alignment = fyne.TextAlignCenter
	v.spinner = newSpinner(videoSpinnerFrames)
	v.dz.init()
	v.initWide()
	v.ExtendBaseWidget(v)
	return v
}

func (v *videoView) CreateRenderer() fyne.WidgetRenderer {
	return &videoRenderer{v: v}
}

// setFrame must be called from the UI goroutine.
func (v *videoView) setFrame(f *image.RGBA) {
	v.frame = f
	if v.status.Text != "" {
		v.status.Text = ""
		v.status.Refresh()
		v.showSpinner(false)
	}
	v.updateImage()
}

// setStatus shows a text over the picture with a spinner (the player is connecting);
// "" hides both.
func (v *videoView) setStatus(text string) {
	v.status.Text = text
	v.status.Refresh()
	v.showSpinner(text != "")
	if text != "" && v.frame == nil {
		v.img.Image = nil
		v.img.Refresh()
	}
}

// clear removes the picture and shows the text; busy - with the spinner.
func (v *videoView) clear(text string, busy bool) {
	v.frame = nil
	v.setStatus(text)
	v.showSpinner(busy)
}

func (v *videoView) showSpinner(on bool) {
	v.spinner.run(on)
}

func (v *videoView) updateImage() {
	if v.frame == nil {
		return
	}
	if !v.zoomed() {
		v.img.Image = v.frame
		v.setScaleMode(v.frame.Rect.Dx())
		v.img.Refresh()
		if v.dz.mini.Visible() {
			v.dz.showMini(false)
		}
		return
	}
	v.img.Image = v.dz.crop(v.frame)
	v.setScaleMode(v.img.Image.Bounds().Dx())
	v.img.Refresh()
	// the overview: a few times a second is enough (each update uploads the whole frame)
	if v.dz.miniTick++; v.dz.miniTick%6 == 0 || !v.dz.mini.Visible() {
		v.dz.mini.Image = v.frame
		v.dz.mini.Refresh()
	}
	v.dz.showMini(true)
	v.layoutMini()
}

// setScaleMode picks who scales a frame w pixels wide to the screen. Fyne's smooth mode
// scales every frame on the CPU (Catmull-Rom, then a copy into RGBA: ~0.7 of a core for
// 1080p at 20 fps); the fastest mode uploads the frame as it is and the GPU scales it
// (bilinear). The video is decoded at the size shown (decodeSize), so the GPU nearly always
// shows it 1:1; only a frame much larger than the view (a grid tile, a window shrinking
// before the decoder follows) keeps the CPU filter, which bilinear would alias.
func (v *videoView) setScaleMode(w int) {
	mode := canvas.ImageScaleFastest
	_, _, sw, _ := v.shownRect()
	if c := fyne.CurrentApp().Driver().CanvasForObject(v); c != nil && float32(w) > sw*c.Scale()*1.3 {
		mode = canvas.ImageScaleSmooth
	}
	v.img.ScaleMode = mode
}

func (v *videoView) Scrolled(e *fyne.ScrollEvent) {
	if v.onScroll != nil && e.Scrolled.DY != 0 {
		v.onScroll(e.Scrolled.DY > 0, e.Position)
	}
}

// Dragged moves the camera ("grab the picture") or resizes the window by its edge.
func (v *videoView) Dragged(e *fyne.DragEvent) {
	dx, dy := e.Dragged.DX, e.Dragged.DY
	if v.wmGrab {
		return
	}
	if !v.dragging {
		v.dragging = true
		v.dragEdge = v.edgeAt(e.Position.Subtract(fyne.NewPos(dx, dy)))
	}
	switch v.dragEdge { // fallback when the window manager can't resize
	case wmRight:
		v.onResize(dx, 0)
		return
	case wmBottom:
		v.onResize(0, dy)
		return
	case wmBottomRight:
		v.onResize(dx, dy)
		return
	}
	if v.zoomed() && (dx != 0 || dy != 0) {
		v.panBy(dx, dy) // zoomed in: the drag moves around the picture
		return
	}
	if v.onCameraDrag != nil && (dx != 0 || dy != 0) {
		v.onCameraDrag(dx, dy)
	}
}

func (v *videoView) DragEnd() {
	v.dragging = false
	v.wmGrab = false
	if v.onCameraDragEnd != nil {
		v.onCameraDragEnd()
	}
}

func (v *videoView) Tapped(e *fyne.PointEvent) {
	ctrl := v.ctrlDown
	v.ctrlDown = false
	if v.wideOn && v.onWideTap != nil {
		if fx, fy, ok := v.widePointAt(e.Position); ok {
			v.onWideTap(fx, fy)
			return
		}
	}
	if (ctrl || v.aimMode) && v.onAimTap != nil {
		v.ctrlDown = ctrl
		fx, fy, ok := v.framePoint(e.Position)
		handled := ok && v.onAimTap(fx, fy)
		v.ctrlDown = false
		if handled {
			return
		}
	}
	if v.onTap != nil {
		v.onTap()
	}
}

func (v *videoView) TappedSecondary(*fyne.PointEvent) {
	if v.onSecondary != nil {
		v.onSecondary()
	}
}

func (v *videoView) MouseIn(e *desktop.MouseEvent) { v.MouseMoved(e) }
func (v *videoView) MouseMoved(e *desktop.MouseEvent) {
	v.edge = v.edgeAt(e.Position)
	v.mouseMoved()
}
func (v *videoView) MouseOut() {}

func (v *videoView) mouseMoved() {
	if v.onMouseMove != nil {
		v.onMouseMove()
	}
}

func (v *videoView) DoubleTapped(*fyne.PointEvent) {
	if v.onDoubleTap != nil {
		v.onDoubleTap()
	}
}

type videoRenderer struct{ v *videoView }

func (r *videoRenderer) Layout(size fyne.Size) {
	for _, o := range r.Objects() {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
	r.v.status.Move(fyne.NewPos(0, size.Height/2-10))
	r.v.status.Resize(fyne.NewSize(size.Width, 20))
	const sp = 32
	r.v.spinner.Resize(fyne.NewSize(sp, sp))
	r.v.spinner.Move(fyne.NewPos((size.Width-sp)/2, size.Height/2-20-sp))
	r.v.layoutWide()
	r.v.layoutMini()
}

func (r *videoRenderer) MinSize() fyne.Size {
	if r.v.small {
		return fyne.NewSize(160, 90) // a grid tile
	}
	return fyne.NewSize(480, 270)
}
func (r *videoRenderer) Refresh() { canvas.Refresh(r.v) }
func (r *videoRenderer) Destroy() {}
func (r *videoRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.v.bg, r.v.img, r.v.dz.mini, r.v.dz.miniFrame, r.v.dz.miniBox,
		r.v.wideImg, r.v.wideFrame, r.v.wideBox, r.v.spinner, r.v.status}
}
