// Package chapter locates the "coding" chapter of a video (where frame
// extraction should begin) and writes chapter metadata to chapters.yaml.
package chapter

import (
	"fmt"
	"strings"

	"github.com/DeanT-04/yt-code-vision-skill/internal/media"
)

// positiveKeywords are weighted coding-intent words. Higher weight = stronger
// signal that a chapter is where actual coding begins. Chapter titles vary
// between creators ("Start of Coding", "Building the bot", "Let's write the
// code", ...) so we match case-insensitively on intent words.
var positiveKeywords = []struct {
	word   string
	weight int
}{
	{"live coding", 6},
	{"coding", 5},
	{"write the", 5},
	{"build", 4},
	{"implement", 4},
	{"from scratch", 4},
	{"code", 4},
	{"program", 3},
	{"develop", 3},
	{"hands-on", 3},
	{"walkthrough", 2},
	{"tutorial", 2},
}

// negativeKeywords discount a chapter from being the coding start. A chapter
// that matches any of these is almost never the code itself. Kept to lead-in /
// lead-out words only — words like "strategy" are deliberately NOT negatives
// because a chapter titled "Implementing the strategy" is still coding.
var negativeKeywords = []string{
	"intro", "backtest", "setup", "overview", "outro", "q&a", "recap",
	"thanks", "conclusion",
}

// negativePenalty is the per-hit score penalty for a negative keyword.
const negativePenalty = 5

// ChapterStart finds the second offset where frame extraction should begin.
//
//   - If override is non-empty, the first chapter whose title contains it
//     (case-insensitive) wins — lets you pin e.g. "Start of Coding".
//   - Otherwise every chapter is scored: positive intent words add their
//     weight, negative words subtract. The highest positive score wins.
//   - If nothing scores positive, the longest non-negative chapter is used as
//     a fallback (coding usually dominates runtime).
//
// Returns start seconds, the matched chapter title, and whether one matched.
// If no chapter qualifies (e.g. no chapters at all), it returns 0,"",false and
// callers should frame everything from the start rather than fail.
func ChapterStart(chapters []media.Chapter, override string) (float64, string, bool) {
	if o := strings.TrimSpace(override); o != "" {
		low := strings.ToLower(o)
		for _, c := range chapters {
			if strings.Contains(strings.ToLower(c.Title), low) {
				return c.StartTime, c.Title, true
			}
		}
		return 0, "", false
	}

	best := -1
	bestScore := 0
	for i, c := range chapters {
		if s := scoreChapter(c.Title); s > bestScore {
			bestScore = s
			best = i
		}
	}
	if best >= 0 && bestScore > 0 {
		return chapters[best].StartTime, chapters[best].Title, true
	}
	return longestNonNegative(chapters)
}

// scoreChapter sums positive-keyword weights and subtracts negative hits.
func scoreChapter(title string) int {
	low := strings.ToLower(title)
	score := 0
	for _, p := range positiveKeywords {
		if strings.Contains(low, p.word) {
			score += p.weight
		}
	}
	for _, n := range negativeKeywords {
		if strings.Contains(low, n) {
			score -= negativePenalty
		}
	}
	return score
}

// hasNegative reports whether a title matches any negative keyword.
func hasNegative(title string) bool {
	low := strings.ToLower(title)
	for _, n := range negativeKeywords {
		if strings.Contains(low, n) {
			return true
		}
	}
	return false
}

// longestNonNegative returns the longest chapter that isn't clearly negative
// (intro/backtest/setup/...), as a fallback when no chapter scores positive.
func longestNonNegative(chapters []media.Chapter) (float64, string, bool) {
	best := -1
	bestDur := 0.0
	for i, c := range chapters {
		if hasNegative(c.Title) {
			continue
		}
		if d := c.EndTime - c.StartTime; d > bestDur {
			bestDur = d
			best = i
		}
	}
	if best < 0 {
		return 0, "", false
	}
	return chapters[best].StartTime, chapters[best].Title, true
}

// FormatStartSection renders a start offset for a 5-minute bench window as a
// yt-dlp --download-sections pair like "START-END" in HH:MM:SS form.
func FormatStartSection(startSec, duration, windowSec float64) string {
	end := startSec + windowSec
	if end > duration {
		end = duration
	}
	return fmt.Sprintf("*%s-%s", Hms(startSec), Hms(end))
}

// Hms formats seconds as HH:MM:SS (yt-dlp/ffmpeg section syntax).
func Hms(sec float64) string {
	total := int(sec)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}
