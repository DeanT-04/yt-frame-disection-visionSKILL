package extract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/DeanT-04/yt-code-vision-skill/internal/chapter"
	"github.com/DeanT-04/yt-code-vision-skill/internal/crop"
	"github.com/DeanT-04/yt-code-vision-skill/internal/paths"
)

// TranscriptionContract is the instruction set for the agent that reads the
// staged state images (code/states/state_XXXXXX.jpg) and writes the
// transcripts. It is written to code/README.md on every staging run so the
// contract travels with the data.
const TranscriptionContract = `# Code-state transcription contract

Read each staged image outputs/<id>/code/states/state_XXXXXX.jpg (the cropped
IDE code pane) with your vision, in temporal order, and write the transcript
outputs/<id>/code/transcripts/state_XXXXXX.txt with EXACTLY this format:

=== state N — frame_FFFFFF.jpg @ HH:MM:SS — lines A-B ===
` + "```mql5\n" + `<verbatim code, one source line per output line, indentation preserved>
` + "```\n" + `
Rules:
- N, FFFFFF, HH:MM:SS are copied from worklist.json for that state.
- A-B is the visible line-number range in the editor gutter (first-last).
  If no gutter line numbers are visible, omit the "— lines A-B" part.
- Transcribe the code exactly as pixels show it: every character, case, space,
  semicolon, brace, quote and comment. Never paraphrase, complete, rename, or
  "fix" anything.
- If the pane shows no code (chart, terminal, dialog, browser), write the
  header line followed by exactly: <NO CODE>
- One file per state. Existing transcripts are never rewritten (except when a
  state is flagged in conflicts.json for re-read).
`

// workEntry is one row of worklist.json: a state waiting to be transcribed.
type workEntry struct {
	Index      int     `json:"index"`
	FrameNum   int     `json:"frame"`
	FrameFile  string  `json:"frame_file"`
	Sec        float64 `json:"sec"`
	Hms        string  `json:"hms"`
	Image      string  `json:"image"`
	Transcript string  `json:"transcript"`
}

// worklist is the staging manifest handed to the transcribing agent.
type worklist struct {
	Total   int         `json:"total"`
	Done    int         `json:"done"`
	Pending int         `json:"pending"`
	Entries []workEntry `json:"entries"`
}

// Run drives the code-extraction pipeline for one video: it detects the ordered
// code states from the frames, stages a cropped pane image + worklist for each
// (the transcription itself is done by the agent reading the staged images —
// see TranscriptionContract), and finally reconstructs the merged code file
// from whatever transcripts exist. Transcripts are written to
// outputs/<id>/code/transcripts/state_%06d.txt and are resumable (existing ones
// are skipped). maxStates caps how many states are worked (0 = all);
// fromState skips lead-in states.
func Run(id string, dryRun bool, maxStates, fromState int) error {
	cr, err := crop.Load(paths.IDDir(id))
	if err != nil {
		return fmt.Errorf("need crop.json first (run --find-crop): %w", err)
	}
	rect := Rect{X: cr.X, Y: cr.Y, W: cr.Width, H: cr.Height}

	codeDir := paths.CodeDir(id)
	transDir := paths.CodeTranscriptsDir(id)
	statesDir := paths.CodeStatesDir(id)
	for _, d := range []string{codeDir, transDir, statesDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}

	// Ordered code states: compare only the pane region so static chrome (and
	// the chart/terminal demo tail) never creates spurious states.
	all, err := DetectStates(paths.FramesDir(id), Options{Crop: &rect})
	if err != nil {
		return err
	}
	if err := writeStatesJSON(filepath.Join(codeDir, "states.json"), all); err != nil {
		return err
	}

	states := all
	if fromState > 0 {
		if fromState >= len(states) {
			return fmt.Errorf("--from-state %d >= total states %d", fromState, len(states))
		}
		states = states[fromState:]
	}
	if maxStates > 0 && len(states) > maxStates {
		states = states[:maxStates]
	}

	fmt.Printf("video %s: %d distinct code states (%d frames scanned, pane %dx%d at %d,%d)\n",
		id, len(all), stateFrameSpan(all), rect.W, rect.H, rect.X, rect.Y)
	if dryRun {
		fmt.Println("dry-run: states detected only. Re-run without --dry-run to stage images for transcription.")
		return nil
	}

	// Stage: one pane-cropped image per state (idempotent overwrite is fine —
	// the source frame never changes), plus the worklist of pending transcripts.
	staged, err := stageStates(id, states, statesDir, rect)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(codeDir, "README.md"), []byte(TranscriptionContract), 0o644); err != nil {
		return err
	}
	fmt.Printf("staged %d state images into %s\n", staged, statesDir)

	// Reconstruct from whatever transcripts already exist (none on first run).
	if transcribed := countTranscripts(transDir); transcribed > 0 {
		outPath, fileLines, err := Reconstruct(id, states)
		if err != nil {
			return err
		}
		if err := WriteManifest(id, states); err != nil {
			return err
		}
		issues := CheckStructure(strings.Join(fileLines, "\n"))
		fmt.Printf("reconstructed %d lines -> %s\n", len(fileLines), outPath)
		if len(issues) > 0 {
			fmt.Printf("structural review flagged %d issue(s) (see outputs/%s/code/review.md)\n", len(issues), id)
		} else {
			fmt.Println("structural checks: clean")
		}
	} else {
		fmt.Printf("no transcripts yet — read the staged images per %s and re-run\n", filepath.Join(codeDir, "README.md"))
	}
	return nil
}

// stageStates writes the pane-cropped image for every state and worklist.json
// listing the states still missing a transcript. Returns how many images were
// written this run.
func stageStates(id string, states []State, statesDir string, rect Rect) (int, error) {
	wl := worklist{Total: len(states)}
	for _, st := range states {
		img := filepath.Join(statesDir, fmt.Sprintf("state_%06d.jpg", st.Index))
		frame := filepath.Join(paths.FramesDir(id), fmt.Sprintf("frame_%06d.jpg", st.FrameNum))
		if err := crop.Single(frame, img, crop.CropRect{X: rect.X, Y: rect.Y, Width: rect.W, Height: rect.H}); err != nil {
			return 0, fmt.Errorf("crop state %d frame %s: %w", st.Index, frame, err)
		}
		out := transcriptPath(paths.CodeTranscriptsDir(id), st)
		if fileExists(out) {
			wl.Done++
			continue
		}
		wl.Pending++
		wl.Entries = append(wl.Entries, workEntry{
			Index:      st.Index,
			FrameNum:   st.FrameNum,
			FrameFile:  fmt.Sprintf("frame_%06d.jpg", st.FrameNum),
			Sec:        st.Sec,
			Hms:        chapter.Hms(st.Sec),
			Image:      img,
			Transcript: out,
		})
	}
	b, err := json.MarshalIndent(wl, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.WriteFile(filepath.Join(paths.CodeDir(id), "worklist.json"), b, 0o644); err != nil {
		return 0, err
	}
	return len(states), nil
}

// countTranscripts counts transcript files in transDir.
func countTranscripts(transDir string) int {
	matches, _ := filepath.Glob(filepath.Join(transDir, "state_*.txt"))
	return len(matches)
}

func writeStatesJSON(path string, states []State) error {
	b, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// transcriptPath returns outputs/<id>/code/transcripts/state_%06d.txt for st.
func transcriptPath(transDir string, st State) string {
	return filepath.Join(transDir, fmt.Sprintf("state_%06d.txt", st.Index))
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// stateFrameSpan returns the number of frames between the first and last state.
func stateFrameSpan(states []State) int {
	if len(states) < 2 {
		return 1
	}
	return states[len(states)-1].FrameNum - states[0].FrameNum + 1
}
