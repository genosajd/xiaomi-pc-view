package main

import (
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
)

// The overview of the wide lens: in the top left corner, the frame where asked, a click on
// it gives the point of the wide picture, a click on the picture the point of the picture.
// XC_WIDESHOT=<dir> with tele.jpg and wide.jpg in it saves the view with the overview.
func TestWideOverview(t *testing.T) {
	a := test.NewTempApp(t)
	a.Settings().SetTheme(appTheme{})
	v := newVideoView()
	w := test.NewWindow(v)
	defer w.Close()
	w.Resize(fyne.NewSize(960, 540))
	frame := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	wide := image.NewRGBA(image.Rect(0, 0, 480, 270))
	dir := os.Getenv("XC_WIDESHOT")
	if dir != "" {
		frame, wide = loadRGBA(t, filepath.Join(dir, "tele.jpg")), shrink(loadRGBA(t, filepath.Join(dir, "wide.jpg")), 480)
	}
	v.setFrame(frame)
	v.showWideView(wide, 0.148, 0.060, 1/2.8, 1/2.8)

	x, y, ow, oh := v.wideArea()
	if x < 0 || y < 0 || ow < 130 || oh < 70 {
		t.Fatalf("overview at %v,%v size %vx%v", x, y, ow, oh)
	}
	// the frame's centre is where asked
	fc := v.wideBox.Position().Add(fyne.NewPos(v.wideBox.Size().Width/2, v.wideBox.Size().Height/2))
	fx, fy, ok := v.widePointAt(fc)
	if !ok || abs64(fx-0.148) > 0.02 || abs64(fy-0.060) > 0.02 {
		t.Errorf("frame centre at %.3f,%.3f (%v), want 0.148,0.060", fx, fy, ok)
	}
	// a click outside the overview is a point of the picture: the centre of the view
	px, py, ok := v.framePoint(fyne.NewPos(480, 270))
	if !ok || abs64(px) > 0.01 || abs64(py) > 0.01 {
		t.Errorf("the view's centre is %.3f,%.3f of the picture (%v)", px, py, ok)
	}
	if _, _, ok = v.widePointAt(fyne.NewPos(480, 270)); ok {
		t.Error("the view's centre is on the overview")
	}
	v.hideWide()
	if v.wideImg.Visible() || v.wideOn {
		t.Error("overview still shown")
	}
	if dir != "" {
		v.showWideView(wide, 0.148, 0.060, 1/2.8, 1/2.8)
		f, err := os.Create(filepath.Join(dir, "overview.png"))
		if err != nil {
			t.Fatal(err)
		}
		_ = png.Encode(f, w.Canvas().Capture())
		_ = f.Close()
	}
}

func abs64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func loadRGBA(t *testing.T, path string) *image.RGBA {
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx()/2, b.Dy()/2))
	big := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(big, big.Rect, img, b.Min, draw.Src)
	return shrinkTo(big, out)
}

// shrinkTo - half size (the frames are 2560 wide; the view decodes ~1280)
func shrinkTo(src, dst *image.RGBA) *image.RGBA {
	s := shrink(src, dst.Rect.Dx())
	return s
}
