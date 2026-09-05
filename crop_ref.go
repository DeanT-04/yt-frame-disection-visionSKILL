package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
)

// cropFromRefCmd detects the green-outline box on a reference image and writes
// crop.json for a video, scaled from the reference resolution to the target
// frame resolution. The reference is expected to show the same screen layout
// as the video (e.g. a marked-up screenshot of the actual frame).
func cropFromRefCmd(id, refPath string) error {
	refImg, err := decodeImage(refPath)
	if err != nil {
		return fmt.Errorf("decode ref %s: %w", refPath, err)
	}
	box, err := findGreenBox(refImg)
	if err != nil {
		return fmt.Errorf("green box detection on %s: %w", refPath, err)
	}
	refW, refH := refImg.Bounds().Dx(), refImg.Bounds().Dy()

	// Target frame resolution: read the first frame for this video.
	frame, err := pickRepresentativeFrame(id)
	if err != nil {
		return err
	}
	frameImg, err := decodeImage(frame)
	if err != nil {
		return fmt.Errorf("decode frame %s: %w", frame, err)
	}
	fw, fh := frameImg.Bounds().Dx(), frameImg.Bounds().Dy()

	// Scale reference coordinates onto the frame (handles any ref-vs-frame
	// size mismatch, e.g. 1918x1078 ref -> 1920x1080 frames).
	sx := float64(fw) / float64(refW)
	sy := float64(fh) / float64(refH)
	scaled := BoxCorners{
		MinX: clampScale(box.MinX, sx, fw),
		MinY: clampScale(box.MinY, sy, fh),
		MaxX: clampScale(box.MaxX, sx, fw),
		MaxY: clampScale(box.MaxY, sy, fh),
	}
	if scaled.MaxX <= scaled.MinX || scaled.MaxY <= scaled.MinY {
		return fmt.Errorf("scaled crop has zero/negative area: %s", scaled.String())
	}

	root := idDir(id)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	crop := CropRect{
		X: scaled.MinX, Y: scaled.MinY,
		Width:  scaled.MaxX - scaled.MinX,
		Height: scaled.MaxY - scaled.MinY,
		Source: "ref",
	}
	cropPath, err := writeCropJSON(root, crop)
	if err != nil {
		return err
	}

	// Annotated preview on the reference image so the box is easy to eyeball.
	prevDir := previewsDir(id)
	if err := os.MkdirAll(prevDir, 0o755); err != nil {
		return err
	}
	preview := filepath.Join(prevDir, "crop_ref_preview.jpg")
	marked := copyImageToDrawable(refImg)
	drawRectOutline(marked, refBoxRect(box), greenOutline)
	if err := encodeJPEG(preview, marked, 90); err != nil {
		return err
	}

	fmt.Printf("ref image:      %s (%dx%d)\n", refPath, refW, refH)
	fmt.Printf("green box (ref): TL=(%d,%d) BR=(%d,%d)  %dx%d\n", box.MinX, box.MinY, box.MaxX, box.MaxY, box.Width(), box.Height())
	fmt.Printf("frames:         %s (%dx%d)\n", frame, fw, fh)
	fmt.Printf("crop (scaled):  x=%d y=%d w=%d h=%d  [%s]\n", crop.X, crop.Y, crop.Width, crop.Height, crop.Source)
	fmt.Printf("crop.json:      %s\n", cropPath)
	fmt.Printf("preview:        %s\n", preview)
	fmt.Println("eyeball the preview; then run --crop-frames to apply.")
	return nil
}

// refBoxRect converts BoxCorners (already pixel coords) to an image.Rectangle.
func refBoxRect(b BoxCorners) image.Rectangle {
	return image.Rect(b.MinX, b.MinY, b.MaxX, b.MaxY)
}

// clampScale scales a coordinate by a factor and clamps it into [0, limit].
func clampScale(v int, scale float64, limit int) int {
	s := int(float64(v)*scale + 0.5)
	if s < 0 {
		return 0
	}
	if s > limit {
		return limit
	}
	return s
}
