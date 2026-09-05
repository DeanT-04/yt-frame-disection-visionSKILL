package crop

import (
	"image"
	"os"
	"path/filepath"
	"testing"
)

// goldenFixtures maps a committed real screenshot (in testdata/fixtures/) to
// its expected code-pane rectangle, in the image's own pixel coordinates, plus
// a per-edge tolerance. These are filled in as the real dark-theme / terminal /
// non-standard-editor screenshots are added — see testdata/fixtures/README.md.
var goldenFixtures = []struct {
	file      string
	want      CropRect
	tolerance int
}{
	// {"dark_theme.png", CropRect{X: 0, Y: 0, Width: 0, Height: 0}, 24},
	// {"terminal.png", CropRect{X: 0, Y: 0, Width: 0, Height: 0}, 24},
	// {"nonstandard_editor.png", CropRect{X: 0, Y: 0, Width: 0, Height: 0}, 24},
}

// TestGoldenFixtures validates detectCodeRect against the real reference frames.
// It skips cleanly until fixtures + their expected rects are populated, so a
// fresh clone without the images still builds and passes.
func TestGoldenFixtures(t *testing.T) {
	if len(goldenFixtures) == 0 {
		t.Skip("no golden crop fixtures configured yet — add entries to goldenFixtures and drop images in testdata/fixtures/")
	}
	for _, f := range goldenFixtures {
		t.Run(f.file, func(t *testing.T) {
			path := filepath.Join("testdata", "fixtures", f.file)
			if _, err := os.Stat(path); err != nil {
				t.Skipf("fixture %s not present — see testdata/fixtures/README.md", f.file)
			}
			img, err := decodeImage(path)
			if err != nil {
				t.Fatalf("decode %s: %v", path, err)
			}
			got, err := detectCodeRect(img)
			if err != nil {
				t.Fatalf("detectCodeRect(%s): %v", path, err)
			}
			assertClose(t, got, f.want.imageRect(), f.tolerance)
		})
	}
}

// assertClose checks each edge of the detected rect against the golden value
// within the given pixel tolerance.
func assertClose(t *testing.T, got, want image.Rectangle, tol int) {
	t.Helper()
	if abs(got.Min.X-want.Min.X) > tol || abs(got.Min.Y-want.Min.Y) > tol ||
		abs(got.Max.X-want.Max.X) > tol || abs(got.Max.Y-want.Max.Y) > tol {
		t.Fatalf("detected %+v, want %+v (tolerance %dpx)", got, want, tol)
	}
}
