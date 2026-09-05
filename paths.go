package main

import "path/filepath"

// Central layout for one video's outputs (all under the project root):
//
//	outputs/<id>/            <- video home
//	  frames/                <- full-res 1fps frames (frame_%06d.jpg)
//	  chapters/chapters.yaml <- chapter metadata
//	  crop.json              <- code-pane crop rect
//	  crop/                  <- cropped code-pane frames
//	  previews/              <- marked-up preview images (green box)
//	bench/<id>/<res>p/       <- resolution-benchmark frames (kept top-level)
func idDir(id string) string       { return filepath.Join("outputs", id) }
func framesDir(id string) string   { return filepath.Join("outputs", id, "frames") }
func chaptersDir(id string) string { return filepath.Join("outputs", id, "chapters") }
func cropsDir(id string) string    { return filepath.Join("outputs", id, "crop") }
func previewsDir(id string) string { return filepath.Join("outputs", id, "previews") }
