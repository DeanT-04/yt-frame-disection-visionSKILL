package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Persistent, project-local media cache + pacing + trash.
//
// Why: repeated full downloads of the same video hammer YouTube and trigger
// per-IP 403 throttling. We now keep downloaded masters in outputs/.cache/<id>/
// so re-runs reuse them with zero network, enforce a minimum gap between any
// two media downloads (shared across runs via a pacing file), and never rm -rf
// user data — destructive ops move it to outputs/.trash/ instead.

// Cache layout:
//
//	outputs/.cache/.last_download   <- unix-ms of last real media download
//	outputs/.cache/<id>/            <- full-video master (mp4 + info.json)
//	outputs/.cache/bench-<id>/      <- 5-min 1080p bench master
//	outputs/.trash/                 <- moved-aside data (never hard-deleted)
const (
	cacheRootName = ".cache"
	trashRootName = ".trash"
	pacingFile    = ".last_download"
	doneMarker    = ".done"
)

func cacheRoot() string { return filepath.Join("outputs", cacheRootName) }
func trashRoot() string { return filepath.Join("outputs", trashRootName) }
func cacheDir(id string) string {
	return filepath.Join("outputs", cacheRootName, id)
}
func pacingPath() string { return filepath.Join(cacheRoot(), pacingFile) }

// envMinDownloadGap returns the enforced minimum seconds between media
// downloads (default 20s; VISION_MIN_DOWNLOAD_GAP overrides).
func envMinDownloadGap() float64 {
	return envFloat("VISION_MIN_DOWNLOAD_GAP", 20)
}

func envFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

// waitForPacing sleeps until the minimum inter-download gap since the last
// media download has elapsed. It is shared across every process/run through
// the pacing file, so running the tool repeatedly cannot hammer YouTube.
func waitForPacing() error {
	if err := os.MkdirAll(cacheRoot(), 0o755); err != nil {
		return err
	}
	last, err := readLastDownload()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	gap := time.Duration(envMinDownloadGap() * float64(time.Second))
	elapsed := time.Since(last)
	if elapsed < gap {
		wait := gap - elapsed
		fmt.Fprintf(os.Stderr, "[pacing] waiting %.0fs before next download (last was %.0fs ago)\n",
			wait.Seconds(), elapsed.Seconds())
		time.Sleep(wait)
	}
	return nil
}

// markDownloaded stamps the pacing file after a media download completes.
func markDownloaded() error {
	if err := os.MkdirAll(cacheRoot(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(pacingPath(), []byte(strconv.FormatInt(time.Now().UnixMilli(), 10)), 0o644)
}

func readLastDownload() (time.Time, error) {
	b, err := os.ReadFile(pacingPath())
	if err != nil {
		return time.Time{}, err
	}
	ms, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse pacing file: %w", err)
	}
	return time.UnixMilli(ms), nil
}

// trashPath returns a unique trash destination for a data directory.
func trashPath(id string) string {
	stamp := time.Now().Format("20060102-150405")
	return filepath.Join(trashRoot(), stamp+"-"+id)
}

// moveToTrash relocates outputs/<id> (or any path) to outputs/.trash/ instead
// of deleting it, so accidental --fresh runs are recoverable.
func moveToTrash(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to trash
		}
		return err
	}
	dst := trashPath(filepath.Base(dir))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Rename(dir, dst); err != nil {
		return fmt.Errorf("move %s -> %s: %w", dir, dst, err)
	}
	fmt.Fprintf(os.Stderr, "[trash] moved %s -> %s (recoverable)\n", dir, dst)
	return nil
}

// purgeCache removes the whole cache + trash trees (explicit user request).
func purgeCache() error {
	for _, d := range []string{cacheRoot(), trashRoot()} {
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}
	fmt.Println("purged outputs/.cache and outputs/.trash")
	return nil
}

// doneMarkerPath is the completion marker for a processed video.
func doneMarkerPath(id string) string {
	return filepath.Join(idDir(id), doneMarker)
}
