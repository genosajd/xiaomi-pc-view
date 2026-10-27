package main

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// The overview of the wide lens on the picture (aimui.go): the wide picture from before
// the camera zoomed in, in the top left corner, with the frame of where the camera looks
// now. The digital zoom's overview goes under it.

func (v *videoView) initWide() {
	v.wideImg = canvas.NewImageFromImage(nil)
	v.wideImg.FillMode = canvas.ImageFillStretch
	v.wideImg.ScaleMode = canvas.ImageScaleSmooth
	v.wideFrame = canvas.NewRectangle(color.NRGBA{A: 0})
	v.wideFrame.StrokeColor = color.NRGBA{R: 255, G: 255, B: 255, A: 140}
	v.wideFrame.StrokeWidth = 1
	v.wideBox = canvas.NewRectangle(color.NRGBA{A: 0})
	v.wideBox.StrokeColor = color.NRGBA{R: 64, G: 200, B: 255, A: 255}
	v.wideBox.StrokeWidth = 2
	v.hideWide()
}

// showWideView shows the overview img with the frame at centre cx, cy (fractions of the
// picture from its centre) and size w, h.
func (v *videoView) showWideView(img image.Image, cx, cy, w, h float64) {
	v.wideRect = [4]float64{cx, cy, w, h}
	if v.wideImg.Image != img {
		v.wideImg.Image = img
		v.wideImg.Refresh()
	}
	if !v.wideOn {
		v.wideOn = true
		for _, o := range []fyne.CanvasObject{v.wideImg, v.wideFrame, v.wideBox} {
			o.Show()
		}
	}
	v.layoutWide()
	v.layoutMini()
}

func (v *videoView) hideWide() {
	v.wideOn = false
	for _, o := range []fyne.CanvasObject{v.wideImg, v.wideFrame, v.wideBox} {
		o.Hide()
	}
	v.layoutMini()
}

// wideArea - where the overview is drawn (view coordinates): under the camera's own date
// and time line (the top ~4.5% of its picture), which stays readable.
func (v *videoView) wideArea() (x, y, w, h float32) {
	sx, sy, sw, sh := v.shownRect()
	w = min(240, max(130, sw*0.22))
	return sx + 12, sy + max(12, sh*0.065), w, w * sh / max(sw, 1)
}

func (v *videoView) layoutWide() {
	if !v.wideOn {
		return
	}
	x, y, w, h := v.wideArea()
	v.wideImg.Move(fyne.NewPos(x, y))
	v.wideImg.Resize(fyne.NewSize(w, h))
	v.wideFrame.Move(fyne.NewPos(x, y))
	v.wideFrame.Resize(fyne.NewSize(w, h))
	// the frame, clipped to the overview (it can lie partly off the old picture)
	r := v.wideRect
	x0 := max(0, min(1, 0.5+r[0]-r[2]/2))
	x1 := max(0, min(1, 0.5+r[0]+r[2]/2))
	y0 := max(0, min(1, 0.5+r[1]-r[3]/2))
	y1 := max(0, min(1, 0.5+r[1]+r[3]/2))
	v.wideBox.Move(fyne.NewPos(x+float32(x0)*w, y+float32(y0)*h))
	v.wideBox.Resize(fyne.NewSize(max(4, float32(x1-x0)*w), max(3, float32(y1-y0)*h)))
}

// widePointAt: the point of the overview under pos (fractions of it from its centre).
func (v *videoView) widePointAt(pos fyne.Position) (fx, fy float64, ok bool) {
	x, y, w, h := v.wideArea()
	if pos.X < x || pos.Y < y || pos.X > x+w || pos.Y > y+h {
		return 0, 0, false
	}
	return float64((pos.X-x)/w) - 0.5, float64((pos.Y-y)/h) - 0.5, true
}

// framePoint: the point of the camera's picture under pos (fractions of it from its
// centre, through the digital zoom); false - not on the picture.
func (v *videoView) framePoint(pos fyne.Position) (fx, fy float64, ok bool) {
	if v.frame == nil {
		return 0, 0, false
	}
	sx, sy, sw, sh := v.shownRect()
	px, py := float64((pos.X-sx)/sw), float64((pos.Y-sy)/sh)
	if sw <= 0 || sh <= 0 || px < 0 || px > 1 || py < 0 || py > 1 {
		return 0, 0, false
	}
	b := v.frame.Bounds()
	r := v.dz.cropRect(b.Dx(), b.Dy())
	fx = (float64(r.Min.X)+px*float64(r.Dx()))/float64(b.Dx()) - 0.5
	fy = (float64(r.Min.Y)+py*float64(r.Dy()))/float64(b.Dy()) - 0.5
	return fx, fy, true
}
