package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// runCmd executes a command, streaming output through to our stderr so
// progress from yt-dlp/ffmpeg is visible. It also captures the output and
// returns it (trimmed) so callers can inspect error text (e.g. detect 403/429).
func runCmd(name string, args ...string) (string, error) {
	//nolint:gosec // name is a hardcoded binary ("yt-dlp"/"ffmpeg"); args passed via exec.Command, never a shell
	cmd := exec.Command(name, args...)
	var buf strings.Builder
	mw := io.MultiWriter(os.Stderr, &buf)
	cmd.Stdout = mw
	cmd.Stderr = mw
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(buf.String())
		if out != "" {
			return "", fmt.Errorf("%s %s: %w (%s)", name, strings.Join(args, " "), err, out)
		}
		return "", fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// infoJSON is the subset of yt-dlp's --dump-single-json we consume.
type infoJSON struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Duration float64 `json:"duration"`
	Chapters []struct {
		StartTime float64 `json:"start_time"`
		EndTime   float64 `json:"end_time"`
		Title     string  `json:"title"`
	} `json:"chapters"`
}

// isThrottle reports whether a download failure looks like a YouTube
// rate-limit / bot block (the kind a wait + retry can overcome) vs a real
// error (e.g. video unavailable). 403 and 429 are throttles; 404/410 etc. are
// not retryable.
func isThrottle(errText string) bool {
	low := strings.ToLower(errText)
	for _, marker := range []string{"403", "429", "too many requests", "rate limit", "rate-limit"} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

// downloadVideo returns media for a video, using the persistent cache when
// possible. dir names the cache entry (outputs/.cache/<id> for full videos,
// outputs/.cache/bench-<id> for bench masters). If a complete media file +
// info JSON already exist there, it reuses them with ZERO network. Otherwise:
//
//  1. pace: enforce a shared minimum gap since the last media download,
//  2. download with yt-dlp (video-only h264 mp4 at <=height; optional section),
//  3. on 403/429 throttle: exponential backoff (15s, 60s, 240s) then clean
//     stop — never a tight retry loop that makes the block worse.
func downloadVideo(dir, videoID, height, section string) (string, *infoJSON, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	infoPath := filepath.Join(dir, videoID+".info.json")
	mediaPath := filepath.Join(dir, videoID+".mp4")

	// Cache hit: both files exist and the media is plausibly complete.
	if info, err := readInfoJSON(infoPath); err == nil {
		if st, statErr := os.Stat(mediaPath); statErr == nil && st.Size() > 1<<20 {
			fmt.Fprintf(os.Stderr, "[cache] reusing %s (%d MB) — no download\n", mediaPath, st.Size()>>20)
			return mediaPath, info, nil
		}
	}

	// Prepare: drop stale partials so yt-dlp starts clean.
	_ = os.Remove(mediaPath + ".part")
	_ = os.Remove(mediaPath)

	if err := waitForPacing(); err != nil {
		return "", nil, err
	}

	format := fmt.Sprintf("bv*[height<=%s][vcodec^=avc1]/bv*[height<=%s]", height, height)
	args := []string{
		"--no-playlist",
		"--no-warnings",
		"-f", format,
		"--write-info-json",
		"-o", filepath.Join(dir, videoID+".%(ext)s"),
	}
	if section != "" {
		args = append(args, "--download-sections", section)
	}
	args = append(args, "--", videoURL(videoID))

	// Throttle backoff ladder (patient, then clean stop): try, then wait
	// 15s / 60s / 240s between the next 3 attempts — ~5.5 min worst case.
	backoff := []time.Duration{15 * time.Second, 60 * time.Second, 240 * time.Second}
	const maxAttempts = 4
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			wait := backoff[min(attempt-2, len(backoff)-1)]
			fmt.Fprintf(os.Stderr, "[throttle] attempt %d/%d blocked; waiting %s before retry…\n",
				attempt-1, maxAttempts, wait)
			time.Sleep(wait)
		}
		_, lastErr = runCmd("yt-dlp", args...)
		if lastErr == nil {
			break
		}
		if !isThrottle(lastErr.Error()) {
			// Real error (unavailable, geo, bad format): don't waste time.
			return "", nil, lastErr
		}
		fmt.Fprintf(os.Stderr, "[throttle] attempt %d/%d blocked by YouTube (%v)\n", attempt, maxAttempts, lastErr)
	}
	if lastErr != nil {
		return "", nil, fmt.Errorf("download blocked by YouTube after %d attempts — try again later: %w", maxAttempts, lastErr)
	}
	if err := markDownloaded(); err != nil {
		return "", nil, err
	}

	// yt-dlp --write-info-json emits <id>.info.json next to the media.
	info, err := readInfoJSON(infoPath)
	if err != nil {
		return "", nil, err
	}
	if st, err := os.Stat(mediaPath); err != nil || st.Size() < 1<<20 {
		return "", nil, fmt.Errorf("expected media file missing or too small after download: %v", err)
	}
	fmt.Fprintf(os.Stderr, "[cache] stored %s for reuse\n", mediaPath)
	return mediaPath, info, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// fetchMeta gets a video's info JSON (title/duration/chapters) WITHOUT
// downloading media — used by bench to pick the coding-chapter window before
// fetching the 5-minute master.
func fetchMeta(dir, videoID string) (*infoJSON, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	args := []string{
		"--no-playlist",
		"--no-warnings",
		"--skip-download",
		"--write-info-json",
		"-o", filepath.Join(dir, videoID+".%(ext)s"),
		"--", videoURL(videoID),
	}
	if _, err := runCmd("yt-dlp", args...); err != nil {
		return nil, err
	}
	return readInfoJSON(filepath.Join(dir, videoID+".info.json"))
}

func videoURL(videoID string) string {
	return "https://www.youtube.com/watch?v=" + videoID
}

func readInfoJSON(path string) (*infoJSON, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read info json: %w", err)
	}
	var info infoJSON
	if err := json.Unmarshal(b, &info); err != nil {
		return nil, fmt.Errorf("parse info json: %w", err)
	}
	return &info, nil
}

// probeDuration returns the media duration in seconds via ffprobe.
func probeDuration(mediaPath string) (float64, error) {
	//nolint:gosec // mediaPath is a local file path passed as an exec argument (no shell involved)
	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		mediaPath).Output()
	if err != nil {
		return 0, fmt.Errorf("ffprobe: %w", err)
	}
	var d float64
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out)), "%f", &d); err != nil {
		return 0, fmt.Errorf("parse duration %q: %w", strings.TrimSpace(string(out)), err)
	}
	return d, nil
}
