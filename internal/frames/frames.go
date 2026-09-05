// Package frames extracts JPEG frames from media with ffmpeg — one per second
// at full resolution, or a custom filter chain for the benchmark downscaling.
package frames

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/DeanT-04/yt-code-vision-skill/internal/chapter"
	"github.com/DeanT-04/yt-code-vision-skill/internal/media"
)

const (
	// DefaultThreads is 80% of 8 logical CPUs.
	DefaultThreads = "6"
	// DefaultQuality is ffmpeg's -q:v JPEG quality (2 = very high).
	DefaultQuality = "2"
)

// ExtractFrames grabs one JPEG per second from the media into outDir, using
// threads and quality. startSec seeks into the media first (only frames from
// that point onward are decoded — saves time on long videos when we only want
// the coding section). Frame files are named frame_000001.jpg, frame_000002.jpg,
// ... Returns the frame count.
func ExtractFrames(mediaPath, outDir, threads, quality string, startSec float64) (int, error) {
	return ExtractScaled(mediaPath, outDir, "fps=1", threads, quality, startSec)
}

// ExtractScaled is ExtractFrames with a custom -vf chain (used for bench
// downscaling: "scale=-2:480,fps=1" etc.). startSec seeks before decoding.
func ExtractScaled(mediaPath, outDir, vf, threads, quality string, startSec float64) (int, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return 0, err
	}
	pattern := filepath.Join(outDir, "frame_%06d.jpg")
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-threads", threads,
	}
	if startSec > 0 {
		// -ss before -i = input seeking: ffmpeg jumps near the keyframe at
		// startSec instead of decoding from 0 (big saving on 80-min videos).
		args = append(args, "-ss", chapter.Hms(startSec))
	}
	args = append(args,
		"-i", mediaPath,
		"-vf", vf,
		"-q:v", quality,
		"-y",
		pattern,
	)
	if _, err := media.RunCmd("ffmpeg", args...); err != nil {
		return 0, fmt.Errorf("ffmpeg (%s): %w", vf, err)
	}
	matches, err := filepath.Glob(filepath.Join(outDir, "frame_*.jpg"))
	if err != nil {
		return 0, err
	}
	return len(matches), nil
}
