package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The lens control follows the camera zoom: Tele from the telephoto lens's zoom on.
// XC_LENSSHOT=<dir> also saves the zoom controls with it (Hebrew, the app's look).
func TestLensControlFollowsTheZoom(t *testing.T) {
	a := test.NewTempApp(t)
	a.Settings().SetTheme(appTheme{})
	old := lang
	lang = "he"
	defer func() { lang = old }()

	u := &ui{}
	u.caps.Tele = 2.8
	lens := u.buildLens()
	for _, c := range []struct {
		z    float64
		want int
	}{{1, 0}, {2.5, 0}, {2.8, 1}, {4, 1}, {1, 0}} {
		u.showLens(c.z)
		if lens.Selected != lens.Options[c.want] {
			t.Errorf("zoom %.1f: %q selected, want %q", c.z, lens.Selected, lens.Options[c.want])
		}
	}

	dir := os.Getenv("XC_LENSSHOT")
	if dir == "" {
		return
	}
	aim := u.buildAimButtons()
	for _, z := range []float64{1, 2.8} {
		u.showLens(z)
		label := widget.NewLabel("1.0×")
		u.aimBtn.Show()
		if z > 1 {
			label.SetText("2.8×")
			u.backBtn.SetText("2:45") // away from the watch view: the way back counts down
			u.backBtn.Show()
		}
		bar := pill(container.NewHBox(
			overlayButton(theme.ZoomOutIcon(), "", nil), label,
			overlayButton(theme.ZoomInIcon(), "", nil),
			overlayButton(theme.ZoomFitIcon(), "", nil),
			container.NewCenter(lens), aim))
		w := test.NewWindow(container.NewCenter(bar))
		w.Resize(fyne.NewSize(bar.MinSize().Width+40, bar.MinSize().Height+30))
		f, err := os.Create(filepath.Join(dir, map[bool]string{false: "lens-wide.png", true: "lens-tele.png"}[z > 1]))
		if err != nil {
			t.Fatal(err)
		}
		_ = png.Encode(f, w.Canvas().Capture())
		_ = f.Close()
		t.Logf("zoom bar %.1f×: min size %v", z, bar.MinSize())
		w.Close()
	}
}
