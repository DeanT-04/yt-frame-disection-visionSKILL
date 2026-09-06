package extract

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// writeFrames writes n synthetic 400x300 JPEG "code pane" frames into dir. Each
// frame draws lines count dark text bands (a new band per line). lineJumps
// lists, per frame, how many lines to draw.
func writeFrames(t *testing.T, dir string, lineCounts []int) {
	t.Helper()
	for i, n := range lineCounts {
		img := image.NewRGBA(image.Rect(0, 0, 400, 300))
		white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
		dark := color.RGBA{R: 40, G: 40, B: 40, A: 255}
		for y := 0; y < 300; y++ {
			for x := 0; x < 400; x++ {
				img.Set(x, y, white)
			}
		}
		for row := 0; row < n; row++ {
			y := 40 + row*22
			for yy := y; yy < y+4 && yy < 300; yy++ {
				for x := 30; x < 370; x++ {
					img.Set(x, yy, dark)
				}
			}
		}
		writeJPG(t, filepath.Join(dir, fmt.Sprintf("frame_%06d.jpg", i+1)), img)
	}
}

func writeJPG(t *testing.T, path string, img image.Image) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
}

func TestNoChangeIsOneState(t *testing.T) {
	dir := t.TempDir()
	writeFrames(t, dir, []int{5, 5, 5, 5, 5})
	states, err := DetectStates(dir, Options{})
	if err != nil {
		t.Fatalf("DetectStates: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("identical frames produced %d states, want 1", len(states))
	}
	if states[0].FrameNum != 1 {
		t.Fatalf("first state frame = %d, want 1", states[0].FrameNum)
	}
}

func TestEditsProduceOrderedStates(t *testing.T) {
	dir := t.TempDir()
	// Runs of identical frames separated by line additions: expect one state
	// at the start of each run (frames 1, 4, 6).
	writeFrames(t, dir, []int{2, 2, 2, 4, 4, 6, 6, 6})
	states, err := DetectStates(dir, Options{})
	if err != nil {
		t.Fatalf("DetectStates: %v", err)
	}
	want := []int{1, 4, 6}
	if len(states) != len(want) {
		t.Fatalf("got %d states %+v, want %d at frames %v", len(states), states, len(want), want)
	}
	for i, fn := range want {
		if states[i].FrameNum != fn {
			t.Errorf("state %d frame = %d, want %d", i, states[i].FrameNum, fn)
		}
		if states[i].Index != i {
			t.Errorf("state %d index = %d, want %d", i, states[i].Index, i)
		}
	}
}

func TestCaretSizedChangeDoesNotCreateState(t *testing.T) {
	dir := t.TempDir()
	// Two frames identical except a tiny 4x4 block (a blinking caret): must not
	// exceed the default threshold.
	writeFrames(t, dir, []int{5})
	// append one frame with a caret-sized blob
	img := toRGBA(t, loadFrame(t, filepath.Join(dir, fmt.Sprintf("frame_%06d.jpg", 1))))
	c := color.RGBA{R: 40, G: 40, B: 40, A: 255}
	for yy := 100; yy < 104; yy++ {
		for xx := 200; xx < 204; xx++ {
			img.Set(xx, yy, c)
		}
	}
	writeJPG(t, filepath.Join(dir, fmt.Sprintf("frame_%06d.jpg", 2)), img)

	states, err := DetectStates(dir, Options{})
	if err != nil {
		t.Fatalf("DetectStates: %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("caret-sized change created %d states, want 1", len(states))
	}
}

func TestCropGatesChangesOutsidePane(t *testing.T) {
	dir := t.TempDir()
	writeFrames(t, dir, []int{5})
	// Frame 2 changes only OUTSIDE the pane region (a UI element at the top).
	img := toRGBA(t, loadFrame(t, filepath.Join(dir, fmt.Sprintf("frame_%06d.jpg", 1))))
	c := color.RGBA{R: 40, G: 40, B: 40, A: 255}
	for yy := 0; yy < 20; yy++ {
		for xx := 0; xx < 60; xx++ {
			img.Set(xx, yy, c)
		}
	}
	writeJPG(t, filepath.Join(dir, fmt.Sprintf("frame_%06d.jpg", 2)), img)

	// With the crop below that region: no new state.
	pane := &Rect{X: 0, Y: 30, W: 400, H: 260}
	states, err := DetectStates(dir, Options{Crop: pane})
	if err != nil {
		t.Fatalf("DetectStates(crop): %v", err)
	}
	if len(states) != 1 {
		t.Fatalf("change outside crop created %d states, want 1", len(states))
	}

	// Without the crop the same two frames differ -> a second state.
	states, err = DetectStates(dir, Options{})
	if err != nil {
		t.Fatalf("DetectStates: %v", err)
	}
	if len(states) != 2 {
		t.Fatalf("full-frame diff created %d states, want 2", len(states))
	}
}

func loadFrame(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	return img
}

// toRGBA converts any decoded image to an *image.RGBA so tests can draw on it.
func toRGBA(t *testing.T, src image.Image) *image.RGBA {
	t.Helper()
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst
}
