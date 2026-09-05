# Project rules (yt-code-vision-skill)

## Project
Go CLI that downloads YouTube videos and extracts 1fps frames + chapter
metadata for code-reading workflows. Stdlib-first (one dependency —
`gopkg.in/yaml.v3` for spec-compliant chapters.yaml); shells out to `yt-dlp`
(download), `ffmpeg` (frame extraction) and `ffprobe` (duration). Entry point
`cmd/yt-code-vision-skill/main.go`; input is a txt of URLs (default
`YT-URL-TEST.txt`).

## Golden rules
- **Stay in this project folder only.** Never read or modify any other project.
- Keep it **as simple as possible** — no premature structure, no fancy abstractions.
- Prefer the **standard library** until a dependency genuinely pays for itself.

## Formatting & linting (do this, don't skip)
Auto format/lint/fix is ONE command — run it before finishing any change and
whenever anything looks off:

```bash
./dev.sh          # fmt -> golangci-lint --fix (incl. gosec) -> vet -> test -> govulncheck -> build
./dev.sh fmt      # just gofmt + goimports -w
./dev.sh lint     # just golangci-lint run --fix ./...
./dev.sh vuln     # just govulncheck ./... (dependency vulnerability scan)
```

- Always write code that is already `gofmt`-clean; `./dev.sh` is the safety net.
- `golangci-lint` v2 config lives in `.golangci.yml` (standard linters + `gosec`
  SAST + gofmt/goimports formatters; local imports grouped under the module
  path). gosec's permission rules (G301/G302/G306) and file-inclusion G304 are
  excluded — see the rationale comments in `.golangci.yml`.
- Never leave a non-zero `./dev.sh` — fix and re-run until it prints `all clean`.
- Do not hand-edit formatting to "look right"; let the tools do it.

## Layout
Standard Go layout: one thin `main` package under `cmd/`, library code under
`internal/` (each package owns one concern). Add a package only when a body of
code is self-contained enough to warrant it.

Per-video output layout (see `internal/paths` — always use those helpers, never
hardcode paths):
- `outputs/<id>/frames/` — full-res 1fps frames
- `outputs/<id>/chapters/chapters.yaml`
- `outputs/<id>/crop.json`, `outputs/<id>/crop/`, `outputs/<id>/previews/`
- `outputs/.cache/<id>` — cached media masters (kept for reuse, never deleted
  by normal runs)
- `outputs/.trash/` — data moved aside by `--fresh` (recoverable)
- `bench/<id>/<res>p/` — benchmark frames (top-level, by design)

## Architecture
- `cmd/yt-code-vision-skill` — entry point, flag parsing (`config`), per-URL
  dispatch, `processVideo` orchestration.
- `internal/media` — cache-aware `yt-dlp` download, throttle/backoff ladder,
  missing-JS-runtime fast-fail, `RunCmd`, `ffprobe` duration.
- `internal/cache` — persistent media cache, cross-run pacing, trash,
  `--purge-cache`, `.done` marker.
- `internal/frames` — ffmpeg frame extraction (`ExtractFrames`/`ExtractScaled`).
- `internal/bench` — resolution ladder + 5-min master window.
- `internal/chapter` — chapters.yaml writer + coding-chapter start-offset detection.
- `internal/crop` — code-pane crop detection (`detectCodeRect`, `findGreenBox`) & application.
- `internal/urls` — URL file loading + 11-char video-id parsing.
- `internal/paths` — output layout helpers (always use these, never hardcode paths).

## Anti-rate-limit & safety rules (do NOT violate)
- **Never delete `outputs/` data with `rm -rf`.** Destructive redo uses
  `--fresh`, which moves the old folder to `outputs/.trash/`. Prefer idempotent
  re-runs (`.done` marker skips finished videos) over deleting.
- **Do not delete or hand-clear `outputs/.cache`** — that is what prevents
  re-downloading the same video (which is what triggered YouTube's per-IP 403
  block). Use `go run ./cmd/yt-code-vision-skill --purge-cache` only when explicitly asked.
- **Never bypass the pacing/backoff in `media.DownloadVideo`.** Media downloads are
  spaced by a shared minimum gap (20s default) and 403/429 gets exponential
  backoff (15s/60s/240s) then a clean stop. Do not loop downloads manually to
  "get past" a 403 — that makes the block worse.
- **Tests must never write to the real `outputs/` tree** — chdir into a temp
  sandbox or use `t.TempDir()`. (See `internal/cache/cache_test.go`.)
- If a download is 403-blocked, report it and wait/retry later — never hammer.

## Commands
- `./dev.sh` — format + lint-fix + vet + test + govulncheck + build (prints `all clean`); `./dev.sh fmt|lint|vuln|check` for stages
- `govulncheck` is installed via `go install golang.org/x/vuln/cmd/govulncheck@latest` (dev.sh skips it gracefully if absent)
- `go build ./...` / `go test ./...` — direct equivalents (build writes `bin/yt-code-vision-skill(.exe)`)
- `go run ./cmd/yt-code-vision-skill` — process URLs in the txt (idempotent; `.done` skips finished videos)
- `go run ./cmd/yt-code-vision-skill --fresh` — redo a video (old data -> outputs/.trash)
- `go run . --purge-cache` — explicitly delete cache + trash
- `go run ./cmd/yt-code-vision-skill --bench` — 5-min frames at 144/240/360/480/720/1080p -> bench/<id>/<res>p/
- `go run ./cmd/yt-code-vision-skill --bench-verdict` — vision-model readability verdict per resolution -> bench/<id>/verdict.md
- `go run ./cmd/yt-code-vision-skill --find-crop` — auto-detect code pane -> crop.json + preview
- `go run ./cmd/yt-code-vision-skill --crop-from-ref <png>` — green-box detect on a reference image -> crop.json
- `go run ./cmd/yt-code-vision-skill --crop-frames` — apply crop.json to all frames -> outputs/<id>/crop/
- Overrides: `--input <txt>`, `--height <px>`, `--start-chapter <keyword>`

## Notes

