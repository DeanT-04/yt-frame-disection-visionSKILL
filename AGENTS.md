# Project rules (yt-code-vision-skill)

## Project
Single-package Go CLI that downloads YouTube videos and extracts 1fps frames +
chapter metadata for code-reading workflows. Pure stdlib (no Go dependencies);
shells out to `yt-dlp` (download), `ffmpeg` (frame extraction) and `ffprobe`
(duration). Entry point `main.go`; input is a txt of URLs (default
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
Flat and small on purpose. `main.go` = entry point. Add packages only when the
single file stops being obviously simpler.

Per-video output layout (see `paths.go` — always use those helpers, never
hardcode paths):
- `outputs/<id>/frames/` — full-res 1fps frames
- `outputs/<id>/chapters/chapters.yaml`
- `outputs/<id>/crop.json`, `outputs/<id>/crop/`, `outputs/<id>/previews/`
- `outputs/.cache/<id>` — cached media masters (kept for reuse, never deleted
  by normal runs)
- `outputs/.trash/` — data moved aside by `--fresh` (recoverable)
- `bench/<id>/<res>p/` — benchmark frames (top-level, by design)

## Architecture
Flat `package main` on purpose (files act as modules — add a package only when
the single package stops being obviously simpler):
- `main.go` — entry point, flag parsing (`config`), per-URL dispatch, `processVideo`.
- `video.go` — cache-aware `yt-dlp` download, throttle/backoff ladder, `runCmd`, `ffprobe` duration.
- `cache.go` — persistent media cache, cross-run pacing, trash, `--purge-cache`, `.done` marker.
- `frames.go` / `bench.go` — ffmpeg frame extraction (`extractFrames`/`extractScaled`); bench resolution ladder.
- `chapters.go` — chapters.yaml writer; `chapterstart.go` — coding-chapter start-offset detection.
- `urls.go` — URL file loading + 11-char video-id parsing.
- `crop.go` / `crop_cmd.go` / `crop_ref.go` / `greenbox.go` — code-pane crop detection & application.
- `paths.go` — output layout helpers (always use these, never hardcode paths).

## Anti-rate-limit & safety rules (do NOT violate)
- **Never delete `outputs/` data with `rm -rf`.** Destructive redo uses
  `--fresh`, which moves the old folder to `outputs/.trash/`. Prefer idempotent
  re-runs (`.done` marker skips finished videos) over deleting.
- **Do not delete or hand-clear `outputs/.cache`** — that is what prevents
  re-downloading the same video (which is what triggered YouTube's per-IP 403
  block). Use `go run . --purge-cache` only when explicitly asked.
- **Never bypass the pacing/backoff in `downloadVideo`.** Media downloads are
  spaced by a shared minimum gap (20s default) and 403/429 gets exponential
  backoff (15s/60s/240s) then a clean stop. Do not loop downloads manually to
  "get past" a 403 — that makes the block worse.
- **Tests must never write to the real `outputs/` tree** — chdir into a temp
  sandbox or use `t.TempDir()`. (See `hardening_test.go`.)
- If a download is 403-blocked, report it and wait/retry later — never hammer.

## Commands
- `./dev.sh` — format + lint-fix + vet + test + govulncheck + build (prints `all clean`); `./dev.sh fmt|lint|vuln|check` for stages
- `govulncheck` is installed via `go install golang.org/x/vuln/cmd/govulncheck@latest` (dev.sh skips it gracefully if absent)
- `go build ./...` / `go test ./...` — direct equivalents (build writes `bin/app.exe`)
- `go run .` — process URLs in the txt (idempotent; `.done` skips finished videos)
- `go run . --fresh` — redo a video (old data -> outputs/.trash)
- `go run . --purge-cache` — explicitly delete cache + trash
- `go run . --bench` — 5-min frames at 144/240/360/480/720/1080p -> bench/<id>/<res>p/
- `go run . --find-crop` — auto-detect code pane -> crop.json + preview
- `go run . --crop-from-ref <png>` — green-box detect on a reference image -> crop.json
- `go run . --crop-frames` — apply crop.json to all frames -> outputs/<id>/crop/
- Overrides: `--input <txt>`, `--height <px>`, `--start-chapter <keyword>`

## Notes

