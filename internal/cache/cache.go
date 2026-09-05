// Package cache owns the persistent, project-local media cache, cross-run
// download pacing, and the trash/`--fresh` flow.
//
// Why: repeated full downloads of the same video hammer YouTube and trigger
// per-IP 403 throttling. We keep downloaded masters in outputs/.cache/<id>/ so
// re-runs reuse them with zero network, enforce a minimum gap between any two
// media downloads (shared across runs via a pacing file), and never rm -rf
// user data — destructive ops move it to outputs/.trash/ instead.
package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DeanT-04/yt-code-vision-skill/internal/paths"
)

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

// CacheRoot is outputs/.cache.
func CacheRoot() string { return filepath.Join("outputs", cacheRootName) }

// TrashRoot is outputs/.trash.
func TrashRoot() string { return filepath.Join("outputs", trashRootName) }

// CacheDir is the cache entry for one video id (e.g. outputs/.cache/<id>).
func CacheDir(id string) string {
	return filepath.Join("outputs", cacheRootName, id)
}

// PacingPath is the shared pacing timestamp file.
func PacingPath() string { return filepath.Join(CacheRoot(), pacingFile) }

// EnvMinDownloadGap returns the enforced minimum seconds between media
// downloads (default 20s; VISION_MIN_DOWNLOAD_GAP overrides).
func EnvMinDownloadGap() float64 {
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

// WaitForPacing sleeps until the minimum inter-download gap since the last
// media download has elapsed. It is shared across every process/run through
// the pacing file, so running the tool repeatedly cannot hammer YouTube.
func WaitForPacing() error {
	if err := os.MkdirAll(CacheRoot(), 0o755); err != nil {
		return err
	}
	last, err := readLastDownload()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	gap := time.Duration(EnvMinDownloadGap() * float64(time.Second))
	elapsed := time.Since(last)
	if elapsed < gap {
		wait := gap - elapsed
		fmt.Fprintf(os.Stderr, "[pacing] waiting %.0fs before next download (last was %.0fs ago)\n",
			wait.Seconds(), elapsed.Seconds())
		time.Sleep(wait)
	}
	return nil
}

// MarkDownloaded stamps the pacing file after a real attempt to fetch media
// from YouTube (success OR failure — a blocked attempt still hit the media
// endpoint and should still be paced, see downloadVideo).
func MarkDownloaded() error {
	if err := os.MkdirAll(CacheRoot(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(PacingPath(), []byte(strconv.FormatInt(time.Now().UnixMilli(), 10)), 0o644)
}

func readLastDownload() (time.Time, error) {
	b, err := os.ReadFile(PacingPath())
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
	return filepath.Join(TrashRoot(), stamp+"-"+id)
}

// MoveToTrash relocates outputs/<id> (or any path) to outputs/.trash/ instead
// of deleting it, so accidental --fresh runs are recoverable.
func MoveToTrash(dir string) error {
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

// PurgeCache removes the whole cache + trash trees (explicit user request).
func PurgeCache() error {
	for _, d := range []string{CacheRoot(), TrashRoot()} {
		if err := os.RemoveAll(d); err != nil {
			return err
		}
	}
	fmt.Println("purged outputs/.cache and outputs/.trash")
	return nil
}

// DoneMarkerPath is the completion marker for a processed video.
func DoneMarkerPath(id string) string {
	return filepath.Join(paths.IDDir(id), doneMarker)
}
