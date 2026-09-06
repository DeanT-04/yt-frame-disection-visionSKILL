package extract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DeanT-04/yt-code-vision-skill/internal/paths"
)

func writeTranscript(t *testing.T, transDir string, st State, code string) {
	t.Helper()
	body := fmt.Sprintf("=== state %d — frame_%06d.jpg @ 00:00:00 ===\ndesc...\n\n## Answer\n\n%s\n", st.Index, st.FrameNum, code)
	if err := os.WriteFile(transcriptPath(transDir, st), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTailOverlap(t *testing.T) {
	cases := []struct {
		a, b []string
		want int
	}{
		{[]string{"1", "2", "3", "4"}, []string{"3", "4", "5", "6"}, 2},
		{[]string{"1", "2", "3", "4"}, []string{"4", "5", "6"}, 1},
		{[]string{"a", "b"}, []string{"a", "b", "c"}, 2}, // full tail reuse + extend
		{[]string{"a", "b"}, []string{"x", "y"}, 0},
	}
	for _, c := range cases {
		if got := tailOverlap(c.a, c.b); got != c.want {
			t.Errorf("tailOverlap(%v,%v)=%d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestParseTranscript(t *testing.T) {
	withAnswer := "=== state 0 — frame_000001.jpg @ 00:03:38 ===\nlines\n\n## Answer\n\nint x = 1;\ndouble y;\n"
	got, err := ParseTranscript(writeTmp(t, withAnswer))
	if err != nil {
		t.Fatal(err)
	}
	if got != "int x = 1;\ndouble y;" {
		t.Errorf("parse with answer = %q", got)
	}

	noAnswer := "=== state 3 — frame_000004.jpg @ 00:03:41 ===\nint z;\n"
	got, err = ParseTranscript(writeTmp(t, noAnswer))
	if err != nil {
		t.Fatal(err)
	}
	if got != "int z;" {
		t.Errorf("parse without answer = %q", got)
	}
}

func writeTmp(t *testing.T, s string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "t.txt")
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReconstructMergesWindows(t *testing.T) {
	// chdir sandbox so paths.CodeDir stays under the temp dir (never real outputs).
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	sandbox := t.TempDir()
	if err := os.Chdir(sandbox); err != nil {
		t.Fatal(err)
	}
	id := "mtWN6oPIi1Y"
	transDir := paths.CodeTranscriptsDir(id)
	if err := os.MkdirAll(transDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Three ordered windows of one file (scrolling down with overlap).
	writeTranscript(t, transDir, State{Index: 0, FrameNum: 1}, "input();\nsetup();\n")
	writeTranscript(t, transDir, State{Index: 1, FrameNum: 4}, "setup();\nloop();\n")
	writeTranscript(t, transDir, State{Index: 2, FrameNum: 7}, "loop();\nteardown();\n")

	outPath, lines, err := Reconstruct(id, []State{
		{Index: 0, FrameNum: 1}, {Index: 1, FrameNum: 4}, {Index: 2, FrameNum: 7},
	})
	if err != nil {
		t.Fatalf("Reconstruct: %v", err)
	}
	want := "input();\nsetup();\nloop();\nteardown();"
	if strings.Join(lines, "\n") != want {
		t.Errorf("reconstructed =\n%s\nwant:\n%s", strings.Join(lines, "\n"), want)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("ea.mq5 missing: %v", err)
	}
	// Manifest is written by the orchestrator (mirrors Run): call it, then
	// confirm review.md (written by Reconstruct) and manifest.md both exist.
	if err := WriteManifest(id, []State{{Index: 0}, {Index: 1}, {Index: 2}}); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"manifest.md", "review.md"} {
		if _, err := os.Stat(filepath.Join(paths.CodeDir(id), f)); err != nil {
			t.Errorf("%s missing: %v", f, err)
		}
	}
}

func TestReconstructSkipsNoCodeAndDuplicates(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	sandbox := t.TempDir()
	if err := os.Chdir(sandbox); err != nil {
		t.Fatal(err)
	}
	id := "vidid"
	transDir := paths.CodeTranscriptsDir(id)
	if err := os.MkdirAll(transDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTranscript(t, transDir, State{Index: 0, FrameNum: 1}, "int a;\n")
	writeTranscript(t, transDir, State{Index: 1, FrameNum: 2}, NoCodeMarker)
	writeTranscript(t, transDir, State{Index: 2, FrameNum: 3}, "int a;\n") // duplicate window
	writeTranscript(t, transDir, State{Index: 3, FrameNum: 4}, "int b;\n")

	_, lines, err := Reconstruct(id, []State{{Index: 0}, {Index: 1}, {Index: 2}, {Index: 3}})
	if err != nil {
		t.Fatalf("Reconstruct: %v", err)
	}
	got := strings.Join(lines, "\n")
	if strings.Contains(got, NoCodeMarker) {
		t.Errorf("no-code marker leaked into output: %q", got)
	}
	if want := "int a;\nint b;"; got != want {
		t.Errorf("reconstructed = %q, want %q", got, want)
	}
}

func TestCodeFromFence(t *testing.T) {
	raw := "IMAGE 1 — one.jpg\n\nThis is a screenshot of MetaEditor.\n```cpp\nvoid OnTick()\n  {\n  }\n```\n"
	lines, ok := codeFrom(raw)
	if !ok {
		t.Fatal("expected code from fenced answer")
	}
	if got := strings.Join(lines, "\n"); got != "void OnTick()\n  {\n  }" {
		t.Errorf("fence code = %q", got)
	}
}

func TestCodeFromTaggedAndNoCode(t *testing.T) {
	if lines, ok := codeFrom("desc\n<CODE>\nint a;\n</CODE>\n"); !ok || strings.Join(lines, "\n") != "int a;" {
		t.Errorf("tagged parse = %v %v", lines, ok)
	}
	if _, ok := codeFrom("The frame shows a report table.\n<NO CODE>"); ok {
		t.Error("no-code answer should yield no code")
	}
	if _, ok := codeFrom(""); ok {
		t.Error("empty answer should yield no code")
	}
}

func TestCheckStructure(t *testing.T) {
	if issues := CheckStructure("void f()\n{\n  int a = 1;\n}\n"); len(issues) != 0 {
		t.Errorf("clean MQL5 flagged: %v", issues)
	}
	if issues := CheckStructure("void f()\n{\n  int a;\n"); len(issues) == 0 {
		t.Error("unbalanced brace not flagged")
	}
	if issues := CheckStructure("input int … x;"); len(issues) == 0 {
		t.Error("uncertainty character not flagged")
	}
	// Braces inside strings must not count.
	if issues := CheckStructure("string s = \"{not a brace}\";"); len(issues) != 0 {
		t.Errorf("string contents flagged as braces: %v", issues)
	}
}

func TestHeaderLineRange(t *testing.T) {
	cases := []struct {
		header    string
		firstLast [2]int
	}{
		{"=== state 5 — frame_000041.jpg @ 00:12:44 — lines 41-68 ===", [2]int{41, 68}},
		{"=== state 5 — frame_000041.jpg @ 00:12:44 ===", [2]int{0, 0}},
		{"=== state 1 — frame_000002.jpg @ 00:00:01 — lines 1-24 ===", [2]int{1, 24}},
	}
	for _, c := range cases {
		p := writeTmp(t, c.header+"\n```mql5\nint a;\n```\n")
		first, last, err := headerLineRange(p)
		if err != nil {
			t.Fatalf("headerLineRange(%q): %v", c.header, err)
		}
		if first != c.firstLast[0] || last != c.firstLast[1] {
			t.Errorf("headerLineRange(%q) = %d,%d want %d,%d", c.header, first, last, c.firstLast[0], c.firstLast[1])
		}
	}
}

func TestMergeAnchoredContinuation(t *testing.T) {
	file := []string{"int a;", "int b;"}
	// Window claims gutter lines 2-4: line 2 must match "int b;", 3-4 append.
	merged, cs, _ := mergeAnchored(State{Index: 1, FrameNum: 5}, []string{"int b;", "int c;", "int d;"}, 2, file)
	if len(cs) != 0 {
		t.Fatalf("clean continuation produced conflicts: %+v", cs)
	}
	if got := strings.Join(merged, "\n"); got != "int a;\nint b;\nint c;\nint d;" {
		t.Errorf("merged = %q", got)
	}
}

func TestMergeAnchoredConflictLaterWins(t *testing.T) {
	file := []string{"int a;", "int b;"}
	// Window claims lines 1-3 but disagrees about line 2: later reading wins
	// (reformat) and the disagreement is recorded.
	merged, cs, _ := mergeAnchored(State{Index: 2, FrameNum: 9}, []string{"int a;", "int B;", "int c;"}, 1, file)
	if len(cs) != 1 || cs[0].Line != 2 || cs[0].Existing != "int b;" || cs[0].New != "int B;" {
		t.Fatalf("conflicts = %+v, want one on line 2", cs)
	}
	if got := strings.Join(merged, "\n"); got != "int a;\nint B;\nint c;" {
		t.Errorf("merged = %q", got)
	}
}
