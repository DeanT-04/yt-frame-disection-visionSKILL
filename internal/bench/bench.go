// Package bench runs the resolution benchmark: download a 5-minute 1080p
// master once, then extract 1fps JPEGs at every rung of the resolution ladder
// so the code-readability of each resolution can be compared.
package bench

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeanT-04/yt-code-vision-skill/internal/cache"
	"github.com/DeanT-04/yt-code-vision-skill/internal/chapter"
	"github.com/DeanT-04/yt-code-vision-skill/internal/frames"
	"github.com/DeanT-04/yt-code-vision-skill/internal/media"
	"github.com/DeanT-04/yt-code-vision-skill/internal/paths"
)

// benchHeights is the resolution ladder for the benchmark (lowest -> 1080p).
// Deliberately stops at 1080p: above that adds cost with no code-reading gain.
var benchHeights = []int{144, 240, 360, 480, 720, 1080}

// RunBench downloads a 5-minute 1080p master once, then extracts 1fps JPEGs
// at every bench height into bench/<id>/<h>p/. Frames never leave the project.
func RunBench(startCh string, fresh bool, id string) error {
	cache := cache.CacheDir("bench-" + id)
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return err
	}

	// Idempotent: if all six res folders already have frames, skip (no
	// network, no re-extraction) unless --fresh.
	if !fresh && benchComplete(id) {
		fmt.Printf("%s: bench already complete — skipping. Re-run with --fresh to redo.\n", id)
		return nil
	}

	fmt.Printf("\n== BENCH %s ==\n", id)
	start := time.Now()

	// Metadata first (no media download) so we know the duration and can pick
	// a 5-minute window starting at the coding chapter.
	meta, err := media.FetchMeta(cache, id)
	if err != nil {
		return fmt.Errorf("fetch metadata: %w", err)
	}
	startSec, chTitle, matched := chapter.ChapterStart(meta.Chapters, startCh)
	section := "*0:00-5:00"
	if matched && startSec > 0 {
		section = chapter.FormatStartSection(startSec, meta.Duration, 300)
	}

	// 5-minute 1080p master (top of the ladder), starting at the coding
	// chapter so the resolution comparison actually shows code, not intro.
	// The master is cached (outputs/.cache/bench-<id>) and kept for reuse.
	mediaPath, info, err := media.DownloadVideo(cache, id, "1080", section)
	if err != nil {
		return err
	}
	if !matched || startSec == 0 {
		fmt.Printf("master: %s (first 5 min — no coding chapter matched)\n", mediaPath)
	} else {
		fmt.Printf("master: %s (5 min from @%s '%s', duration %.0fs)\n", mediaPath, chapter.Hms(startSec), chTitle, info.Duration)
	}

	summary := make([]string, 0, len(benchHeights))
	for _, h := range benchHeights {
		outDir := paths.BenchResDir(id, h)
		scale := fmt.Sprintf("scale=-2:%d,fps=1", h)
		n, err := frames.ExtractScaled(mediaPath, outDir, scale, frames.DefaultThreads, frames.DefaultQuality, 0)
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
		dir := paths.BenchResDir(id, h)
		matches, err := filepath.Glob(filepath.Join(dir, "frame_*.jpg"))
		if err != nil || len(matches) == 0 {
			return false
		}
	}
	return true
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
