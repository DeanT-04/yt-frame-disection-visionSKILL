# Pipeline Failure Report — YouTube `403: Forbidden` on Media Download

> **Status:** ⛔ BLOCKED — reproducible, ongoing
> **Last verified:** 2026-09-05 05:13 (run 2, full 4-attempt backoff ladder)
> **Video under test:** `mtWN6oPIi1Y` — "The Wayward Trading Bot... The Wait Is OVER!! (Full Code)", 2166 s (~36 min), 2160p source
> **Scope of this file:** exact, evidence-backed walkthrough of what happens when the pipeline (`go run .`) is executed, zoomed into the main issue — YouTube refusing the actual video-data fetch with HTTP 403 while every metadata request succeeds.

---

## 1. Executive summary

The pipeline **never gets past its first stage** (media download). Running `go run .` reproduces, 100 % of the time across two separate test windows (~38 hours apart), the following split:

| Request type | Result |
|---|---|
| YouTube *webpage / player-API / metadata* (title, duration, chapters, format list) | ✅ Always succeeds |
| YouTube *video-data fetch* (the actual `.mp4` stream, `videoplayback` URL) | ❌ Always `HTTP Error 403: Forbidden` |

yt-dlp picks format **`137`** (1080p avc1, video-only) by default and format **`136`** (720p avc1) under `--height 720`; **both** are refused with 403. The block is therefore **not** resolution-, codec- or format-specific — it is a **per-IP block on media downloads** (classic YouTube bot-throttling on the videoplayback endpoint), exactly the scenario this repo's anti-rate-limit machinery was built for.

The app handles it correctly by design: it runs the sanctioned backoff ladder (attempts at +0 s / +15 s / +60 s / +240 s ≈ 5.5 min worst case) and then **stops cleanly** with a "try again later" error. It never hammers. But **time alone has not cleared the block** (first observed 2026-09-03 ~15:55, still present 2026-09-05 ~05:13 — see §6.1).

**The one actionable conclusion:** the pipeline cannot complete until the per-IP media block decays or the environment changes (new IP / authenticated client / different extraction path). No flag, format or resolution in this tool sidesteps it.

---

## 2. What "running the pipeline" actually does (stage by stage)

Entry: `main.go:54` `main()` → `main.go:92` `processVideo()` (per URL from `YT-URL-TEST.txt`).

| # | Stage | Code | Success output | Failure mode observed |
|---|---|---|---|---|
| 1 | Load URLs | `main.go:61` | `processing 1 URL(s) from YT-URL-TEST.txt` | — |
| 2 | Mkdir tree | `main.go:119-123` | creates `outputs/<id>/`, `frames/`, `chapters/`, `outputs/.cache/<id>/` | — |
| 3 | **Download master** | `main.go:127` → `video.go:69` `downloadVideo()` | `media -> … (2166 s total duration, cached for reuse)` | **⛔ 403 here — pipeline dies** |
| 4 | Pick frame start | `main.go:138-144` `chapterStart()` | `frame start: @… (chapter "…" …)` | never reached |
| 5 | Extract 1 fps frames | `main.go:148-152` → `frames.go:9` `extractFrames()` | `frames: N extracted into …` | never reached |
| 6 | Write chapters YAML | `main.go:154-162` `writeChaptersYAML()` | `chapters: N -> …/chapters.yaml` | never reached |
| 7 | Mark done (idempotency) | `main.go:166` `.done` | `done: … (cached master kept at …)` | never reached |

So of 7 stages, **only stages 1–3 start**, and stage 3 always fails. No frames, no chapters, no `.done` marker are ever produced.

### 3.1 Stage-3 internals (the part that breaks)

`downloadVideo` (`video.go:69-144`) does, in order:

1. **Cache check** (`video.go:77-82`): if `outputs/.cache/<id>/<id>.mp4` (> 1 MB) + `<id>.info.json` exist → zero-network reuse. *Not the case here — no media was ever successfully downloaded.*
2. **Stale-partial cleanup** (`video.go:85-86`): removes any leftover `.part` / `.mp4`.
3. **Pacing wait** (`video.go:88` → `cache.go:58` `waitForPacing()`): sleeps until 20 s (default) after the *last successful* download. *Not triggered — no `.last_download` stamp exists because nothing ever succeeded (`cache.go:78` `markDownloaded()` is only called on success, `video.go:130`).*
4. **yt-dlp invocation** (`video.go:92-103`):
   - format selector: `bv*[height<=1080][vcodec^=avc1]/bv*[height<=1080]` (or `<=720` with `--height 720`)
   - `--write-info-json` into the cache dir, then `-- <videoURL>`
5. **Backoff ladder** (`video.go:105-128`): up to **4 attempts**, waiting 15 s → 60 s → 240 s between retries **only when the error is classified as throttle** (`isThrottle`, `video.go:50` — matches `403`, `429`, `rate limit`, etc.).
6. **Failure exit** (`video.go:127-129`): after 4 throttle attempts → `download blocked by YouTube after 4 attempts — try again later: …`

### 3.2 The exact command that fails every time

```text
yt-dlp --no-playlist --no-warnings
       -f bv*[height<=1080][vcodec^=avc1]/bv*[height<=1080]
       --write-info-json
       -o outputs\.cache\mtWN6oPIi1Y\mtWN6oPIi1Y.%(ext)s
       -- https://www.youtube.com/watch?v=mtWN6oPIi1Y
```

---

## 4. Raw evidence — run 2 (2026-09-05 05:07–05:13), full ladder to clean stop

Repro command: `go run .` (background), output captured to a log. Verbatim transcript:

```text
processing 1 URL(s) from YT-URL-TEST.txt

== mtWN6oPIi1Y ==
[youtube] Extracting URL: https://www.youtube.com/watch?v=mtWN6oPIi1Y
[youtube] mtWN6oPIi1Y: Downloading webpage                     ← stage A: OK
[youtube] mtWN6oPIi1Y: Downloading android vr player API JSON  ← stage B: OK
[info] mtWN6oPIi1Y: Downloading 1 format(s): 137               ← format picked (1080p avc1)
[info] Writing video metadata as JSON to: outputs\.cache\mtWN6oPIi1Y\mtWN6oPIi1Y.info.json
ERROR: unable to download video data: HTTP Error 403: Forbidden ← stage C: BLOCKED
[throttle] attempt 1/4 blocked by YouTube (yt-dlp …: exit status 1 (… HTTP Error 403: Forbidden))
[throttle] attempt 1/4 blocked; waiting 15s before retry…
[youtube] … Downloading webpage / android vr player API JSON    ← repeats, OK
[info] … 137
ERROR: unable to download video data: HTTP Error 403: Forbidden
[throttle] attempt 2/4 blocked by YouTube (…)
[throttle] attempt 2/4 blocked; waiting 1m0s before retry…
… attempt 3 … waiting 4m0s … attempt 4 …
ERROR: unable to download video data: HTTP Error 403: Forbidden
FAIL mtWN6oPIi1Y: download blocked by YouTube after 4 attempts — try again later: yt-dlp … HTTP Error 403: Forbidden
```

Observed facts from this run:

- **`grep -c "HTTP Error 403"` = 9** occurrences → 4 distinct failed attempts (each attempt emits the error once raw + once inside the wrapped Go error, plus the final `FAIL` wrapper).
- Attempt spacing confirmed: +0 s, +15 s, +60 s, +240 s (tool's own `[throttle] attempt N/4 blocked; waiting …` lines).
- **Program exit code = 0** even though the video failed — see issue 6.3.
- After the run: **no `.done`**, **no `.mp4`**, **0 frames**, only a re-written `info.json` (572,760 bytes, mtime `Sep 5 05:13`).

### 4.1 What the metadata stage proves (it is NOT a generic IP ban)

Every attempt successfully completes three separate YouTube requests *before* the media fetch:

```text
[youtube] Extracting URL: https://www.youtube.com/watch?v=mtWN6oPIi1Y
[youtube] mtWN6oPIi1Y: Downloading webpage
[youtube] mtWN6oPIi1Y: Downloading android vr player API JSON
[info] mtWN6oPIi1Y: Downloading 1 format(s): 137
```

`--write-info-json` then lands a full 572 KB `info.json` in the cache — containing the complete format table incl. stream URLs (that is why the 403s all occur only after the format has been selected). Conclusion: **the player/client path is not blocked; only the `videoplayback` media endpoints are refused.** This is the signature of per-IP rate-limiting / bot detection on media downloads, not an account or geo block.

---

## 5. Raw evidence — run 1 (2026-09-03 ~15:55), including the 720p experiment

### 5.1 Pre-run probe (misleadingly "clean")

```text
$ yt-dlp --no-playlist --no-warnings --skip-download \
         --print "%(id)s | %(title)s | %(duration).0fs | %(height)sp" \
         https://youtu.be/mtWN6oPIi1Y
mtWN6oPIi1Y | The Wayward Trading Bot... The Wait Is OVER!! (Full Code) | 2166s | 2160p
probe exit: 0
```

**⚠️ Key lesson (issue 6.2):** this probe *does not* exercise the media fetch — it only hits the player/API path (exactly the path that is NOT blocked). A "clean" metadata probe therefore **cannot** tell you the block has lifted. Only a real media download is conclusive.

### 5.2 Default 1080p run — format 137 blocked

Same transcript shape as §4: format `137` selected → `ERROR: unable to download video data: HTTP Error 403: Forbidden` twice (run was stopped at attempt 2/4 as two consecutive 403s were already conclusive; the ladder was not allowed to burn its remaining 60 s + 240 s).

### 5.3 `--height 720` experiment — format 136 blocked identically

Requested by the user to test whether only the 1080p/avc1 stream was throttled:

```text
[info] mtWN6oPIi1Y: Downloading 1 format(s): 136        ← 720p avc1
ERROR: unable to download video data: HTTP Error 403: Forbidden
[throttle] attempt 1/4 blocked … waiting 15s before retry…
[info] … 136
ERROR: unable to download video data: HTTP Error 403: Forbidden
[throttle] attempt 2/4 blocked …
```

→ **Same 403 on a different format & resolution.** Conclusive: per-IP block on media downloads, not a format-selection problem. Stopped at attempt 2/4.

---

## 6. The main issues, zoomed in

### 6.1 ⛔ Issue 1 — Per-IP 403 block on the media endpoint; does not decay quickly

| | |
|---|---|
| **Symptoms** | All metadata OK; every media fetch → `HTTP Error 403: Forbidden`; identical across formats 137/136; identical across two runs. |
| **First observed** | 2026-09-03 ~15:55 (filesystem mtimes: `outputs/` dirs `Sep 3 15:55`, cache `info.json` `Sep 3 15:56`) |
| **Re-verified** | 2026-09-05 05:07–05:13 → **~38 h later the block is unchanged** |
| **Code path** | `video.go:92-103` builds the yt-dlp command; the 403 arrives from the *download*, not the extraction — `video.go:50-58` `isThrottle()` classifies it correctly as throttle. |
| **Why "wait it out" alone is not sufficient evidence** | The project rule assumes these blocks decay over "tens of minutes to a few hours" (`AGENTS.md`, prior-session analysis). 38 h of elapsed time disproves that assumption for this IP/video pair. |

**Likely contributing factors (hypotheses, unverified — no extra network hammering was done to test them):**
1. **No authenticated client / no cookies / no PO-token.** The extraction path is `android vr player API JSON` (visible in every log line). YouTube increasingly requires a PO (Proof-of-Origin) token or logged-in cookies for the videoplayback URLs handed to anonymous android/ios clients — a common cause of *exactly this symptom*: player JSON fine, media URL 403.
2. **yt-dlp `2026.07.04`** may predate YouTube's current signing/token requirements for this client.
3. **Prior request volume from this IP** triggered the block; it persists because every re-run adds 4 more attempts.

**What would need to change (options — none applied, all require a user decision / are outside the current code):** wait substantially longer; run from a different IP/network; supply cookies (`--cookies-from-browser` / a cookies.txt); update yt-dlp; or accept an authenticated extraction path. Per the repo's anti-rate-limit rules these were deliberately **not** attempted.

### 6.2 ⚠️ Issue 2 — The probe is a false-negative machine

The cheap readiness probe (`yt-dlp --skip-download … --print`, §5.1) and even the app's own metadata fetch inside each attempt both succeed while the block is fully active. Anyone checking "is the block gone?" with a metadata-only probe gets **"yes, clean"** — then the real download 403s immediately. Evidence: §5.1 probe exit 0 on 2026-09-03, minutes before format 137 403'd; §4 shows the same split *within a single run*.

**The only valid test of the block is a real media download** (or a full `go run .`). Cheap probes are fine for "is the video still up" but must never be used to declare the throttle cleared.

### 6.3 ⚠️ Issue 3 — A fully blocked run still exits 0

Observation from run 2: after 4 attempts and the `FAIL … download blocked by YouTube after 4 attempts` message, the process exited **0** (verified via the run's exit-code capture). Cause: `main.go:94-96` logs per-video failures with `log.Printf` but never propagates an error to the exit status (`main()` only `log.Fatalf`s on usage/input errors). For a batch input that is deliberate (one bad video shouldn't fail the rest), but for the current single-URL input it means **shell automation cannot distinguish "pipeline completed" from "pipeline blocked"** by exit code — it must parse stderr for `FAIL`/`blocked by YouTube`.

### 6.4 ⚠️ Issue 4 — Failed attempts leave no pacing trace (cross-run hammering is only prevented by convention)

`markDownloaded()` — which stamps `outputs/.cache/.last_download` — is called **only after a successful download** (`video.go:130`). Because every run so far failed, **no `.last_download` file exists**, so `waitForPacing()` (`cache.go:58-75`) has nothing to enforce and every fresh `go run .` fires its full 4-attempt ladder immediately (evidence: `.cache/` contains only `mtWN6oPIi1Y/`, no `.last_download`; run 1 and run 2 both started attempt 1 with zero inter-run delay, ~38 h apart).

Consequence: nothing *in code* stops a user/agent from launching `go run .` repeatedly and stacking 4 YouTube media hits per invocation with no inter-run spacing. The repo currently relies solely on the AGENTS.md rule ("report it and wait/retry later — never hammer") and on `isThrottle` only applying backoff *within* a run. A cheap hardening would be to stamp a failed-attempt timestamp too (e.g. write `.last_download` on the final 403) so even failed runs are paced.

### 6.5 ℹ️ Issue 5 — Blocked runs leave semi-prepared state (benign, but looks "stuck")

After every blocked run the tree contains:

```text
outputs/.cache/mtWN6oPIi1Y/mtWN6oPIi1Y.info.json   ← 572 KB, rewritten every attempt (metadata ONLY)
outputs/mtWN6oPIi1Y/frames/                          ← empty
outputs/mtWN6oPIi1Y/chapters/                        ← empty
outputs/mtWN6oPIi1Y/.done                            ← absent (so re-runs are NOT skipped)
```

No `.mp4`, no frames, no `.done`. This is the *correct* idempotent state (a later successful run reuses the cached `info.json` and proceeds), but the presence of a full `outputs/<id>/` skeleton with an absent `.done` can look like a half-completed run. The cache hit at `video.go:77-82` never triggers because it requires an `.mp4` > 1 MB.

### 6.6 ℹ️ Issue 6 — The app's own handling works as designed (not a bug)

To be fair to the code: the throttle classifier, backoff ladder and clean stop all behaved exactly as specified (`video.go:105-129`). The message `download blocked by YouTube after 4 attempts — try again later` is the intended terminal state; the design's assumption (blocks decay in minutes–hours) is the only part reality has contradicted (§6.1).

---

## 7. Filesystem evidence (state after run 2)

```text
$ date                                   → Sat Sep  5 05:07:04 GMTST 2026
$ ls -la outputs/
drwxr-xr-x … Sep  3 15:55 .cache
drwxr-xr-x … Sep  3 15:55 mtWN6oPIi1Y
$ ls -la outputs/.cache/mtWN6oPIi1Y/
-rw-r--r-- … 572760 Sep  5 05:13 mtWN6oPIi1Y.info.json     ← rewritten by run 2 (4×)
$ cat outputs/mtWN6oPIi1Y/.done          → No such file or directory
$ ls outputs/mtWN6oPIi1Y/frames | wc -l  → 0
$ ls outputs/.cache/.last_download       → No such file (no successful download ever)
$ git status --short                      → clean (outputs/ is gitignored)
```

`info.json` proves metadata completeness each attempt: contains `id: mtWN6oPIi1Y`, full `formats` array incl. storyboards and `137`/`136`, `title`, `duration`, `chapters`. It is the *only* artifact YouTube lets us keep.

---

## 8. Environment (at time of evidence)

| Component | Version | Notes |
|---|---|---|
| OS | Windows (GMTST, bash shell) | — |
| Go | 1.26.5 windows/amd64 | go.mod `go 1.26.5` |
| yt-dlp | 2026.07.04 | scoop shim; drives extraction + download |
| ffmpeg / ffprobe | 9.0-full_build-www.gyan.dev | used only for frames/chapters (never reached) |
| Video | `mtWN6oPIi1Y`, 2166 s, 2160p | from `YT-URL-TEST.txt` |
| Format ladder | default `bv*[height<=1080][vcodec^=avc1]…` → 137; `--height 720` → 136 | both 403 |
| Test suite | `./dev.sh` → `all clean ✔` (fmt, golangci-lint 0 issues, vet, test ok, govulncheck 0 reachable, build) | green — problem is 100 % external |

---

## 9. How to reproduce & how to read the result

```bash
# 1) cheapest valid signal — the real test is a media fetch, so just run the pipeline:
go run .

# 2) result reading
#    - "FAIL … download blocked by YouTube after 4 attempts"   → STILL BLOCKED (~5.5 min, clean stop)
#    - "media -> … (2166 s …)" then "frames: N …" "chapters: …" "done: …" → UNBLOCKED, full e2e ran
#    - "already processed (.done present) — skipping"           → video finished in a prior successful run

# 3) optional variant to test format independence (known blocked too):
go run . --height 720
```

Watch for the smoking-gun sequence in any run: `Downloading webpage` ✅ → `Downloading android vr player API JSON` ✅ → `Downloading 1 format(s): 137` → `ERROR: unable to download video data: HTTP Error 403: Forbidden` ❌.

---

## 10. Appendix — verbatim log excerpts

**Run 2, attempt 1 (2026-09-05):**

```text
[youtube] Extracting URL: https://www.youtube.com/watch?v=mtWN6oPIi1Y
[youtube] mtWN6oPIi1Y: Downloading webpage
[youtube] mtWN6oPIi1Y: Downloading android vr player API JSON
[info] mtWN6oPIi1Y: Downloading 1 format(s): 137
[info] Writing video metadata as JSON to: outputs\.cache\mtWN6oPIi1Y\mtWN6oPIi1Y.info.json
ERROR: unable to download video data: HTTP Error 403: Forbidden
[throttle] attempt 1/4 blocked by YouTube (yt-dlp --no-playlist --no-warnings -f bv*[height<=1080][vcodec^=avc1]/bv*[height<=1080] --write-info-json -o outputs\.cache\mtWN6oPIi1Y\mtWN6oPIi1Y.%(ext)s -- https://www.youtube.com/watch?v=mtWN6oPIi1Y: exit status 1 ([youtube] Extracting URL: https://www.youtube.com/watch?v=mtWN6oPIi1Y
[youtube] mtWN6oPIi1Y: Downloading webpage
[youtube] mtWN6oPIi1Y: Downloading android vr player API JSON
[info] mtWN6oPIi1Y: Downloading 1 format(s): 137
[info] Writing video metadata as JSON to: outputs\.cache\mtWN6oPIi1Y\mtWN6oPIi1Y.info.json
ERROR: unable to download video data: HTTP Error 403: Forbidden))
[throttle] attempt 1/4 blocked; waiting 15s before retry…
```

**Run 2, final state (tail of log):**

```text
ERROR: unable to download video data: HTTP Error 403: Forbidden
FAIL mtWN6oPIi1Y: download blocked by YouTube after 4 attempts — try again later: yt-dlp --no-playlist --no-warnings -f bv*[height<=1080][vcodec^=avc1]/bv*[height<=1080] --write-info-json -o outputs\.cache\mtWN6oPIi1Y\mtWN6oPIi1Y.%(ext)s -- https://www.youtube.com/watch?v=mtWN6oPIi1Y: exit status 1 (… HTTP Error 403: Forbidden)
EXIT=0
```

**Run 1, 720p variant (2026-09-03):**

```text
[info] mtWN6oPIi1Y: Downloading 1 format(s): 136
[info] Writing video metadata as JSON to: outputs\.cache\mtWN6oPIi1Y\mtWN6oPIi1Y.info.json
ERROR: unable to download video data: HTTP Error 403: Forbidden
[throttle] attempt 1/4 blocked by YouTube (… -f bv*[height<=720][vcodec^=avc1]/bv*[height<=720] … HTTP Error 403: Forbidden))
[throttle] attempt 1/4 blocked; waiting 15s before retry…
```

---

*Report generated 2026-09-05 from two instrumented pipeline runs (2026-09-03, 2026-09-05) + code reading. All log excerpts are verbatim from captured output; code references are to commit `60c5274` and stable file:line anchors. No extra YouTube requests were made beyond the tool's own sanctioned attempts.*
