package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteChaptersYAMLWithChapters(t *testing.T) {
	dir := t.TempDir()
	info := &infoJSON{
		ID:    "mtWN6oPIi1Y",
		Title: "The Wayward Trading Bot... The Wait Is OVER!! (Full Code)",
		Chapters: []struct {
			StartTime float64 `json:"start_time"`
			EndTime   float64 `json:"end_time"`
			Title     string  `json:"title"`
		}{
			{StartTime: 0, EndTime: 28, Title: "Intro"},
			{StartTime: 28, EndTime: 140, Title: "Strategy Backtest"},
			{StartTime: 140, EndTime: 218, Title: "The Strategy in the Trading Bot"},
		},
	}
	path, err := writeChaptersYAML(dir, info)
	if err != nil {
		t.Fatalf("writeChaptersYAML: %v", err)
	}
	// File must be outputs-style <dir>/chapters/chapters.yaml.
	if want := filepath.Join(dir, "chapters", "chapters.yaml"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	n, err := readChapterCount(path)
	if err != nil {
		t.Fatalf("readChapterCount: %v", err)
	}
	if n != 3 {
		t.Fatalf("chapter count = %d, want 3", n)
	}
	b, _ := os.ReadFile(path)
	got := string(b)
	for _, want := range []string{"video_id: mtWN6oPIi1Y", "start_time: 28", "end_time: 218", "chapters:"} {
		if !contains(got, want) {
			t.Errorf("chapters.yaml missing %q\n---\n%s", want, got)
		}
	}
	if contains(got, "NaN") {
		t.Errorf("chapters.yaml should not contain NaN when chapters exist")
	}
}

func TestWriteChaptersYAMLNaN(t *testing.T) {
	dir := t.TempDir()
	info := &infoJSON{ID: "abc123def45", Title: "No chapters here"}
	path, err := writeChaptersYAML(dir, info)
	if err != nil {
		t.Fatalf("writeChaptersYAML: %v", err)
	}
	n, err := readChapterCount(path)
	if err != nil {
		t.Fatalf("readChapterCount: %v", err)
	}
	if n != -1 {
		t.Fatalf("chapter count = %d, want -1 (NaN sentinel)", n)
	}
	b, _ := os.ReadFile(path)
	if !contains(string(b), "chapters: NaN") {
		t.Errorf("expected 'chapters: NaN' sentinel, got:\n%s", b)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
