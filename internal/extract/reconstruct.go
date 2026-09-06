package extract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// Conflict records one line where two states disagree about the same source
// line. Conflicting windows are never merged — they are written to
// conflicts.json so the state gets re-read instead of silently corrupting the
// output.
type Conflict struct {
	State    int    `json:"state"`
	Frame    int    `json:"frame"`
	Line     int    `json:"line"`
	Existing string `json:"existing"`
	New      string `json:"new"`
}

// Reconstruct merges the ordered transcripts into the final code file. When a
// transcript header records the visible gutter line range ("lines A-B"), the
// merge is line-anchored: line A of the window lands on line A of the
// accumulated file, disagreements are applied later-wins and recorded in
// conflicts.json, and lines past the end are appended. Transcripts without a
// line range fall back to longest tail-overlap alignment. States that show no
// code are skipped. Returns the output path and the merged lines.
func Reconstruct(id string, states []State) (string, []string, error) {
	transDir := paths.CodeTranscriptsDir(id)
	var file []string
	prevKey := ""
	var notes []string
	var conflicts []Conflict

	for _, st := range states {
		path := transcriptPath(transDir, st)
		raw, err := ParseTranscript(path)
		if os.IsNotExist(err) {
			continue // not transcribed yet (coarse pass / resumable runs)
		}
		if err != nil {
			return "", nil, fmt.Errorf("read transcript state %d: %w", st.Index, err)
		}
		first, _, err := headerLineRange(path)
		if err != nil {
			return "", nil, fmt.Errorf("read header state %d: %w", st.Index, err)
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

		if first > 0 {
			merged, cs, nNotes := mergeAnchored(st, lines, first, file)
			file = merged
			conflicts = append(conflicts, cs...)
			notes = append(notes, nNotes...)
			continue
		}

		// Fallback: no gutter range — align by longest tail overlap.
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
	outPath := filepath.Join(paths.CodeDir(id), "ea.mq5")
	body := strings.Join(file, "\n") + "\n"
	if err := os.WriteFile(outPath, []byte(body), 0o644); err != nil {
		return "", nil, err
	}
	if err := writeConflicts(id, conflicts); err != nil {
		return "", nil, err
	}
	if err := writeNotes(id, notes); err != nil {
		return "", nil, err
	}
	return outPath, file, nil
}

// mergeAnchored places window lines at gutter line first (line 1 = index 0) in
// file. Lines inside the existing file are verified; a disagreement is applied
// (later-wins — the video's later state is closer to the final code, e.g. after
// a reformat) and recorded as a conflict for the audit trail. Lines past the
// end are appended, filling any gap with blanks.
func mergeAnchored(st State, lines []string, first int, file []string) ([]string, []Conflict, []string) {
	var cs []Conflict
	var notes []string

	if len(file) == 0 {
		if first > 1 {
			notes = append(notes, fmt.Sprintf("state %d (frame_%06d): first visible line is %d — %d leading lines unknown (REVIEW)", st.Index, st.FrameNum, first, first-1))
			file = make([]string, first-1)
		}
		return append(file, lines...), nil, notes
	}

	for i, ln := range lines {
		pos := first + i
		switch {
		case pos <= len(file):
			if file[pos-1] != ln {
				cs = append(cs, Conflict{State: st.Index, Frame: st.FrameNum, Line: pos, Existing: file[pos-1], New: ln})
				file[pos-1] = ln
			}
		default: // pos beyond the file end: fill any gap with blanks, then append
			if pos > len(file)+1 {
				notes = append(notes, fmt.Sprintf("state %d (frame_%06d): gap before line %d filled with blanks (REVIEW)", st.Index, st.FrameNum, pos))
				for len(file) < pos-1 {
					file = append(file, "")
				}
			}
			file = append(file, ln)
		}
	}
	return file, cs, notes
}

// headerLineRange extracts the "lines A-B" range from a transcript's header
// line ("=== state N — frame_F.jpg @ T — lines A-B ==="). Returns 0,0 when the
// header records no range.
func headerLineRange(path string) (int, int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}
	header := string(b)
	if i := strings.IndexByte(header, '\n'); i >= 0 {
		header = header[:i]
	}
	j := strings.Index(header, "lines ")
	if j < 0 {
		return 0, 0, nil
	}
	rest := header[j+len("lines "):]
	if end := strings.IndexAny(rest, " \t="); end >= 0 {
		rest = rest[:end]
	}
	k := strings.IndexByte(rest, '-')
	if k <= 0 {
		return 0, 0, nil
	}
	first, err1 := strconv.Atoi(rest[:k])
	last, err2 := strconv.Atoi(rest[k+1:])
	if err1 != nil || err2 != nil || first < 1 || last < first {
		return 0, 0, nil
	}
	return first, last, nil
}

func writeConflicts(id string, conflicts []Conflict) error {
	if conflicts == nil {
		conflicts = []Conflict{}
	}
	b, err := json.MarshalIndent(conflicts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(paths.CodeDir(id), "conflicts.json"), append(b, '\n'), 0o644)
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
