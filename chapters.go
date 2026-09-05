package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// writeChaptersYAML writes the video chapters into dir/chapters.yaml. When no
// chapters exist the file contains just "chapters: NaN". Time fields are exact
// float seconds (as reported by yt-dlp) so downstream math stays lossless.
func writeChaptersYAML(dir string, info *infoJSON) (string, error) {
	chaptersDir := filepath.Join(dir, "chapters")
	if err := os.MkdirAll(chaptersDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(chaptersDir, "chapters.yaml")

	var sb strings.Builder
	sb.WriteString("video_id: " + yamlScalar(info.ID) + "\n")
	sb.WriteString("title: " + yamlScalar(info.Title) + "\n")
	if len(info.Chapters) == 0 {
		sb.WriteString("chapters: NaN\n")
	} else {
		sb.WriteString("chapters:\n")
		for _, c := range info.Chapters {
			sb.WriteString("  - title: " + yamlScalar(c.Title) + "\n")
			sb.WriteString("    start_time: " + formatSeconds(c.StartTime) + "\n")
			sb.WriteString("    end_time: " + formatSeconds(c.EndTime) + "\n")
		}
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// yamlScalar safely renders a string as a single-line YAML scalar.
func yamlScalar(s string) string {
	needQuote := s == "" ||
		strings.ContainsAny(s, ":#{}[],&*!|>'\"%@`") ||
		strings.HasPrefix(s, "-") ||
		strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") ||
		strings.Contains(s, "\n")
	if !needQuote {
		return s
	}
	return strconv.Quote(s)
}

// formatSeconds renders a float second count without needless trailing zeros.
func formatSeconds(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	return s
}

// readChapterCount parses back the number of chapters from a chapters.yaml
// (used by tests/verification). Returns -1 for the NaN sentinel.
func readChapterCount(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	text := string(b)
	if strings.Contains(text, "chapters: NaN") {
		return -1, nil
	}
	count := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "  - title: ") {
			count++
		}
	}
	if count == 0 {
		return 0, fmt.Errorf("no chapters found in %s", path)
	}
	return count, nil
}
