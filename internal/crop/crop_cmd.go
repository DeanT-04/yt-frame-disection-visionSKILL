package crop

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DeanT-04/yt-code-vision-skill/internal/paths"
)

// FindCrop analyzes a representative frame of a video and writes crop.json
// + an annotated preview PNG (into previews/). It needs a frame that shows the
// IDE with code on screen; it auto-picks the best code frame from
// outputs/<id>/frames/.
func FindCrop(id string) error {
	frame, err := pickRepresentativeFrame(id)
	if err != nil {
		return err
	}
	img, err := decodeImage(frame)
	if err != nil {
		return fmt.Errorf("decode %s: %w", frame, err)
	}
	rect, err := detectCodeRect(img)
	if err != nil {
		return err
	}
	root := paths.IDDir(id)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	crop := CropRect{
		X: rect.Min.X, Y: rect.Min.Y,
		Width: rect.Dx(), Height: rect.Dy(),
		Source: "auto",
	}
	cropPath, err := writeCropJSON(root, crop)
	if err != nil {
		return err
	}

	// Annotated preview: original frame + green outline of the detected pane.
	prevDir := paths.PreviewsDir(id)
	if err := os.MkdirAll(prevDir, 0o755); err != nil {
		return err
	}
	preview := filepath.Join(prevDir, "crop_preview.jpg")
	marked := copyImageToDrawable(img)
	drawRectOutline(marked, rect, greenOutline)
	if err := encodeJPEG(preview, marked, 90); err != nil {
		return err
	}

	fmt.Printf("representative frame: %s\n", frame)
	fmt.Printf("detected code pane:   x=%d y=%d w=%d h=%d (%s)\n", crop.X, crop.Y, crop.Width, crop.Height, crop.Source)
	fmt.Printf("crop.json:            %s\n", cropPath)
	fmt.Printf("preview (green box):  %s\n", preview)
	fmt.Println("eyeball the preview; if the green box misses the code, edit crop.json (x/y/width/height) and re-run --crop-frames")
	return nil
}

// CropFrames applies the saved crop.json to every frame of a video, writing
// the cropped code panes into outputs/<id>/crop/. Uses an existing crop.json
// (auto-detected or manually edited) — refuses to run without one.
func CropFrames(id string) error {
	srcDir := paths.IDDir(id)
	c, err := readCropJSON(srcDir)
	if err != nil {
		return fmt.Errorf("need crop.json first (run --find-crop): %w", err)
	}
	outDir := paths.CropsDir(id)
	n, total, err := CropDir(paths.FramesDir(id), outDir, *c)
	if err != nil {
		return err
	}
	fmt.Printf("cropped %d/%d frames -> %s (crop x=%d y=%d w=%d h=%d)\n", n, total, outDir, c.X, c.Y, c.Width, c.Height)
	return nil
}

// CropDir crops every JPEG in srcDir into dstDir using rect (already in src
// pixel coordinates), writing same-named files. Returns frames written and
// total found.
func CropDir(srcDir, dstDir string, rect CropRect) (int, int, error) {
	frames, err := filepath.Glob(filepath.Join(srcDir, "frame_*.jpg"))
	if err != nil {
		return 0, 0, err
	}
	if len(frames) == 0 {
		return 0, 0, fmt.Errorf("no frames in %s", srcDir)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return 0, 0, err
	}
	r := rect.imageRect()
	n := 0
	for _, f := range frames {
		img, err := decodeImage(f)
		if err != nil {
			return 0, 0, fmt.Errorf("decode %s: %w", f, err)
		}
		// Crop against image bounds defensively (screenshots/videos may vary).
		b := img.Bounds().Intersect(r)
		if b.Empty() {
			continue
		}
		sub := imageSubImage(img, b)
		out := filepath.Join(dstDir, filepath.Base(f))
		if err := encodeJPEG(out, sub, 90); err != nil {
			return 0, 0, err
		}
		n++
	}
	return n, len(frames), nil
}

// pickRepresentativeFrame finds a frame that plausibly shows the IDE with code:
// it coarsely scores frames (every 20th) by light-background + text-ink
// coverage and returns the best one. Falls back to the first frame if only a
// few exist.
func pickRepresentativeFrame(id string) (string, error) {
	frames, err := filepath.Glob(filepath.Join(paths.FramesDir(id), "frame_*.jpg"))
	if err != nil {
		return "", err
	}
	if len(frames) == 0 {
		return "", fmt.Errorf("no frames found for video %s (run the extractor first)", id)
	}
	if len(frames) < 20 {
		return frames[0], nil
	}

	best := ""
	bestScore := 0.0
	for i := 0; i < len(frames); i += 20 {
		score, err := codeFrameScore(frames[i])
		if err != nil {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = frames[i]
		}
	}
	if best == "" {
		best = frames[0]
	}
	return best, nil
}

// codeFrameScore rates how much of a frame is light background containing text
// ink (0..1). Used to pick a representative code frame for crop detection.
func codeFrameScore(path string) (float64, error) {
	img, err := decodeImage(path)
	if err != nil {
		return 0, err
	}
	b := img.Bounds()
	const block = 32
	var total, ink int
	for by := 0; by < b.Dy()/block; by += 2 {
		for bx := 0; bx < b.Dx()/block; bx += 2 {
			var nPix, light, dark int
			for y := by * block; y < (by+1)*block && y < b.Dy(); y += 2 {
				for x := bx * block; x < (bx+1)*block && x < b.Dx(); x += 2 {
					r, g, bl, _ := img.At(x, y).RGBA()
					lum := 0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)
					nPix++
					if lum >= 200 {
						light++
					} else if lum <= 80 {
						dark++
					}
				}
			}
			fl := float64(light) / float64(nPix)
			fd := float64(dark) / float64(nPix)
			total++
			if fl >= 0.55 && fd >= 0.004 {
				ink++
			}
		}
	}
	if total == 0 {
		return 0, nil
	}
	return float64(ink) / float64(total), nil
}
