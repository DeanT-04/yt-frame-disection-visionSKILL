// Package extract turns an ordered sequence of screen-recording frames into a
// compact, ordered list of distinct "code states" — one per moment the code
// pane actually changed (typing, edit, scroll) — so that each state can later
// be read verbatim by a vision model without re-reading near-identical frames.
package extract

import (
	"fmt"
	"image"
	_ "image/jpeg"
	"os"
	"path/filepath"
	"sort"
)

// State is one distinct frame worth reading: the source frame that changed and
// its position in the sequence.
type State struct {
	Index    int     // 0-based state number, in temporal order
	FrameNum int     // 1-based frame number within the frameset (frame_NNNNNN.jpg)
	Sec      float64 // approximate seconds since the first frame (1fps)
}

// Options tunes the change detector. Zero values select defaults.
type Options struct {
	// Crop, when non-nil, restricts comparison to the code-pane rectangle (in
	// full-frame pixel coordinates). When nil the whole frame is compared.
	Crop *Rect
	// Threshold is the minimum fraction of changed pixels (after noise-robust
	// downscaling) that starts a new state. Default 0.004 (0.4%).
	Threshold float64
}

// Rect is a plain integer rectangle (kept local so extract stays decoupled
// from the crop package).
type Rect struct{ X, Y, W, H int }

// DefaultThreshold separates real content changes from codec noise: a full
// edited line at pane resolution changes well over 1% of the downscaled pixels,
// while MPEG/JPEG noise on a static scene stays far below this.
const DefaultThreshold = 0.004

// DetectStates scans the JPEGs named frame_000001.jpg, frame_000002.jpg, ... in
// dir (lexical order = temporal order) and returns one State per change point,
// plus the first frame. Frames that only differ by codec noise or a blinking
// caret do not start new states.
func DetectStates(dir string, opts Options) ([]State, error) {
	thresh := opts.Threshold
	if thresh == 0 {
		thresh = DefaultThreshold
	}
	frames, err := sortedFrames(dir)
	if err != nil {
		return nil, err
	}
	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames in %s", dir)
	}

	var states []State
	var prev []byte
	const compareW = 160 // low-res width; height scales to match the region
	for i, f := range frames {
		cur, err := downscaleGray(f, opts.Crop, compareW)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		if i == 0 {
			prev = cur
			states = append(states, State{Index: 0, FrameNum: 1, Sec: 0})
			continue
		}
		if changedFrac(prev, cur) >= thresh {
			states = append(states, State{
				Index:    len(states),
				FrameNum: i + 1,
				Sec:      float64(i), // 1fps: frame k+1 is k seconds after the first frame
			})
		}
		prev = cur
	}
	return states, nil
}

// sortedFrames returns frame_*.jpg paths in lexical (== temporal) order.
func sortedFrames(dir string) ([]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "frame_*.jpg"))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	return matches, nil
}

// downscaleGray decodes path (any stdlib-supported format) and returns a
// luminance []byte scaled so the compared region is compareW pixels wide.
func downscaleGray(path string, crop *Rect, compareW int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, err
	}

	rx, ry, rw, rh := img.Bounds().Min.X, img.Bounds().Min.Y, img.Bounds().Dx(), img.Bounds().Dy()
	if crop != nil {
		rx, ry, rw, rh = crop.X, crop.Y, crop.W, crop.H
		// Clamp to the image bounds (defensive: crops may exceed frame edges).
		b := img.Bounds()
		if rx < b.Min.X {
			rw += rx - b.Min.X
			rx = b.Min.X
		}
		if ry < b.Min.Y {
			rh += ry - b.Min.Y
			ry = b.Min.Y
		}
		if rx+rw > b.Max.X {
			rw = b.Max.X - rx
		}
		if ry+rh > b.Max.Y {
			rh = b.Max.Y - ry
		}
	}
	if rw <= 0 || rh <= 0 {
		return nil, fmt.Errorf("empty compare region for %s", path)
	}

	outW := compareW
	outH := outW * rh / rw
	if outH < 1 {
		outH = 1
	}
	out := make([]byte, outW*outH)
	for oy := 0; oy < outH; oy++ {
		sy := ry + oy*rh/outH
		for ox := 0; ox < outW; ox++ {
			sx := rx + ox*rw/outW
			r, g, bl, _ := img.At(sx, sy).RGBA()
			out[oy*outW+ox] = byte(0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8))
		}
	}
	return out, nil
}

// changedFrac returns the fraction of pixels differing by more than tolerance.
func changedFrac(a, b []byte) float64 {
	if len(a) != len(b) {
		return 1
	}
	const tol = 12 // out of 255
	changed := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		if d > tol {
			changed++
		}
	}
	return float64(changed) / float64(len(a))
}
