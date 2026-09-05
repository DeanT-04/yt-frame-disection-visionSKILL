package chapter

import (
	"testing"

	"github.com/DeanT-04/yt-code-vision-skill/internal/media"
)

func testChapters() []media.Chapter {
	return []media.Chapter{
		{StartTime: 0, EndTime: 28, Title: "Intro"},
		{StartTime: 28, EndTime: 140, Title: "Strategy Backtest"},
		{StartTime: 140, EndTime: 218, Title: "The Strategy in the Trading Bot"},
		{StartTime: 218, EndTime: 378, Title: "Start of Coding"},
		{StartTime: 378, EndTime: 769, Title: "1st Visualization of Settings"},
		{StartTime: 769, EndTime: 1125, Title: "2nd Visualization of Settings"},
	}
}

func TestChapterStartAutoKeyword(t *testing.T) {
	start, title, ok := ChapterStart(testChapters(), "")
	if !ok {
		t.Fatal("expected a match")
	}
	if start != 218 {
		t.Fatalf("start = %v, want 218", start)
	}
	if title != "Start of Coding" {
		t.Fatalf("title = %q, want Start of Coding", title)
	}
}

func TestChapterStartKeywordVariants(t *testing.T) {
	cases := []struct {
		title string
		want  float64
	}{
		{"Let's build the bot", 30},
		{"Coding begins now", 10},
		{"Implementing the strategy", 20},
		{"Writing code from scratch", 40},
		{"Development phase", 50},
	}
	for _, c := range cases {
		ch := []media.Chapter{{StartTime: c.want, Title: c.title}}
		start, _, ok := ChapterStart(ch, "")
		if !ok {
			t.Errorf("%q: no match", c.title)
			continue
		}
		if start != c.want {
			t.Errorf("%q: start = %v, want %v", c.title, start, c.want)
		}
	}
}

func TestChapterStartOverride(t *testing.T) {
	// Explicit override pins a specific chapter even if earlier ones match.
	start, title, ok := ChapterStart(testChapters(), "Visualization")
	if !ok {
		t.Fatal("expected override match")
	}
	if start != 378 || title != "1st Visualization of Settings" {
		t.Fatalf("override: start=%v title=%q, want 378 '1st Visualization of Settings'", start, title)
	}
}

func TestChapterStartNoMatch(t *testing.T) {
	if _, _, ok := ChapterStart(testChapters(), "nonsense"); ok {
		t.Fatal("expected no match for unknown override")
	}
	if _, _, ok := ChapterStart(nil, ""); ok {
		t.Fatal("expected no match on empty chapters")
	}
}

// A chapter that only has negative words must lose to a genuinely positive one,
// even though the negative chapter comes first.
func TestChapterStartNegativeLosesToPositive(t *testing.T) {
	ch := []media.Chapter{
		{StartTime: 0, EndTime: 30, Title: "Intro & Overview"},
		{StartTime: 30, EndTime: 90, Title: "Environment Setup"},
		{StartTime: 90, EndTime: 300, Title: "Building the Feature"},
	}
	start, title, ok := ChapterStart(ch, "")
	if !ok {
		t.Fatal("expected a match")
	}
	if start != 90 || title != "Building the Feature" {
		t.Fatalf("start=%v title=%q, want 90 'Building the Feature'", start, title)
	}
}

// When no chapter scores positive, fall back to the longest non-negative
// chapter (coding usually dominates runtime) instead of framing from 0.
func TestChapterStartLongestFallback(t *testing.T) {
	ch := []media.Chapter{
		{StartTime: 0, EndTime: 20, Title: "Intro"},
		{StartTime: 20, EndTime: 60, Title: "Deep Dive"},
		{StartTime: 60, EndTime: 660, Title: "Main Session"},
	}
	start, title, ok := ChapterStart(ch, "")
	if !ok {
		t.Fatal("expected a fallback match")
	}
	if start != 60 || title != "Main Session" {
		t.Fatalf("start=%v title=%q, want 60 'Main Session'", start, title)
	}
}

// If every chapter is negative (or there are none), report no match so the
// caller frames from 0 rather than guessing.
func TestChapterStartAllNegative(t *testing.T) {
	ch := []media.Chapter{
		{StartTime: 0, EndTime: 20, Title: "Intro"},
		{StartTime: 20, EndTime: 60, Title: "Backtest Setup"},
	}
	if _, _, ok := ChapterStart(ch, ""); ok {
		t.Fatal("expected no match when every chapter is negative")
	}
}

func TestHmsAndSection(t *testing.T) {
	if got := Hms(218); got != "00:03:38" {
		t.Fatalf("Hms(218) = %q, want 00:03:38", got)
	}
	sec := FormatStartSection(218, 2166, 300)
	if sec != "*00:03:38-00:08:38" {
		t.Fatalf("section = %q, want *00:03:38-00:08:38", sec)
	}
	// window clipped at video end
	if got := FormatStartSection(2100, 2166, 300); got != "*00:35:00-00:36:06" {
		t.Fatalf("clipped section = %q, want *00:35:00-00:36:06", got)
	}
}
