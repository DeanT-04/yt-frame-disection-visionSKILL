// Command yt-code-vision-skill extracts frames + chapter data from YouTube
// videos. Input: a txt file of video URLs in the project root. Output: frames
// under outputs/<id>/ and chapter YAML under outputs/<id>/chapters/.
//
// ALL artifacts (outputs, bench, temp downloads, previews) stay inside the
// project folder — nothing is ever written outside of it.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"
)

const (
	defaultHeight = "1080"
	defaultInput  = "YT-URL-TEST.txt"
	threads       = "6" // 80% of 8 logical CPUs
	fps           = "1" // 1 frame per second
)

type config struct {
	input      string
	height     string
	bench      bool
	findCrop   bool
	cropFrames bool
	cropRef    string // green-box reference image path (--crop-from-ref)
	startCh    string // chapter title/keyword override for frame start (--start-chapter)
	fresh      bool   // redo an already-processed video (old data -> trash)
	purgeCache bool   // remove outputs/.cache and outputs/.trash
}

func parseFlags(args []string) (config, error) {
	fs := flag.NewFlagSet("yt-code-vision-skill", flag.ContinueOnError)
	cfg := config{input: defaultInput, height: defaultHeight}
	fs.StringVar(&cfg.input, "input", defaultInput, "txt file with YouTube URLs")
	fs.StringVar(&cfg.height, "height", defaultHeight, "max download height in px (default 1080)")
	fs.BoolVar(&cfg.bench, "bench", false, "extract 5-min bench frames at 144/240/360/480/720/1080p")
	fs.BoolVar(&cfg.findCrop, "find-crop", false, "detect the IDE code pane on a representative frame -> crop.json + preview")
	fs.StringVar(&cfg.cropRef, "crop-from-ref", "", "detect green outline box on a reference PNG -> crop.json (scaled to frames)")
	fs.BoolVar(&cfg.cropFrames, "crop-frames", false, "apply crop.json to all frames -> outputs/<id>/crop/")
	fs.StringVar(&cfg.startCh, "start-chapter", "", "start frame extraction at the chapter whose title contains this (default: auto-detect coding chapter)")
	fs.BoolVar(&cfg.fresh, "fresh", false, "redo processing for videos already in outputs (old data moved to outputs/.trash)")
	fs.BoolVar(&cfg.purgeCache, "purge-cache", false, "delete outputs/.cache and outputs/.trash")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func main() {
	log.SetFlags(0)
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		log.Fatalf("usage error: %v", err)
	}

	urls, err := loadURLs(cfg.input)
	if err != nil {
		log.Fatalf("input: %v", err)
	}

	if cfg.purgeCache {
		if err := purgeCache(); err != nil {
			log.Fatalf("purge: %v", err)
		}
		return
	}

	fmt.Printf("processing %d URL(s) from %s\n", len(urls), cfg.input)

	for _, u := range urls {
		id, err := videoID(u)
		if err != nil {
			log.Printf("SKIP %q: %v", u, err)
			continue
		}
		var runErr error
		switch {
		case cfg.cropRef != "":
			runErr = cropFromRefCmd(id, cfg.cropRef)
		case cfg.findCrop:
			runErr = findCropCmd(id)
		case cfg.cropFrames:
			runErr = cropFramesCmd(id)
		case cfg.bench:
			runErr = runBench(cfg, id)
		default:
			runErr = processVideo(cfg, id)
		}
		if runErr != nil {
			log.Printf("FAIL %s: %v", id, runErr)
		}
	}
}

// processVideo processes one video: it reuses a cached master if present
// (else downloads it into outputs/.cache/<id>, which is KEPT for reuse),
// extracts 1 frame/sec JPEGs into outputs/<id>/frames/, writes chapter YAML,
// then marks the run done. Re-running an already-done id is a no-op unless
// --fresh is passed.
func processVideo(cfg config, id string) error {
	idRoot := idDir(id)
	cache := cacheDir(id)
	if cfg.fresh {
		if err := moveToTrash(idRoot); err != nil {
			return err
		}
	}
	// Idempotent: already fully processed -> skip everything (no network).
	if _, err := os.Stat(doneMarkerPath(id)); err == nil && !cfg.fresh {
		fmt.Printf("%s: already processed (.done present) — skipping. Re-run with --fresh to redo.\n", id)
		return nil
	}

	for _, d := range []string{cache, idRoot, framesDir(id), chaptersDir(id)} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	fmt.Printf("\n== %s ==\n", id)
	start := time.Now()
	mediaPath, info, err := downloadVideo(cache, id, cfg.height, "")
	if err != nil {
		return err
	}
	fmt.Printf("media -> %s (%.0f s total duration, cached for reuse)\n", mediaPath, info.Duration)
	if info.Title != "" {
		fmt.Printf("title:  %s\n", info.Title)
	}

	// Start framing at the coding chapter (or a --start-chapter override).
	// Everything before it is usually intro/strategy/talk — no code, no value.
	startSec, chTitle, matched := chapterStart(toChapters(info.Chapters), cfg.startCh)
	if matched && startSec > 0 {
		fmt.Printf("frame start: @%s (chapter %q — skipping %.0f s of lead-in)\n", hms(startSec), chTitle, startSec)
	} else {
		fmt.Printf("frame start: 0 (no coding chapter matched%s)\n",
			map[bool]string{true: " for override " + cfg.startCh, false: ""}[cfg.startCh != ""])
	}

	// 1 frame per second, JPEG quality 2, 6 threads (80% of 8 cores).
	framesOut := framesDir(id)
	n, err := extractFrames(mediaPath, framesOut, threads, "2", startSec)
	if err != nil {
		return err
	}
	fmt.Printf("frames:   %d extracted into %s\n", n, framesOut)

	chPath, err := writeChaptersYAML(idRoot, info)
	if err != nil {
		return err
	}
	if len(info.Chapters) > 0 {
		fmt.Printf("chapters: %d -> %s\n", len(info.Chapters), chPath)
	} else {
		fmt.Printf("chapters: none (chapters: NaN written)\n")
	}

	// Frames + chapters are done: mark idempotent completion. The cached
	// master is intentionally kept (outputs/.cache/<id>) for future re-runs.
	if err := os.WriteFile(doneMarkerPath(id), []byte(time.Now().Format(time.RFC3339)+"\n"), 0o644); err != nil {
		return err
	}
	fmt.Printf("done:     %s (cached master kept at %s)\n", time.Since(start).Round(time.Second), mediaPath)
	return nil
}
