package main

import (
	"image"
	"image/color"
	"testing"
)

// makeCodePaneFrame builds a synthetic 1920x1080 frame: dark chrome around a
// white code pane with dark text lines — the shape detectCodeRect should find.
func makeCodePaneFrame() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	dark := color.RGBA{R: 30, G: 30, B: 30, A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	text := color.RGBA{R: 20, G: 20, B: 20, A: 255}
	// fill everything dark, then a white pane from (200,150) to (1720,930)
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			img.Set(x, y, dark)
		}
	}
	for y := 150; y < 930; y++ {
		for x := 200; x < 1720; x++ {
			img.Set(x, y, white)
		}
	}
	// draw dark text lines (pseudo code rows) inside the pane
	for row := 0; row < 40; row++ {
		y := 200 + row*17
		for x := 220; x < 1600; x++ {
			// text occupies ~60% of each line with gaps
			if (x/9)%5 != 4 {
				img.Set(x, y, text)
			}
		}
	}
	return img
}

func TestDetectCodeRectFindsPane(t *testing.T) {
	img := makeCodePaneFrame()
	r, err := detectCodeRect(img)
	if err != nil {
		t.Fatalf("detectCodeRect: %v", err)
	}
	// Pane spans (200,150)-(1720,930). Detection works on a 16px block grid and
	// trims half a block of margin, so expect roughly the pane, not the chrome.
	if r.Min.X < 100 || r.Min.Y < 50 {
		t.Errorf("crop starts too far top-left: %+v (pane starts 200,150)", r)
	}
	if r.Max.X > 1820 || r.Max.Y > 1030 {
		t.Errorf("crop extends beyond pane right/bottom edge: %+v", r)
	}
	if r.Dx() < 1200 || r.Dy() < 600 {
		t.Errorf("crop too small to be the code pane: %+v (pane is 1520x780)", r)
	}
}

func TestDetectCodeRectRejectsNoText(t *testing.T) {
	// Same layout but a plain white panel with no ink: must be rejected.
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	dark := color.RGBA{R: 30, G: 30, B: 30, A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	for y := 0; y < 1080; y++ {
		for x := 0; x < 1920; x++ {
			c := dark
			if x >= 200 && x < 1720 && y >= 150 && y < 930 {
				c = white
			}
			img.Set(x, y, c)
		}
	}
	if _, err := detectCodeRect(img); err == nil {
		t.Fatal("expected error for a light panel with no text ink")
	}
}

func TestCropJSONRoundTrip(t *testing.T) {
	dir := t.TempDir()
	want := CropRect{X: 120, Y: 488, Width: 1792, Height: 528, Source: "auto"}
	p, err := writeCropJSON(dir, want)
	if err != nil {
		t.Fatalf("writeCropJSON: %v", err)
	}
	got, err := readCropJSON(dir)
	if err != nil {
		t.Fatalf("readCropJSON: %v", err)
	}
	if *got != want {
		t.Fatalf("round trip = %+v, want %+v", *got, want)
	}
	if p == "" {
		t.Fatal("expected a path")
	}
}
