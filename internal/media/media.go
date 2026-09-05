// Package media downloads YouTube videos via yt-dlp (with a persistent cache,
// cross-run pacing, and throttle backoff) and probes media duration via
// ffprobe. It also classifies yt-dlp failures so callers can distinguish a
// transient IP throttle from a deterministic missing-JS-runtime error.
package media

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/DeanT-04/yt-code-vision-skill/internal/cache"
)

// RunCmd executes a command, streaming output through to our stderr so
// progress from yt-dlp/ffmpeg is visible. It also captures the output and
// returns it (trimmed) so callers can inspect error text (e.g. detect 403/429).
func RunCmd(name string, args ...string) (string, error) {
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

// InfoJSON is the subset of yt-dlp's --dump-single-json we consume.
type InfoJSON struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Duration float64   `json:"duration"`
	Chapters []Chapter `json:"chapters"`
}

// Chapter is one video chapter as reported by yt-dlp.
type Chapter struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Title     string  `json:"title"`
}

// IsThrottle reports whether a download failure looks like a YouTube
// rate-limit / bot block (the kind a wait + retry can overcome) vs a real
// error (e.g. video unavailable). 403 and 429 are throttles; 404/410 etc. are
// not retryable.
func IsThrottle(errText string) bool {
	low := strings.ToLower(errText)
	for _, marker := range []string{"403", "429", "too many requests", "rate limit", "rate-limit"} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

// IsMissingJSRuntime reports whether a failure is yt-dlp's own diagnostic for
// "no JS runtime available" rather than an IP-level rate limit. Since
// yt-dlp 2025.11.12, YouTube's nsig/signature challenge requires an external
// JS runtime (Deno/Node/Bun/QuickJS); without one yt-dlp silently falls back
// to non-JS clients (e.g. android_vr), whose media URLs YouTube has been
// actively 403-blocking (see yt-dlp issues #17456, #16150 — an ongoing,
// unresolved YouTube-side change as of 2026, not something backoff fixes).
// This is deterministic and will NOT clear with time, unlike a real throttle.
func IsMissingJSRuntime(errText string) bool {
	return strings.Contains(strings.ToLower(errText), "no supported javascript runtime")
}

// jsRuntimeBinaries are the executable names yt-dlp's JS-challenge solver
// looks for (in yt-dlp's own order of recommendation). Deno is the only one
// yt-dlp enables by default (see https://github.com/yt-dlp/yt-dlp/wiki/EJS).
var jsRuntimeBinaries = []string{"deno", "node", "bun", "qjs"}

// HasJSRuntime does a cheap, local, no-network check for whether any
// JS-runtime binary yt-dlp can use is on PATH. Best-effort: yt-dlp does the
// authoritative check itself, this just lets us warn early instead of
// burning a full throttle-backoff cycle to discover it.
func HasJSRuntime() bool {
	for _, bin := range jsRuntimeBinaries {
		if _, err := exec.LookPath(bin); err == nil {
			return true
		}
	}
	return false
}

// DownloadVideo returns media for a video, using the persistent cache when
// possible. dir names the cache entry (outputs/.cache/<id> for full videos,
// outputs/.cache/bench-<id> for bench masters). If a complete media file +
// info JSON already exist there, it reuses them with ZERO network. Otherwise:
//
//  1. pace: enforce a shared minimum gap since the last media download,
//  2. warn (best-effort) if no JS runtime is on PATH — yt-dlp needs one for
//     YouTube's signature challenge and otherwise falls back to clients
//     YouTube has been 403-blocking for media fetches,
//  3. download with yt-dlp (video-only h264 mp4 at <=height; optional section),
//  4. on 403/429 throttle: exponential backoff (15s, 60s, 240s) then clean
//     stop — never a tight retry loop that makes the block worse. A missing
//     JS runtime is detected and treated as terminal immediately instead,
//     since (unlike a real throttle) it will not clear by waiting.
func DownloadVideo(dir, videoID, height, section string) (string, *InfoJSON, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	infoPath := filepath.Join(dir, videoID+".info.json")
	mediaPath := filepath.Join(dir, videoID+".mp4")

	// Cache hit: both files exist and the media is plausibly complete.
	if info, err := ReadInfoJSON(infoPath); err == nil {
		if st, statErr := os.Stat(mediaPath); statErr == nil && st.Size() > 1<<20 {
			fmt.Fprintf(os.Stderr, "[cache] reusing %s (%d MB) — no download\n", mediaPath, st.Size()>>20)
			return mediaPath, info, nil
		}
	}

	// Prepare: drop stale partials so yt-dlp starts clean.
	_ = os.Remove(mediaPath + ".part")
	_ = os.Remove(mediaPath)

	if err := cache.WaitForPacing(); err != nil {
		return "", nil, err
	}

	if !HasJSRuntime() {
		fmt.Fprintln(os.Stderr, "[warn] no JS runtime (deno/node/bun/qjs) found on PATH — "+
			"yt-dlp will fall back to non-JS clients (e.g. android_vr) whose media fetches "+
			"YouTube has been 403-blocking. Install Deno (`scoop install deno` / `winget install DenoLand.Deno`) "+
			"and re-run. See https://github.com/yt-dlp/yt-dlp/wiki/EJS")
	}

	// NOTE: intentionally NOT --no-warnings. yt-dlp's own warnings (esp. "No
	// supported JavaScript runtime could be found…") are the single best
	// diagnostic for the 403-on-media-only failure mode below, and suppressing
	// them made a real, non-transient config problem look identical to a
	// transient IP throttle in every log we captured.
	format := fmt.Sprintf("bv*[height<=%s][vcodec^=avc1]/bv*[height<=%s]", height, height)
	args := []string{
		"--no-playlist",
		"-f", format,
		"--write-info-json",
		"-o", filepath.Join(dir, videoID+".%(ext)s"),
	}
	if section != "" {
		args = append(args, "--download-sections", section)
	}
	args = append(args, "--", VideoURL(videoID))

	// Throttle backoff ladder (patient, then clean stop): try, then wait
	// 15s / 60s / 240s between the next 3 attempts — ~5.5 min worst case.
	// This ladder assumes the block DECAYS with time, which is only true for
	// a real IP-level rate limit. A missing-JS-runtime failure is
	// deterministic (see IsMissingJSRuntime) — retrying it on the same
	// ladder just wastes 5.5 minutes reproducing the same outcome, so that
	// case exits immediately with an actionable message instead.
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
		_, lastErr = RunCmd("yt-dlp", args...)
		// Every real attempt hit YouTube's media endpoint (success or not) —
		// pace the NEXT attempt (this run or a future process) off of it, not
		// just off successes. Otherwise a fully-blocked run leaves no trace
		// and a repeated `go run .` immediately re-fires with zero spacing.
		if markErr := cache.MarkDownloaded(); markErr != nil {
			return "", nil, markErr
		}
		if lastErr == nil {
			break
		}
		if IsMissingJSRuntime(lastErr.Error()) {
			return "", nil, fmt.Errorf(
				"yt-dlp has no JS runtime available, so it fell back to a client YouTube is "+
					"currently 403-blocking for media fetches — this will NOT clear on its own. "+
					"Install Deno (`scoop install deno` on Windows, or see "+
					"https://github.com/yt-dlp/yt-dlp/wiki/EJS) and re-run: %w", lastErr)
		}
		if !IsThrottle(lastErr.Error()) {
			// Real error (unavailable, geo, bad format): don't waste time.
			return "", nil, lastErr
		}
		fmt.Fprintf(os.Stderr, "[throttle] attempt %d/%d blocked by YouTube (%v)\n", attempt, maxAttempts, lastErr)
	}
	if lastErr != nil {
		return "", nil, fmt.Errorf("download blocked by YouTube after %d attempts — try again later: %w", maxAttempts, lastErr)
	}

	// yt-dlp --write-info-json emits <id>.info.json next to the media.
	info, err := ReadInfoJSON(infoPath)
	if err != nil {
		return "", nil, err
	}
	if st, err := os.Stat(mediaPath); err != nil || st.Size() < 1<<20 {
		return "", nil, fmt.Errorf("expected media file missing or too small after download: %v", err)
	}
	fmt.Fprintf(os.Stderr, "[cache] stored %s for reuse\n", mediaPath)
	return mediaPath, info, nil
}

// FetchMeta gets a video's info JSON (title/duration/chapters) WITHOUT
// downloading media — used by bench to pick the coding-chapter window before
// fetching the 5-minute master.
func FetchMeta(dir, videoID string) (*InfoJSON, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	args := []string{
		"--no-playlist",
		"--skip-download",
		"--write-info-json",
		"-o", filepath.Join(dir, videoID+".%(ext)s"),
		"--", VideoURL(videoID),
	}
	if _, err := RunCmd("yt-dlp", args...); err != nil {
		return nil, err
	}
	return ReadInfoJSON(filepath.Join(dir, videoID+".info.json"))
}

// VideoURL builds the canonical watch URL for a video id.
func VideoURL(videoID string) string {
	return "https://www.youtube.com/watch?v=" + videoID
}

// ReadInfoJSON parses a yt-dlp --write-info-json output file.
func ReadInfoJSON(path string) (*InfoJSON, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read info json: %w", err)
	}
	var info InfoJSON
	if err := json.Unmarshal(b, &info); err != nil {
		return nil, fmt.Errorf("parse info json: %w", err)
	}
	return &info, nil
}

// ProbeDuration returns the media duration in seconds via ffprobe.
func ProbeDuration(mediaPath string) (float64, error) {
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
