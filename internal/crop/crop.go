package crop

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
)

// CropRect is the detected/overridden code-pane rectangle in original-frame
// pixel coordinates. Stored as crop.json next to a video's frames.
type CropRect struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Source string `json:"source"` // "auto" or "manual"
}

func (c CropRect) imageRect() image.Rectangle {
	return image.Rect(c.X, c.Y, c.X+c.Width, c.Y+c.Height)
}

// decodeImage loads a PNG or JPEG by extension using the stdlib codecs.
func decodeImage(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png":
		return png.Decode(f)
	case ".jpg", ".jpeg":
		return jpeg.Decode(f)
	default:
		return nil, fmt.Errorf("unsupported image type %q (want png/jpg)", ext)
	}
}

// encodeJPEG writes an image as a JPEG (used for the annotated preview).
func encodeJPEG(path string, img image.Image, quality int) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	return jpeg.Encode(f, img, &jpeg.Options{Quality: quality})
}

// panelPolarity is which luminance class counts as the code-pane "background"
// vs the "ink" (text). A screen recording can use a light editor theme
// (light bg, dark text) or a dark theme / terminal (dark bg, light text).
type panelPolarity int

const (
	lightPanel panelPolarity = iota // light bg, dark text (classic IDE theme)
	darkPanel                       // dark bg, light text (dark theme / terminal)
)

// detectCodeRect finds the code pane in a screen-recording frame. It tries both
// polarities (light and dark backgrounds) and returns the larger detected pane,
// so it works for light-theme IDEs, dark-theme IDEs, and terminal-heavy frames
// alike. Works on a coarse block grid (16px cells) with a maximal-rectangle
// search, then maps back to full pixel coordinates. Pure stdlib; no external
// image deps.
func detectCodeRect(img image.Image) (image.Rectangle, error) {
	b := img.Bounds()
	const block = 16
	cols := b.Dx() / block
	rows := b.Dy() / block
	if cols < 8 || rows < 4 {
		return image.Rectangle{}, fmt.Errorf("image too small for crop detection (%dx%d)", b.Dx(), b.Dy())
	}

	light, lightErr := detectPanelRect(img, block, rows, cols, lightPanel)
	dark, darkErr := detectPanelRect(img, block, rows, cols, darkPanel)
	switch {
	case lightErr == nil && darkErr == nil:
		if dark.Dx()*dark.Dy() > light.Dx()*light.Dy() {
			return dark, nil
		}
		return light, nil
	case lightErr == nil:
		return light, nil
	case darkErr == nil:
		return dark, nil
	default:
		return image.Rectangle{}, fmt.Errorf("no code pane found (light: %v; dark: %v)", lightErr, darkErr)
	}
}

// detectPanelRect finds the largest background panel of the given polarity that
// contains interior "ink" (the opposite polarity) — i.e. a code pane. It builds
// per-block background/ink coverage on a coarse grid, then takes the maximal
// rectangle and requires interior ink so a plain card or empty sidebar is
// rejected.
func detectPanelRect(img image.Image, block, rows, cols int, pol panelPolarity) (image.Rectangle, error) {
	b := img.Bounds()
	panel := make([][]bool, rows)
	ink := make([][]bool, rows)
	for by := 0; by < rows; by++ {
		panel[by] = make([]bool, cols)
		ink[by] = make([]bool, cols)
		for bx := 0; bx < cols; bx++ {
			var n, bg, fg int
			for y := by * block; y < (by+1)*block && y < b.Dy(); y++ {
				for x := bx * block; x < (bx+1)*block && x < b.Dx(); x++ {
					r, g, bl, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
					lum := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
					n++
					switch pol {
					case lightPanel:
						if lum >= 200 {
							bg++
						} else if lum <= 120 {
							fg++
						}
					case darkPanel:
						if lum <= 60 {
							bg++
						} else if lum >= 160 {
							fg++
						}
					}
				}
			}
			fracBG := float64(bg) / float64(n)
			fracFG := float64(fg) / float64(n)
			// Panel-ish: mostly background. There is deliberately NO upper bound
			// on background fraction — blank lines and margins inside a code pane
			// are pure background and must stay part of the connected region. The
			// ink check below is what stops us from selecting an empty area.
			panel[by][bx] = fracBG >= 0.45
			ink[by][bx] = fracFG >= 0.003 && fracFG <= 0.6
		}
	}

	best, bestArea := largestPanelRect(panel)
	minArea := (rows * cols) / 16 // generous lower bound to reject noise
	if bestArea < minArea {
		return image.Rectangle{}, fmt.Errorf("no large panel found (best %d blocks, need >=%d)", bestArea, minArea)
	}

	// Require the panel to contain ink in its INTERIOR (excluding the outer
	// ring of blocks — those sit on the pane/chrome boundary and always look
	// "inky"). Without this, a plain card is accepted.
	inkBlocks := 0
	for by := best.top + 1; by <= best.bottom-1; by++ {
		for bx := best.left + 1; bx <= best.right-1; bx++ {
			if ink[by][bx] {
				inkBlocks++
			}
		}
	}
	if inkBlocks < 3 {
		return image.Rectangle{}, fmt.Errorf("largest panel has no interior ink (%d ink blocks); refusing crop", inkBlocks)
	}

	px := func(v int) int { return v * block }
	// Keep a small margin (half a block) inside the found region.
	m := block / 2
	x0 := px(best.left) + m
	y0 := px(best.top) + m
	x1 := px(best.right+1) - m
	y1 := px(best.bottom+1) - m
	if x1 <= x0 || y1 <= y0 {
		return image.Rectangle{}, fmt.Errorf("detected crop has zero area")
	}
	return image.Rect(x0, y0, x1, y1), nil
}

// panelRect is a maximal rectangle of "background" blocks, in block-grid
// coordinates.
type panelRect struct{ top, left, bottom, right int }

// largestPanelRect finds the largest all-true rectangle in a boolean grid using
// a per-row histogram + monotonic stack (maximal rectangle in a histogram).
func largestPanelRect(panel [][]bool) (panelRect, int) {
	rows := len(panel)
	cols := 0
	if rows > 0 {
		cols = len(panel[0])
	}
	best := panelRect{}
	bestArea := 0
	height := make([]int, cols)
	for by := 0; by < rows; by++ {
		for bx := 0; bx < cols; bx++ {
			if panel[by][bx] {
				height[bx]++
			} else {
				height[bx] = 0
			}
		}
		// monotonic stack: for each bar find the widest rect ending at this row.
		type bar struct{ idx, h int }
		var st []bar
		for bx := 0; bx <= cols; bx++ {
			h := 0
			if bx < cols {
				h = height[bx]
			}
			start := bx
			for len(st) > 0 && st[len(st)-1].h > h {
				top := st[len(st)-1]
				st = st[:len(st)-1]
				area := top.h * (bx - top.idx)
				if area > bestArea {
					bestArea = area
					best = panelRect{top: by - top.h + 1, left: top.idx, bottom: by, right: bx - 1}
				}
				start = top.idx
			}
			if h > 0 {
				st = append(st, bar{idx: start, h: h})
			}
		}
	}
	return best, bestArea
}

// greenOutline is the color used to mark detected/derived crops on previews.
var greenOutline = color.RGBA{R: 0, G: 255, B: 0, A: 255}

// drawRectOutline draws a colored 3px border around r on img (for previews).
func drawRectOutline(img draw.Image, r image.Rectangle, c color.RGBA) {
	thick := 3
	col := func(x0, y0, x1, y1 int) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.Set(x, y, c)
			}
		}
	}
	col(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+thick) // top
	col(r.Min.X, r.Max.Y-thick, r.Max.X, r.Max.Y) // bottom
	col(r.Min.X, r.Min.Y, r.Min.X+thick, r.Max.Y) // left
	col(r.Max.X-thick, r.Min.Y, r.Max.X, r.Max.Y) // right
}

// writeCropJSON stores the crop rect next to the video frames.
func writeCropJSON(dir string, c CropRect) (string, error) {
	path := filepath.Join(dir, "crop.json")
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// readCropJSON loads a manually-provided or previously-detected crop.json.
func readCropJSON(dir string) (*CropRect, error) {
	b, err := os.ReadFile(filepath.Join(dir, "crop.json"))
	if err != nil {
		return nil, err
	}
	var c CropRect
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.Width <= 0 || c.Height <= 0 {
		return nil, fmt.Errorf("invalid crop.json rect")
	}
	return &c, nil
}

// Load reads the saved crop.json from dir (outputs/<id>/).
func Load(dir string) (*CropRect, error) {
	return readCropJSON(dir)
}

// Scale scales a CropRect by factor, for mapping a full-resolution crop onto
// downscaled frames (e.g. the bench resolution ladder). Rounds to nearest px.
func Scale(c CropRect, factor float64) CropRect {
	return CropRect{
		X:      int(float64(c.X)*factor + 0.5),
		Y:      int(float64(c.Y)*factor + 0.5),
		Width:  int(float64(c.Width)*factor + 0.5),
		Height: int(float64(c.Height)*factor + 0.5),
		Source: c.Source,
	}
}

// copyImageToDrawable makes a draw.Image copy of any decoded image so we can
// overlay the preview rectangle.
func copyImageToDrawable(src image.Image) draw.Image {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)
	return dst
}

// imageSubImage crops src to the given bounds into a new RGBA.
func imageSubImage(src image.Image, r image.Rectangle) draw.Image {
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Bounds(), src, r.Min, draw.Src)
	return dst
}
