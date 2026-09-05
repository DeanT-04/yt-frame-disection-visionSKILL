package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// benchHeights is the resolution ladder for the benchmark (lowest -> 1080p).
// Deliberately stops at 1080p: above that adds cost with no code-reading gain.
var benchHeights = []int{144, 240, 360, 480, 720, 1080}

// runBench downloads a 5-minute 1080p master once, then extracts 1fps JPEGs
// at every bench height into bench/<id>/<h>p/. Frames never leave the project.
func runBench(cfg config, id string) error {
	cache := cacheDir("bench-" + id)
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return err
	}

	// Idempotent: if all six res folders already have frames, skip (no
	// network, no re-extraction) unless --fresh.
	if !cfg.fresh && benchComplete(id) {
		fmt.Printf("%s: bench already complete — skipping. Re-run with --fresh to redo.\n", id)
		return nil
	}

	fmt.Printf("\n== BENCH %s ==\n", id)
	start := time.Now()

	// Metadata first (no media download) so we know the duration and can pick
	// a 5-minute window starting at the coding chapter.
	meta, err := fetchMeta(cache, id)
	if err != nil {
		return fmt.Errorf("fetch metadata: %w", err)
	}
	startSec, chTitle, matched := chapterStart(toChapters(meta.Chapters), cfg.startCh)
	section := "*0:00-5:00"
	if matched && startSec > 0 {
		section = formatStartSection(startSec, meta.Duration, 300)
	}

	// 5-minute 1080p master (top of the ladder), starting at the coding
	// chapter so the resolution comparison actually shows code, not intro.
	// The master is cached (outputs/.cache/bench-<id>) and kept for reuse.
	mediaPath, info, err := downloadVideo(cache, id, "1080", section)
	if err != nil {
		return err
	}
	if !matched || startSec == 0 {
		fmt.Printf("master: %s (first 5 min — no coding chapter matched)\n", mediaPath)
	} else {
		fmt.Printf("master: %s (5 min from @%s '%s', duration %.0fs)\n", mediaPath, hms(startSec), chTitle, info.Duration)
	}

	summary := make([]string, 0, len(benchHeights))
	for _, h := range benchHeights {
		outDir := filepath.Join("bench", id, fmt.Sprintf("%dp", h))
		scale := fmt.Sprintf("scale=-2:%d,fps=1", h)
		n, err := extractScaled(mediaPath, outDir, scale, threads, "2", 0)
		if err != nil {
			return fmt.Errorf("%dp: %w", h, err)
		}
		size, err := dirSize(outDir)
		if err != nil {
			return err
		}
		summary = append(summary, fmt.Sprintf("%4dp: %4d frames  %6.1f MB", h, n, mb(size)))
	}
	// Cached master is intentionally kept for reuse (outputs/.cache/bench-<id>).
	fmt.Printf("bench done in %s (master cached):\n%s\n", time.Since(start).Round(time.Second), strings.Join(summary, "\n"))
	return nil
}

// benchComplete reports whether all bench res folders already contain frames.
func benchComplete(id string) bool {
	for _, h := range benchHeights {
		dir := filepath.Join("bench", id, fmt.Sprintf("%dp", h))
		matches, err := filepath.Glob(filepath.Join(dir, "frame_*.jpg"))
		if err != nil || len(matches) == 0 {
			return false
		}
	}
	return true
}

// extractScaled is extractFrames with a custom -vf chain (used for bench
// downscaling: "scale=-2:480,fps=1" etc.). startSec seeks before decoding.
func extractScaled(mediaPath, outDir, vf, threads, quality string, startSec float64) (int, error) {
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
		args = append(args, "-ss", hms(startSec))
	}
	args = append(args,
		"-i", mediaPath,
		"-vf", vf,
		"-q:v", quality,
		"-y",
		pattern,
	)
	if _, err := runCmd("ffmpeg", args...); err != nil {
		return 0, fmt.Errorf("ffmpeg (%s): %w", vf, err)
	}
	matches, err := filepath.Glob(filepath.Join(outDir, "frame_*.jpg"))
	if err != nil {
		return 0, err
	}
	return len(matches), nil
}

// dirSize sums the bytes of all files under dir.
func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.Walk(dir, func(_ string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	return total, err
}

func mb(bytes int64) float64 {
	return float64(bytes) / (1024 * 1024)
}
