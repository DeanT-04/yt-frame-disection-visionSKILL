package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeanT-04/yt-code-vision-skill/internal/paths"
)

// NoCodeMarker is the exact line a transcript contains when the frame had no
// readable code (see the MQL5Prompt instruction).
const NoCodeMarker = "<NO CODE>"

// answerSeparator is the helper's stdout marker between the per-image
// descriptions and the model's answer to our question.
const answerSeparator = "## Answer"

// Transcript holds one parsed state transcript: the verbatim code lines the
// model read off that frame (empty when the frame had no code).
type Transcript struct {
	State   State
	Code    []string
	HasCode bool
	Raw     string
}

// ParseTranscript reads a transcript file (header + helper stdout) and returns
// the model's answer text (everything after "## Answer", else the body).
func ParseTranscript(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := string(b)
	if i := strings.LastIndex(s, answerSeparator); i >= 0 {
		s = s[i+len(answerSeparator):]
	}
	// Drop the leading "=== state N — frame_... ===" header we prepend.
	if j := strings.Index(s, "==="); j >= 0 {
		s = s[j:]
		if k := strings.Index(s, "===\n"); k >= 0 {
			s = s[k+4:]
		}
	}
	return strings.TrimSpace(s), nil
}

// codeFrom extracts the actual code lines from a transcript's answer. The model
// fences code in ```...``` blocks (new prompt) or <CODE>...</CODE> (old), and
// may prepend a description — this strips all of that. Returns the code lines
// and whether any code was found (false = empty or explicitly no-code).
func codeFrom(raw string) ([]string, bool) {
	t := strings.TrimSpace(raw)
	if t == "" {
		return nil, false
	}
	if isNoCode(t) {
		return nil, false
	}
	if lines, ok := fencedBlock(t); ok {
		return lines, true
	}
	if lines, ok := taggedBlock(t, "<CODE>", "</CODE>"); ok {
		return lines, true
	}
	// Fallback (old transcripts): drop description-preamble lines and hope the
	// remainder is code. Anything left that still reads as prose is flagged by
	// the structural checks rather than silently kept.
	lines := stripProse(strings.Split(t, "\n"))
	lines = trimBlank(lines)
	return lines, len(lines) > 0
}

// fencedBlock extracts the inside of ```...``` code fences, dropping any
// language tag on the opening line. Multiple fences are concatenated.
func fencedBlock(s string) ([]string, bool) {
	const fence = "```"
	var parts []string
	rest := s
	for {
		i := strings.Index(rest, fence)
		if i < 0 {
			break
		}
		rest = rest[i+len(fence):]
		j := strings.Index(rest, fence)
		if j < 0 {
			break
		}
		inner := rest[:j]
		if nl := strings.IndexByte(inner, '\n'); nl >= 0 {
			inner = inner[nl+1:]
		}
		parts = append(parts, inner)
		rest = rest[j+len(fence):]
	}
	if len(parts) == 0 {
		return nil, false
	}
	return trimBlank(strings.Split(strings.Join(parts, "\n"), "\n")), true
}

func taggedBlock(s, open, close string) ([]string, bool) {
	i := strings.Index(s, open)
	if i < 0 {
		return nil, false
	}
	s = s[i+len(open):]
	j := strings.Index(s, close)
	if j < 0 {
		return nil, false
	}
	return trimBlank(strings.Split(s[:j], "\n")), true
}

// stripProse drops lines that read as the model's image description rather than
// code. Description lines start with these phrases; code lines never do.
func stripProse(lines []string) []string {
	var out []string
	for _, ln := range lines {
		low := strings.ToLower(strings.TrimSpace(ln))
		skip := false
		for _, p := range []string{
			"image ", "this is", "the image", "the frame", "this frame",
			"here is", "the screenshot", "a screenshot", "the video", "in the",
			"on the", "it shows", "it is", "the code", "the window", "visible text",
		} {
			if strings.HasPrefix(low, p) {
				skip = true
				break
			}
		}
		if !skip {
			out = append(out, ln)
		}
	}
	return out
}

// Reconstruct merges the ordered transcripts into the final code file. States
// are windows over a file that the creator builds top-down; consecutive states
// overlap, so the merge aligns each new window onto the tail of what it already
// has (longest exact-line overlap) and appends the newer lines. States that
// show no code are skipped. Later, identical windows are deduped. Returns the
// output path and the merged lines.
func Reconstruct(id string, states []State) (string, []string, error) {
	transDir := paths.CodeTranscriptsDir(id)
	var file []string
	prevKey := ""
	var notes []string

	for _, st := range states {
		path := transcriptPath(transDir, st)
		raw, err := ParseTranscript(path)
		if err != nil {
			return "", nil, fmt.Errorf("read transcript state %d: %w", st.Index, err)
		}
		lines, hasCode := codeFrom(raw)
		if !hasCode {
			switch {
			case isNoCode(raw):
				notes = append(notes, fmt.Sprintf("state %d (frame_%06d): no code on screen, skipped", st.Index, st.FrameNum))
			case raw == "":
				notes = append(notes, fmt.Sprintf("state %d (frame_%06d): empty transcript, skipped", st.Index, st.FrameNum))
			default:
				notes = append(notes, fmt.Sprintf("state %d (frame_%06d): no code fence found, skipped (REVIEW — consider re-reading this state)", st.Index, st.FrameNum))
			}
			continue
		}
		key := strings.Join(lines, "\n")
		if key == prevKey {
			continue // unchanged window (e.g. caret-only state)
		}
		prevKey = key

		if len(file) == 0 {
			file = lines
			continue
		}
		k := tailOverlap(file, lines)
		if k == 0 {
			notes = append(notes, fmt.Sprintf("state %d (frame_%06d): no tail overlap with prior window — appended as-is (REVIEW)", st.Index, st.FrameNum))
		}
		file = append(file, lines[k:]...)
	}

	if len(file) == 0 {
		return "", nil, fmt.Errorf("reconstruction produced no lines (no code transcripts found)")
	}
	outPath := filepath.Join(paths.CodeDir(id), "reconstructed.mq5")
	body := strings.Join(file, "\n") + "\n"
	if err := os.WriteFile(outPath, []byte(body), 0o644); err != nil {
		return "", nil, err
	}
	if err := writeNotes(id, notes); err != nil {
		return "", nil, err
	}
	return outPath, file, nil
}

// isNoCode reports whether a transcript's answer says the frame had no readable
// code. The model sometimes wraps the marker in a sentence, so a containment
// check on any line is used rather than exact equality.
func isNoCode(raw string) bool {
	for _, line := range strings.Split(raw, "\n") {
		if strings.Contains(line, NoCodeMarker) {
			return true
		}
	}
	return false
}

// tailOverlap returns the largest k such that the head of b matches the tail of
// a, where a is the accumulated file and b the new window. Zero means no
// alignment (the creator jumped windows without shared context).
func tailOverlap(a, b []string) int {
	best := 0
	// The window head can match anywhere at the END of the file: try every
	// suffix start of a that lets b's prefix align.
	for start := len(a) - 1; start >= 0; start-- {
		k := 0
		for k < len(b) && start+k < len(a) && a[start+k] == b[k] {
			k++
		}
		// The alignment is meaningful when b's matched prefix reaches the very
		// end of a (the new window continues right where the file left off).
		if start+k == len(a) && k > best {
			best = k
		}
	}
	return best
}

func trimBlank(lines []string) []string {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

func writeNotes(id string, notes []string) error {
	if len(notes) == 0 {
		notes = []string{"no skipped or ambiguous states — every transcript merged cleanly"}
	}
	return os.WriteFile(filepath.Join(paths.CodeDir(id), "review.md"),
		[]byte("# Extraction review — "+id+"\n\n"+strings.Join(notes, "\n")+"\n"), 0o644)
}
