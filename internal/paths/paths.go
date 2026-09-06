// Package paths is the single source of truth for the on-disk output layout.
// Every artifact (frames, chapters, crops, bench) lives under the project
// root; callers use these helpers instead of hand-building paths so the layout
// can never drift between call sites.
package paths

import (
	"fmt"
	"path/filepath"
)

// One video's output layout (all under the project root):
//
//	outputs/<id>/            <- video home
//	  frames/                <- full-res 1fps frames (frame_%06d.jpg)
//	  chapters/chapters.yaml <- chapter metadata
//	  crop.json              <- code-pane crop rect
//	  crop/                  <- cropped code-pane frames
//	  previews/              <- marked-up preview images (green box)
//	bench/<id>/<res>p/       <- resolution-benchmark frames (kept top-level)
func IDDir(id string) string       { return filepath.Join("outputs", id) }
func FramesDir(id string) string   { return filepath.Join("outputs", id, "frames") }
func ChaptersDir(id string) string { return filepath.Join("outputs", id, "chapters") }
func CropsDir(id string) string    { return filepath.Join("outputs", id, "crop") }
func PreviewsDir(id string) string { return filepath.Join("outputs", id, "previews") }

// BenchDir returns the top-level benchmark root for a video.
func BenchDir(id string) string { return filepath.Join("bench", id) }

// BenchResDir returns the benchmark frame folder for one resolution ladder
// step (e.g. "bench/<id>/480p").
func BenchResDir(id string, heightPx int) string {
	return filepath.Join("bench", id, fmt.Sprintf("%dp", heightPx))
}

// BenchResCropDir returns the cropped benchmark frame folder for one resolution
// ladder step (e.g. "bench/<id>/480p-crop").
func BenchResCropDir(id string, heightPx int) string {
	return filepath.Join("bench", id, fmt.Sprintf("%dp-crop", heightPx))
}

// CodeDir is where the code-extraction pipeline works (outputs/<id>/code).
func CodeDir(id string) string { return filepath.Join("outputs", id, "code") }

// CodeTranscriptsDir holds one verbatim transcript per state
// (outputs/<id>/code/transcripts/state_%06d.txt).
func CodeTranscriptsDir(id string) string { return filepath.Join("outputs", id, "code", "transcripts") }

// CodeStatesDir holds one pane-cropped image per state
// (outputs/<id>/code/states/state_%06d.jpg), staged for vision transcription.
func CodeStatesDir(id string) string { return filepath.Join("outputs", id, "code", "states") }
