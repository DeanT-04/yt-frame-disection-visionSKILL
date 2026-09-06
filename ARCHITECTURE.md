# Architecture

Standard Go layout: one thin `main` package under `cmd/`, library code under
`internal/`. No Go dependencies beyond `yaml.v3`; the heavy lifting is shelled
out to `yt-dlp`, `ffmpeg`, and `ffprobe`.

## Repo layout

```
cmd/yt-code-vision-skill/     entry point, flag parsing, dispatch, processVideo
internal/media/               yt-dlp download, throttle/backoff, JS-runtime detection, ffprobe
internal/cache/               media cache, cross-run pacing, trash, --purge-cache, .done
internal/frames/              ffmpeg frame extraction (ExtractFrames / ExtractScaled)
internal/chapter/             chapters.yaml writer + scored coding-chapter matcher
internal/crop/                code-pane detection (light + dark), green-box, crop apply
internal/urls/                URL loading + 11-char video-id parsing
internal/paths/               single source of truth for the output layout
internal/bench/               resolution ladder + vision-model readability verdict
internal/extract/             video→code: state detection, vision transcription, reconstruction
```

Dependency direction is one-way: `cmd` → everything; `bench` → media/cache/
chapter/frames/crop/paths; `chapter`, `frames`, `crop` → media/paths; `cache` →
paths. No import cycles.

## Data flow

1. `urls.LoadURLs` reads the txt file; `urls.VideoID` extracts a whitelisted
   11-char id (the single gate every output path and URL derives from).
2. `media.DownloadVideo` reuses a cached master in `outputs/.cache/<id>` if
   present, else paces + downloads a video-only h264 mp4 via `yt-dlp`.
3. `chapter.ChapterStart` scores chapters to find where coding begins; frames
   start there (or at 0 if nothing matches).
4. `frames.ExtractFrames` runs `ffmpeg` at 1 fps into `outputs/<id>/frames/`.
5. `chapter.WriteChaptersYAML` writes spec-compliant `chapters.yaml`
   (`chapters: null` when empty).
6. Optional: `crop.FindCrop`/`CropFrames` detect and apply the code pane;
   `bench.RunBench` builds the resolution ladder; `bench.RunVerdict` reads each
   rung with the vision model and recommends the lowest readable one.
7. Optional (`--extract-code`): `extract.Run` requires a `crop.json`, detects
   distinct screen states by pixel-diffing downscaled consecutive frames
   (`DetectStates` → `states.json`), transcribes each state's cropped frame via
   the `vision-inspect` helper (resumable, worker pool), merges the ordered
   transcripts into `code/reconstructed.mq5` by tail-overlap alignment
   (`Reconstruct`), and lints the result for placeholder chars and unbalanced
   brackets (`CheckStructure`).

## Key decisions

- **Idempotency.** A `.done` marker skips finished videos; `--fresh` moves old
  data to `outputs/.trash/` (never `rm -rf`).
- **Anti-rate-limit.** Masters persist in `outputs/.cache/` for zero-network
  reuse; a shared pacing file enforces a minimum gap between any two media
  downloads across runs; 403/429 gets a 15s/60s/240s backoff ladder then a clean
  stop. A deterministic missing-JS-runtime failure is detected and short-
  circuits the ladder (it never clears by waiting).
- **Chapter matching.** Weighted positive intent words, negative lead-in/out
  words, and a longest-non-negative-chapter fallback; `--start-chapter` pins
  exactly.
- **Crop detection.** A coarse 16px block grid + maximal-rectangle search, run
  for both light and dark panel polarity, so light and dark IDEs (and terminals)
  are handled. Interior "ink" is required so a plain card isn't selected.
- **Bench verdict.** Crops each resolution's frames (scaled box), runs the
  `vision-inspect` helper in read mode over a deduped sample, and diffs each
  rung's transcription against the 1080p baseline with a token-Jaccard
  similarity; the lowest rung at ≥90% overlap wins.
- **Code reconstruction.** States are changes in the code pane, not frames —
  consecutive frames are 160px-wide grayscale-diffed so codec noise and the
  blinking caret don't split states. Transcript windows are merged with the
  largest prefix-of-new vs tail-of-file alignment that reaches the file's end,
  so overlapping re-shows of the same code converge instead of duplicating.

## Output & cache layout

See `internal/paths` for the authoritative helpers. `outputs/` and `bench/` are
git-ignored; `outputs/.cache/` and `outputs/.trash/` are the persistent state
that makes re-runs cheap and safe.
