package main

import (
	"fmt"
	"strings"
)

// codingKeywords mark the chapter where actual coding begins. Chapter titles
// vary between creators ("Start of Coding", "Building the bot", "Let's write
// the code", ...) so we match case-insensitively on intent words. Kept as a
// heuristic list for now — refinements live in docs/future-ideas.yaml.
var codingKeywords = []string{
	"coding", "code", "build", "implement", "write the", "program", "develop",
}

// chapterStart finds the second offset where frame extraction should begin.
//   - If override is non-empty, the first chapter whose title contains it
//     (case-insensitive) wins — lets you pin e.g. "Start of Coding".
//   - Otherwise the first chapter whose title matches a coding intent keyword.
//
// Returns start seconds, the matched chapter title, and whether one matched.
// Every video is expected to have chapters; if none match, callers should
// fall back to 0 (frame everything) rather than fail.
func chapterStart(chapters []chapter, override string) (float64, string, bool) {
	hasOverride := strings.TrimSpace(override) != ""
	terms := codingKeywords
	if hasOverride {
		terms = []string{strings.ToLower(strings.TrimSpace(override))}
	}
	for _, c := range chapters {
		low := strings.ToLower(c.Title)
		for _, t := range terms {
			if strings.Contains(low, strings.ToLower(t)) {
				return c.StartTime, c.Title, true
			}
		}
	}
	return 0, "", false
}

// chapter mirrors the infoJSON chapters slice for matcher unit tests.
type chapter struct {
	StartTime float64
	EndTime   float64
	Title     string
}

// toChapters converts the parsed info JSON chapter slice to the matcher type.
func toChapters(infos []struct {
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Title     string  `json:"title"`
}) []chapter {
	out := make([]chapter, 0, len(infos))
	for _, c := range infos {
		out = append(out, chapter{StartTime: c.StartTime, EndTime: c.EndTime, Title: c.Title})
	}
	return out
}

// formatStartSection renders a start offset for a 5-minute bench window as a
// yt-dlp --download-sections pair like "START-END" in HH:MM:SS form.
func formatStartSection(startSec, duration, windowSec float64) string {
	end := startSec + windowSec
	if end > duration {
		end = duration
	}
	return fmt.Sprintf("*%s-%s", hms(startSec), hms(end))
}

// hms formats seconds as HH:MM:SS (yt-dlp/ffmpeg section syntax).
func hms(sec float64) string {
	total := int(sec)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}
