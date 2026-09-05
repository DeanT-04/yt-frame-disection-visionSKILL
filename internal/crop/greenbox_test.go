package crop

import (
	"image"
	"image/color"
	"testing"
)

// makeGreenBorderedRef builds a synthetic reference: dark content, a bright
// green 3px outline rectangle from (360,120) to (1890,790), mimicking the
// measured layout of testdata/fixtures/ref_image.png.
func makeGreenBorderedRef(w, h, x0, y0, x1, y1 int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	bg := color.RGBA{R: 245, G: 245, B: 245, A: 255}
	green := color.RGBA{R: 0, G: 255, B: 0, A: 255}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := bg
			// 3px border: within the rect's outer edge
			if x >= x0 && x <= x1 && y >= y0 && y <= y1 &&
				(x <= x0+2 || x >= x1-2 || y <= y0+2 || y >= y1-2) {
				c = green
			}
			img.Set(x, y, c)
		}
	}
	// scattered green-ish syntax pixels inside (should NOT become a band)
	for i := 0; i < 200; i++ {
		x := x0 + 30 + (i*97)%(x1-x0-60)
		y := y0 + 30 + (i*61)%(y1-y0-60)
		img.Set(x, y, color.RGBA{R: 40, G: 190, B: 60, A: 255})
	}
	return img
}

func TestFindGreenBoxSynthetic(t *testing.T) {
	w, h := 1918, 1078
	x0, y0, x1, y1 := 360, 120, 1890, 790
	img := makeGreenBorderedRef(w, h, x0, y0, x1, y1)
	got, err := findGreenBox(img)
	if err != nil {
		t.Fatalf("findGreenBox: %v", err)
	}
	// Interior is the 3px border inset by ~2px: expect ~x0+5 .. x1-5 etc.
	wantMinX, wantMinY := x0+5, y0+5
	wantMaxX, wantMaxY := x1-5, y1-5
	if abs(got.MinX-wantMinX) > 4 || abs(got.MinY-wantMinY) > 4 ||
		abs(got.MaxX-wantMaxX) > 4 || abs(got.MaxY-wantMaxY) > 4 {
		t.Fatalf("corners off: got %s, want around TL=(%d,%d) BR=(%d,%d)",
			got.String(), wantMinX, wantMinY, wantMaxX, wantMaxY)
	}
}

func TestFindGreenBoxNoGreen(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			img.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	if _, err := findGreenBox(img); err == nil {
		t.Fatal("expected error when no green outline exists")
	}
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
