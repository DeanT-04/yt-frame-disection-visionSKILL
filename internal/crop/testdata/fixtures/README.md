# Crop detector reference fixtures

Real screenshots used to validate/tune `detectCodeRect` (`internal/crop`) beyond
its synthetic unit tests. The detector supports two polarities — **light
background + dark text** (classic IDE theme) and **dark background + light
text** (dark theme / terminal) — so each fixture should exercise a distinct,
real layout.

## What to drop here

Add one PNG (or JPEG) per layout below. Use a **full-resolution** screen
recording frame (the same 1920x1080-ish size the pipeline produces), not a
thumbnail.

| File | What it should show |
| --- | --- |
| `dark_theme.png` | A dark-theme IDE with the code editor pane visible (light text on dark bg). |
| `terminal.png` | A terminal-heavy screen (shell / `vim` / `htop`), full-width dark panel with light text. |
| `nonstandard_editor.png` | A non-standard editor/layout (e.g. notebook, split panes, unusual chrome) to see how the detector copes. |
| `light_theme.png` | *(optional)* A light-theme IDE, to confirm the original polarity still holds on real pixels. |

Screenshots are **not** required to build or pass CI — the golden test skips
cleanly when a file is absent.

## Wire each fixture into the golden test

1. Open `internal/crop/golden_test.go`.
2. Measure the code pane's rectangle **in the image's own pixel coordinates**
   (top-left `x,y` and `width,height`) — any image editor shows this.
3. Add/keep the entry, e.g.:
   ```go
   {"dark_theme.png", CropRect{X: 340, Y: 90, Width: 1500, Height: 860}, 24},
   ```
   The `24` is the per-edge pixel tolerance.
4. Run `go test ./internal/crop -run TestGoldenFixtures -v` and tune thresholds
   until it passes.

The existing `ref_image.png` is the original green-box reference (used by the
`--crop-from-ref` flow), kept here for continuity.
