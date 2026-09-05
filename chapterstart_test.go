package main

import (
	"testing"
)

func testChapters() []chapter {
	return []chapter{
		{StartTime: 0, EndTime: 28, Title: "Intro"},
		{StartTime: 28, EndTime: 140, Title: "Strategy Backtest"},
		{StartTime: 140, EndTime: 218, Title: "The Strategy in the Trading Bot"},
		{StartTime: 218, EndTime: 378, Title: "Start of Coding"},
		{StartTime: 378, EndTime: 769, Title: "1st Visualization of Settings"},
		{StartTime: 769, EndTime: 1125, Title: "2nd Visualization of Settings"},
	}
}

func TestChapterStartAutoKeyword(t *testing.T) {
	start, title, ok := chapterStart(testChapters(), "")
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
		ch := []chapter{{StartTime: c.want, Title: c.title}}
		start, _, ok := chapterStart(ch, "")
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
	start, title, ok := chapterStart(testChapters(), "Visualization")
	if !ok {
		t.Fatal("expected override match")
	}
	if start != 378 || title != "1st Visualization of Settings" {
		t.Fatalf("override: start=%v title=%q, want 378 '1st Visualization of Settings'", start, title)
	}
}

func TestChapterStartNoMatch(t *testing.T) {
	if _, _, ok := chapterStart(testChapters(), "nonsense"); ok {
		t.Fatal("expected no match for unknown override")
	}
	if _, _, ok := chapterStart(nil, ""); ok {
		t.Fatal("expected no match on empty chapters")
	}
}

func TestHmsAndSection(t *testing.T) {
	if got := hms(218); got != "00:03:38" {
		t.Fatalf("hms(218) = %q, want 00:03:38", got)
	}
	sec := formatStartSection(218, 2166, 300)
	if sec != "*00:03:38-00:08:38" {
		t.Fatalf("section = %q, want *00:03:38-00:08:38", sec)
	}
	// window clipped at video end
	if got := formatStartSection(2100, 2166, 300); got != "*00:35:00-00:36:06" {
		t.Fatalf("clipped section = %q, want *00:35:00-00:36:06", got)
	}
}
