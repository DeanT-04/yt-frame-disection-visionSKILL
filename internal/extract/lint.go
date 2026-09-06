package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeanT-04/yt-code-vision-skill/internal/paths"
)

// CheckStructure runs cheap deterministic structural checks over the merged
// code and returns human-readable issues. It intentionally does NOT try to be
// a full parser — its job is to catch the failure modes of transcription
// (dropped/misplaced braces, split lines) so a human reviews flagged spots.
func CheckStructure(code string) []string {
	var issues []string
	for _, line := range strings.Split(code, "\n") {
		// Replacement / ellipsis characters the model may emit when unsure.
		if strings.ContainsAny(line, "…\uFFFD\u2028") {
			issues = append(issues, fmt.Sprintf("uncertainty character on line: %q", line))
		}
		if strings.Contains(line, "<NO CODE>") || strings.Contains(line, "## Answer") {
			issues = append(issues, fmt.Sprintf("pipeline noise leaked into output: %q", line))
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.Contains(line, "\t") {
			issues = append(issues, fmt.Sprintf("tab character (MQL5 uses spaces): %q", line))
		}
	}
	// Balance of structural tokens outside line comments (rough approximation).
	code = stripLineComments(code)
	code = stripStrings(code)
	for _, pair := range []struct{ open, close string }{
		{"{", "}"}, {"(", ")"}, {"[", "]"},
	} {
		o := strings.Count(code, pair.open)
		c := strings.Count(code, pair.close)
		if o != c {
			issues = append(issues, fmt.Sprintf("unbalanced %q: %d open vs %d close", pair.open, o, c))
		}
	}
	return issues
}

// stripLineComments removes // ... to end-of-line (MQL5 comments).
func stripLineComments(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			b.WriteString(line[:i])
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// stripStrings blanks double-quoted string literals so quotes/braces inside
// string constants don't skew balance checks.
func stripStrings(s string) string {
	var b strings.Builder
	inStr := false
	esc := false
	for _, r := range s {
		if inStr {
			if esc {
				esc = false
			} else if r == '\\' {
				esc = true
			} else if r == '"' {
				inStr = false
			}
			b.WriteRune(' ')
			continue
		}
		if r == '"' {
			inStr = true
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// WriteManifest writes outputs/<id>/code/manifest.md linking every state to its
// source frame and timestamp so any line can be traced back to the exact frame.
func WriteManifest(id string, states []State) error {
	var b strings.Builder
	b.WriteString("# Code-extraction manifest — " + id + "\n\n")
	b.WriteString("| state | frame | seconds into frameset | transcript |\n|---|---|---|---|\n")
	for _, st := range states {
		fmt.Fprintf(&b, "| %d | frame_%06d.jpg | %.0f | `transcripts/state_%06d.txt` |\n",
			st.Index, st.FrameNum, st.Sec, st.Index)
	}
	b.WriteString("\nFrames begin at the coding-chapter start offset (see chapters.yaml).\n")
	return os.WriteFile(filepath.Join(paths.CodeDir(id), "manifest.md"), []byte(b.String()), 0o644)
}
