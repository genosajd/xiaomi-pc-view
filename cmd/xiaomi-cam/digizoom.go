package main

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// Digital zoom of the picture, for cameras without their own zoom and beyond the camera's
// zoom (hybrid: the camera zooms first, the picture after it). The crop comes from the
// decoded frame, and while zoomed the video is decoded to a larger tier (decodeSize), up to
// the stream's own size: a 2x zoom of a 1080p camera shows its real 960x540 pixels, not a
// stretched window-sized picture. The GPU scales the crop to the view (smooth).

const (
	maxDigital       = 8.0  // cameras without their own zoom
	maxDigitalHybrid = 4.0  // on top of the camera's zoom
	digitalStep      = 1.25 // one wheel step / key press
)

// digitalZoom is the zoom state of a videoView: 1 - off.
type digitalZoom struct {
	z      float64
	cx, cy float64 // the centre of the crop, 0..1 of the frame
	buf    *image.RGBA

	// the overview in the corner: the whole picture and the part shown
	mini      *canvas.Image
	miniFrame *canvas.Rectangle
	miniBox   *canvas.Rectangle
	miniTick  int
}

func (d *digitalZoom) init() {
	d.z, d.cx, d.cy = 1, 0.5, 0.5
	d.mini = canvas.NewImageFromImage(nil)
	d.mini.FillMode = canvas.ImageFillStretch
	d.mini.ScaleMode = canvas.ImageScaleFastest
	d.miniFrame = canvas.NewRectangle(color.NRGBA{A: 0})
	d.miniFrame.StrokeColor = color.NRGBA{R: 255, G: 255, B: 255, A: 120}
	d.miniFrame.StrokeWidth = 1
	d.miniBox = canvas.NewRectangle(color.NRGBA{A: 0})
	d.miniBox.StrokeColor = color.NRGBA{R: 245, G: 165, B: 36, A: 255}
	d.miniBox.StrokeWidth = 2
	d.showMini(false)
}

func (d *digitalZoom) showMini(on bool) {
	for _, o := range []fyne.CanvasObject{d.mini, d.miniFrame, d.miniBox} {
		if on {
			o.Show()
		} else {
			o.Hide()
		}
	}
}

// cropRect returns the part of a frame of size w x h that is shown.
func (d *digitalZoom) cropRect(w, h int) image.Rectangle {
	cw, ch := max(16, int(float64(w)/d.z)), max(9, int(float64(h)/d.z))
	x0 := int(d.cx*float64(w)) - cw/2
	y0 := int(d.cy*float64(h)) - ch/2
	x0 = min(max(x0, 0), w-cw)
	y0 = min(max(y0, 0), h-ch)
	return image.Rect(x0, y0, x0+cw, y0+ch)
}

// crop copies the shown part of f into the reused buffer (rows of RGBA: no scaling here).
func (d *digitalZoom) crop(f *image.RGBA) *image.RGBA {
	r := d.cropRect(f.Bounds().Dx(), f.Bounds().Dy())
	if d.buf == nil || d.buf.Rect.Dx() != r.Dx() || d.buf.Rect.Dy() != r.Dy() {
		d.buf = image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	}
	row := r.Dx() * 4
	for y := 0; y < r.Dy(); y++ {
		src := f.Pix[(r.Min.Y+y)*f.Stride+r.Min.X*4:]
		copy(d.buf.Pix[y*d.buf.Stride:y*d.buf.Stride+row], src[:row])
	}
	return d.buf
}

// --- videoView: the picture through the digital zoom

// shownRect is where the picture is drawn in the view (ImageFillContain letterbox).
func (v *videoView) shownRect() (x, y, w, h float32) {
	size := v.Size()
	if v.frame == nil || size.Width <= 0 || size.Height <= 0 {
		return 0, 0, size.Width, size.Height
	}
	b := v.frame.Bounds()
	aspect := float32(b.Dx()) / float32(b.Dy())
	w, h = size.Width, size.Width/aspect
	if h > size.Height {
		h, w = size.Height, size.Height*aspect
	}
	return (size.Width - w) / 2, (size.Height - h) / 2, w, h
}

// setDigitalZoom zooms to z keeping the frame point under pos (view coordinates) in place;
// pos nil - around the centre of the view.
func (v *videoView) setDigitalZoom(z float64, pos *fyne.Position) {
	d := &v.dz
	z = max(1, z)
	if v.frame != nil && pos != nil {
		sx, sy, sw, sh := v.shownRect()
		px := float64(min(max((pos.X-sx)/sw, 0), 1)) // 0..1 across the shown picture
		py := float64(min(max((pos.Y-sy)/sh, 0), 1))
		b := v.frame.Bounds()
		r := d.cropRect(b.Dx(), b.Dy())
		fx := (float64(r.Min.X) + px*float64(r.Dx())) / float64(b.Dx()) // the frame point under the mouse
		fy := (float64(r.Min.Y) + py*float64(r.Dy())) / float64(b.Dy())
		d.cx = fx + (0.5-px)/z
		d.cy = fy + (0.5-py)/z
	}
	if z <= 1 {
		d.cx, d.cy = 0.5, 0.5
	}
	d.z = z
	d.clampCenter()
	v.updateImage()
	v.Refresh()
}

func (d *digitalZoom) clampCenter() {
	half := 0.5 / d.z
	d.cx = min(max(d.cx, half), 1-half)
	d.cy = min(max(d.cy, half), 1-half)
}

// panBy moves the zoomed picture with the mouse (view pixels): "grab the picture".
func (v *videoView) panBy(dx, dy float32) {
	_, _, sw, sh := v.shownRect()
	if sw <= 0 || sh <= 0 {
		return
	}
	v.dz.cx -= float64(dx/sw) / v.dz.z
	v.dz.cy -= float64(dy/sh) / v.dz.z
	v.dz.clampCenter()
	v.updateImage()
}

// zoomed: the digital zoom is on (drag pans the picture instead of turning the camera).
func (v *videoView) zoomed() bool { return v.dz.z > 1.001 }

// layoutMini places the overview in the top left corner of the shown picture.
func (v *videoView) layoutMini() {
	sx, sy, sw, sh := v.shownRect()
	mw := min(200, max(110, sw*0.18))
	mh := mw * sh / max(sw, 1)
	mx, my := sx+12, sy+12
	if v.wideOn { // under the overview of the wide lens
		_, wy, _, wh := v.wideArea()
		my = wy + wh + 8
	}
	d := &v.dz
	d.mini.Move(fyne.NewPos(mx, my))
	d.mini.Resize(fyne.NewSize(mw, mh))
	d.miniFrame.Move(fyne.NewPos(mx, my))
	d.miniFrame.Resize(fyne.NewSize(mw, mh))
	if v.frame != nil {
		b := v.frame.Bounds()
		r := d.cropRect(b.Dx(), b.Dy())
		fx, fy := float32(r.Min.X)/float32(b.Dx()), float32(r.Min.Y)/float32(b.Dy())
		fw, fh := float32(r.Dx())/float32(b.Dx()), float32(r.Dy())/float32(b.Dy())
		d.miniBox.Move(fyne.NewPos(mx+fx*mw, my+fy*mh))
		d.miniBox.Resize(fyne.NewSize(max(4, fw*mw), max(3, fh*mh)))
	}
}
