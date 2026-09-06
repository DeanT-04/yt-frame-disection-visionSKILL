package extract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

	"github.com/DeanT-04/yt-code-vision-skill/internal/chapter"
	"github.com/DeanT-04/yt-code-vision-skill/internal/crop"
	"github.com/DeanT-04/yt-code-vision-skill/internal/paths"
)

// MQL5Prompt is the character-exact transcription instruction used for every
// state read. MQL5 is case- and punctuation-sensitive, so the model is told to
// reproduce the pixels, never to "fix" or paraphrase.
const MQL5Prompt = `This frame is from a MetaTrader 5 (MQL5) coding tutorial. Transcribe the code exactly as it appears in the editor: every character, letter case, space, tab and indentation, semicolon, brace, parenthesis, quote and comment must match the image. Do NOT describe the image, do NOT comment on the code, do NOT paraphrase, translate, summarize, complete, rename, or "fix" anything. Reply with ONLY the code, inside a single fenced code block tagged mql5. If the editor area shows no code (only a chart, report, terminal, browser, or settings dialog), reply with exactly: <NO CODE>`

// estTokensPerState is a rough per-read-mode-call input-token estimate used only
// for the --dry-run cost projection (image + prompt). The proof run measures
// the real number.
const estTokensPerState = 2400

// Run drives the code-extraction pipeline for one video: it detects the ordered
// code states from the frames, then (unless dryRun) transcribes each state
// verbatim through the vision-inspect helper. Transcripts are written to
// outputs/<id>/code/transcripts/state_%06d.txt and are resumable (existing ones
// are skipped). maxStates caps how many states are processed (0 = all);
// fromState skips lead-in states; workers runs that many reads concurrently.
func Run(id string, dryRun bool, maxStates, fromState, workers int) error {
	cr, err := crop.Load(paths.IDDir(id))
	if err != nil {
		return fmt.Errorf("need crop.json first (run --find-crop): %w", err)
	}
	rect := Rect{X: cr.X, Y: cr.Y, W: cr.Width, H: cr.Height}

	codeDir := paths.CodeDir(id)
	transDir := paths.CodeTranscriptsDir(id)
	for _, d := range []string{codeDir, transDir} {
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

	// Cap / offset the states we actually transcribe this run.
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
	fmt.Printf("est: %d read-mode vision calls, ~%.0fk input tokens — real cost measured on the run\n",
		len(states), float64(len(states)*estTokensPerState)/1000)
	if dryRun {
		fmt.Println("dry-run: no API calls made. Re-run without --dry-run to transcribe (resumable).")
		return nil
	}

	helper, err := VisionHelper()
	if err != nil {
		return err
	}
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("node is required to run the vision-inspect helper: %w", err)
	}

	// Transcribe pending states concurrently. The helper analyzes a whole
	// folder, so each worker stages its state's cropped frame as one.jpg inside
	// its own scratch subfolder (never shared -> no clobbering).
	if workers < 1 {
		workers = 1
	}
	var pending []State
	for _, st := range states {
		if !fileExists(transcriptPath(transDir, st)) {
			pending = append(pending, st)
		}
	}
	fmt.Printf("transcribing %d pending states with %d worker(s)...\n", len(pending), workers)
	var mu sync.Mutex
	newOnes := 0
	jobs := make(chan State)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		scratch := filepath.Join(codeDir, ".read", fmt.Sprintf("w%d", w))
		if err := os.MkdirAll(scratch, 0o755); err != nil {
			return err
		}
		wg.Add(1)
		go func(scratch string) {
			defer wg.Done()
			for st := range jobs {
				out := transcriptPath(transDir, st)
				if err := transcribeOne(helper, scratch, id, st, rect, out); err != nil {
					fmt.Fprintf(os.Stderr, "state %d failed: %v (left for retry on next run)\n", st.Index, err)
					continue
				}
				mu.Lock()
				newOnes++
				mu.Unlock()
				fmt.Printf("state %3d/%-3d frame %06d @ %s\n", st.Index+1, len(states), st.FrameNum, chapter.Hms(st.Sec))
			}
		}(scratch)
	}
	for _, st := range pending {
		jobs <- st
	}
	close(jobs)
	wg.Wait()
	if newOnes < len(pending) {
		fmt.Printf("WARNING: %d/%d states failed (transcripts left for retry on the next run)\n", len(pending)-newOnes, len(pending))
	}
	fmt.Printf("transcribed %d new states into %s (existing transcripts skipped)\n", newOnes, transDir)

	// Reconstruct the final code file from the ordered transcripts, then run
	// the structural checks and write the traceability manifest.
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
		fmt.Printf("structural review flagged %d issue(s) (see outputs/%s/code/review.md):\n", len(issues), id)
		for _, is := range issues {
			fmt.Println("  - " + is)
		}
	} else {
		fmt.Println("structural checks: clean")
	}
	return nil
}

// VisionHelper returns the path to the vision-inspect helper (inspect.mjs),
// overridable via VISION_INSPECT.
func VisionHelper() (string, error) {
	if p := os.Getenv("VISION_INSPECT"); p != "" {
		//nolint:gosec // p is an explicit config path (env override), not user-tainted input; Stat only checks existence
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("VISION_INSPECT %q not found: %w", p, err)
		}
		return p, nil
	}
	def := filepath.Join(os.Getenv("USERPROFILE"), "Documents", "projects", "vision-inspect", "inspect.mjs")
	//nolint:gosec // def is a documented default location; Stat only checks existence
	if _, err := os.Stat(def); err != nil {
		return "", fmt.Errorf("vision-inspect helper not found at %s — set VISION_INSPECT to its path: %w", def, err)
	}
	return def, nil
}

// transcribeOne crops state st's frame to the pane and asks the vision helper
// to transcribe it verbatim, writing the transcript to out.
func transcribeOne(helper, scratch, id string, st State, rect Rect, out string) error {
	frame := filepath.Join(paths.FramesDir(id), fmt.Sprintf("frame_%06d.jpg", st.FrameNum))
	one := filepath.Join(scratch, "one.jpg")
	if err := crop.Single(frame, one, crop.CropRect{X: rect.X, Y: rect.Y, Width: rect.W, Height: rect.H}); err != nil {
		return fmt.Errorf("crop state %d frame %s: %w", st.Index, frame, err)
	}
	//nolint:gosec // "node" is a fixed binary; helper/scratch are file paths passed as argv (never a shell); prompt is a constant
	cmd := exec.Command("node", helper, scratch, "-q", MQL5Prompt, "--think", "--detail", "original")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("vision read state %d (frame %06d): %w: %s", st.Index, st.FrameNum, err, strings.TrimSpace(stderr.String()))
	}
	body := fmt.Sprintf("=== state %d — frame_%06d.jpg @ %s ===\n%s\n",
		st.Index, st.FrameNum, chapter.Hms(st.Sec), stdout.String())
	return os.WriteFile(out, []byte(body), 0o644)
}

// transcriptPath returns outputs/<id>/code/transcripts/state_%06d.txt for st.
func transcriptPath(transDir string, st State) string {
	return filepath.Join(transDir, fmt.Sprintf("state_%06d.txt", st.Index))
}

func writeStatesJSON(path string, states []State) error {
	b, err := json.MarshalIndent(states, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
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
