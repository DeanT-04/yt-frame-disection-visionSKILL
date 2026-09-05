package chapter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/DeanT-04/yt-code-vision-skill/internal/media"
)

func TestWriteChaptersYAMLWithChapters(t *testing.T) {
	dir := t.TempDir()
	info := &media.InfoJSON{
		ID:    "mtWN6oPIi1Y",
		Title: "The Wayward Trading Bot... The Wait Is OVER!! (Full Code)",
		Chapters: []media.Chapter{
			{StartTime: 0, EndTime: 28, Title: "Intro"},
			{StartTime: 28, EndTime: 140, Title: "Strategy Backtest"},
			{StartTime: 140, EndTime: 218, Title: "The Strategy in the Trading Bot"},
		},
	}
	path, err := WriteChaptersYAML(dir, info)
	if err != nil {
		t.Fatalf("WriteChaptersYAML: %v", err)
	}
	// File must be outputs-style <dir>/chapters/chapters.yaml.
	if want := filepath.Join(dir, "chapters", "chapters.yaml"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Contains(got, "NaN") {
		t.Errorf("chapters.yaml should not contain NaN: %s", got)
	}
	var out chaptersYAML
	if err := yaml.Unmarshal(b, &out); err != nil {
		t.Fatalf("yaml.Unmarshal: %v\n---\n%s", err, got)
	}
	if out.VideoID != "mtWN6oPIi1Y" {
		t.Errorf("video_id = %q, want mtWN6oPIi1Y", out.VideoID)
	}
	if out.Chapters == nil {
		t.Fatal("chapters = nil, want 3")
	}
	if len(*out.Chapters) != 3 {
		t.Fatalf("chapters = %d, want 3", len(*out.Chapters))
	}
	if (*out.Chapters)[0].StartTime != 0 || (*out.Chapters)[0].EndTime != 28 {
		t.Errorf("chapter[0] = %+v, want start 0 end 28", (*out.Chapters)[0])
	}
	if (*out.Chapters)[2].EndTime != 218 {
		t.Errorf("chapter[2].EndTime = %v, want 218", (*out.Chapters)[2].EndTime)
	}
}

func TestWriteChaptersYAMLNoChapters(t *testing.T) {
	dir := t.TempDir()
	info := &media.InfoJSON{ID: "abc123def45", Title: "No chapters here"}
	path, err := WriteChaptersYAML(dir, info)
	if err != nil {
		t.Fatalf("WriteChaptersYAML: %v", err)
	}
	b, _ := os.ReadFile(path)
	got := string(b)
	if !strings.Contains(got, "chapters: null") {
		t.Errorf("expected 'chapters: null' for empty chapters, got:\n%s", got)
	}
	if strings.Contains(got, "NaN") {
		t.Errorf("chapters.yaml should not contain NaN: %s", got)
	}
	var out chaptersYAML
	if err := yaml.Unmarshal(b, &out); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if out.Chapters != nil {
		t.Fatalf("chapters = %v, want nil", *out.Chapters)
	}
}
