package chapter

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/DeanT-04/yt-code-vision-skill/internal/media"
)

// chaptersYAML is the on-disk chapters.yaml schema. `chapters` is a nil
// pointer when the video has none, so it marshals to a spec-compliant
// `chapters: null` (never the invalid `NaN` sentinel the old hand-rolled
// writer emitted).
type chaptersYAML struct {
	VideoID  string         `yaml:"video_id"`
	Title    string         `yaml:"title"`
	Chapters *[]chapterYAML `yaml:"chapters"`
}

type chapterYAML struct {
	Title     string  `yaml:"title"`
	StartTime float64 `yaml:"start_time"`
	EndTime   float64 `yaml:"end_time"`
}

// WriteChaptersYAML writes the video chapters into dir/chapters.yaml as
// spec-compliant YAML (via yaml.v3). Time fields are exact float seconds as
// reported by yt-dlp, so downstream math stays lossless.
func WriteChaptersYAML(dir string, info *media.InfoJSON) (string, error) {
	chaptersDir := filepath.Join(dir, "chapters")
	if err := os.MkdirAll(chaptersDir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(chaptersDir, "chapters.yaml")

	out := chaptersYAML{VideoID: info.ID, Title: info.Title}
	if len(info.Chapters) > 0 {
		chs := make([]chapterYAML, 0, len(info.Chapters))
		for _, c := range info.Chapters {
			chs = append(chs, chapterYAML{
				Title:     c.Title,
				StartTime: c.StartTime,
				EndTime:   c.EndTime,
			})
		}
		out.Chapters = &chs
	}

	b, err := yaml.Marshal(out)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	return path, nil
}
