package bench

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/DeanT-04/yt-code-vision-skill/internal/crop"
	"github.com/DeanT-04/yt-code-vision-skill/internal/paths"
)

// benchMasterHeight is the resolution the 5-minute bench master is downloaded
// at, and the resolution crop.json coordinates are expressed in.
const benchMasterHeight = 1080

// fidelityThreshold is the minimum token-overlap vs the 1080p baseline for a
// resolution to count as "code-readable".
const fidelityThreshold = 0.9

// visionQuestion is asked per resolution; the helper transcribes code exactly.
const visionQuestion = "These are cropped frames from a coding-tutorial screen recording. Transcribe the visible code exactly, in order, ignoring editor chrome."

// resExtract is one resolution's extracted code text.
type resExtract struct {
	height int
	text   string
}

// RunVerdict compares code readability across bench resolutions: it crops the
// code pane out of each resolution's frames, runs the vision-inspect helper
// (read mode) over a deduped sample, diffs each against the 1080p baseline, and
// writes bench/<id>/verdict.md recommending the lowest readable resolution.
//
// It calls the DeepSeek vision API (spends tokens) — only run it deliberately.
func RunVerdict(id string) error {
	// crop.json is required — we only want to read the code pane, not chrome.
	c, err := crop.Load(paths.IDDir(id))
	if err != nil {
		return fmt.Errorf("need crop.json first (run --find-crop): %w", err)
	}

	for _, h := range benchHeights {
		if !dirHasFrames(paths.BenchResDir(id, h)) {
			return fmt.Errorf("no bench frames at %dp — run --bench first", h)
		}
	}

	helper, err := inspectHelper()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("node is required to run the vision-inspect helper: %w", err)
	}

	fmt.Printf("running vision-inspect over %d resolutions (calls the DeepSeek API — spends tokens)\n", len(benchHeights))

	extracts := make([]resExtract, 0, len(benchHeights))
	for _, h := range benchHeights {
		cropDir := paths.BenchResCropDir(id, h)
		scaled := crop.Scale(*c, float64(h)/benchMasterHeight)
		if n, _, err := crop.CropDir(paths.BenchResDir(id, h), cropDir, scaled); err != nil {
			return err
		} else if n == 0 {
			return fmt.Errorf("%dp: crop produced no frames (crop box outside the frame?)", h)
		}
		text, err := runVision(helper, cropDir)
		if err != nil {
			return fmt.Errorf("%dp vision read: %w", h, err)
		}
		extracts = append(extracts, resExtract{height: h, text: text})
	}

	base := baselineText(extracts)
	recommended, ok := recommendLowest(extracts, fidelityThreshold)
	if err := writeVerdict(paths.BenchDir(id), extracts, base, recommended, ok, fidelityThreshold); err != nil {
		return err
	}
	if ok {
		fmt.Printf("recommendation: %dp is the lowest resolution that preserves code fidelity (>=%.0f%% overlap vs 1080p)\n", recommended, fidelityThreshold*100)
	} else {
		fmt.Println("recommendation: no resolution met the fidelity threshold — keep 1080p")
	}
	return nil
}

// inspectHelper returns the path to the vision-inspect helper (inspect.mjs),
// overridable via VISION_INSPECT.
func inspectHelper() (string, error) {
	if p := os.Getenv("VISION_INSPECT"); p != "" {
		//nolint:gosec // p is an explicit config path (env override), not user-tainted input; Stat only checks existence
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("VISION_INSPECT %q not found: %w", p, err)
		}
		return p, nil
	}
	def := filepath.Join(os.Getenv("USERPROFILE"), "Documents", "projects", "vision-inspect", "inspect.mjs")
	//nolint:gosec // def is a documented default location, not user-tainted input; Stat only checks existence
	if _, err := os.Stat(def); err != nil {
		return "", fmt.Errorf("vision-inspect helper not found at %s — set VISION_INSPECT to its path: %w", def, err)
	}
	return def, nil
}

// runVision runs the helper in read mode (full-res + thinking) over dir and
// returns its stdout transcript.
func runVision(helper, dir string) (string, error) {
	//nolint:gosec // "node" is a fixed binary; helper/dir are file paths passed as argv (never a shell), question is a constant
	cmd := exec.Command("node", helper, dir, "-q", visionQuestion, "--think", "--detail", "high", "--dedupe", "--max", "12")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("helper: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// dirHasFrames reports whether dir contains at least one frame_*.jpg.
func dirHasFrames(dir string) bool {
	matches, err := filepath.Glob(filepath.Join(dir, "frame_*.jpg"))
	return err == nil && len(matches) > 0
}

// baselineText returns the 1080p extraction (the reference).
func baselineText(extracts []resExtract) string {
	for _, e := range extracts {
		if e.height == benchMasterHeight {
			return e.text
		}
	}
	return ""
}

// recommendLowest returns the lowest-resolution entry whose extracted code is at
// least threshold-similar to the 1080p baseline, plus whether one qualified.
func recommendLowest(extracts []resExtract, threshold float64) (int, bool) {
	base := baselineText(extracts)
	if base == "" {
		return 0, false
	}
	sorted := make([]resExtract, len(extracts))
	copy(sorted, extracts)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].height < sorted[j].height })
	for _, e := range sorted {
		if e.height == benchMasterHeight {
			continue // the baseline is the reference, not a candidate
		}
		if similarity(e.text, base) >= threshold {
			return e.height, true
		}
	}
	return 0, false
}

// similarity is a token-set Jaccard similarity (0..1) between two transcriptions.
func similarity(a, b string) float64 {
	ta := tokenSet(a)
	tb := tokenSet(b)
	if len(ta) == 0 && len(tb) == 0 {
		return 1
	}
	inter := 0
	for t := range ta {
		if tb[t] {
			inter++
		}
	}
	union := len(ta) + len(tb) - inter
	if union == 0 {
		return 1
	}
	return float64(inter) / float64(union)
}

// tokenSet lowercases a text and splits it into an alphanumeric token set.
func tokenSet(s string) map[string]bool {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	out := make(map[string]bool, len(fields))
	for _, f := range fields {
		out[f] = true
	}
	return out
}

// writeVerdict writes bench/<id>/verdict.md with the per-resolution table and
// recommendation.
func writeVerdict(dir string, extracts []resExtract, base string, recommended int, ok bool, threshold float64) error {
	var sb strings.Builder
	sb.WriteString("# Bench readability verdict\n\n")
	fmt.Fprintf(&sb, "Fidelity is token overlap vs the 1080p baseline; a resolution counts as readable at >= %.0f%%.\n\n", threshold*100)
	sb.WriteString("| resolution | readable |\n|---|---|\n")
	for _, e := range extracts {
		sim := similarity(e.text, base)
		mark := "no"
		if sim >= threshold {
			mark = "yes"
		}
		fmt.Fprintf(&sb, "| %dp | %s (%.0f%%) |\n", e.height, mark, sim*100)
	}
	sb.WriteString("\n")
	if ok {
		fmt.Fprintf(&sb, "**Recommendation:** use **%dp** — the lowest resolution that preserves code fidelity.\n", recommended)
	} else {
		sb.WriteString("**Recommendation:** no resolution below 1080p preserved code fidelity — keep 1080p.\n")
	}
	return os.WriteFile(filepath.Join(dir, "verdict.md"), []byte(sb.String()), 0o644)
}
