package extract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestTuneThreshold prints the state count at several thresholds over a real
// frameset. Gated behind EXTRACT_TUNE_DIR (never runs in CI / normal tests) so
// the tuning decision stays a manual, documented operation:
//
//	EXTRACT_TUNE_DIR=outputs/mtWN6oPIi1Y/frames go test ./internal/extract -run TestTuneThreshold -v
func TestTuneThreshold(t *testing.T) {
	dir := os.Getenv("EXTRACT_TUNE_DIR")
	if dir == "" {
		t.Skip("set EXTRACT_TUNE_DIR to a real frames dir to tune the threshold")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	rect := readCropJSONFor(t, filepath.Join(abs, "..", "crop.json"))
	for _, th := range []float64{0.004, 0.015, 0.06} {
		states, err := DetectStates(abs, Options{Threshold: th, Crop: rect})
		if err != nil {
			t.Fatalf("threshold %.4f: %v", th, err)
		}
		t.Logf("threshold %.4f -> %d states", th, len(states))
	}
}

func readCropJSONFor(t *testing.T, path string) *Rect {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read crop: %v", err)
	}
	var r struct {
		X, Y, Width, Height int
	}
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("parse crop %s: %v", path, err)
	}
	return &Rect{X: r.X, Y: r.Y, W: r.Width, H: r.Height}
}
