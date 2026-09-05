package crop

import (
	"fmt"
	"image"
	"sort"
)

// BoxCorners is the code-pane box recovered from a green-outline reference
// image, in that image's pixel coordinates. Coordinates are the INSIDE of the
// green outline (an inset past the border bands).
type BoxCorners struct {
	MinX, MinY int // top-left (inside border)
	MaxX, MaxY int // bottom-right (inside border)
}

func (b BoxCorners) Width() int  { return b.MaxX - b.MinX }
func (b BoxCorners) Height() int { return b.MaxY - b.MinY }
func (b BoxCorners) String() string {
	return fmt.Sprintf("TL=(%d,%d) BR=(%d,%d)  box %dx%d", b.MinX, b.MinY, b.MaxX, b.MaxY, b.Width(), b.Height())
}

// isGreen reports whether a pixel reads as the marker-green outline:
// green channel clearly dominant over red and blue.
func isGreen(r, g, b uint32) bool {
	g8 := int(g >> 8)
	r8 := int(r >> 8)
	b8 := int(b >> 8)
	return g8 > 140 && g8 > r8+60 && g8 > b8+60
}

// findGreenBox locates the green-outline rectangle in an image. It builds
// per-row and per-column green coverage histograms; the outline's horizontal
// and vertical strokes show up as long bands. Returns the corners just inside
// the outline (an inset so the border pixels themselves are excluded).
func findGreenBox(img image.Image) (BoxCorners, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()

	// Coverage per row and per column (count of green pixels), sampled every
	// other pixel for speed.
	rowGreen := make([]int, h)
	colGreen := make([]int, w)
	greenPx := 0
	for y := b.Min.Y; y < b.Max.Y; y += 2 {
		for x := b.Min.X; x < b.Max.X; x += 2 {
			r, g, bl, _ := img.At(x, y).RGBA()
			if isGreen(r, g, bl) {
				rowGreen[y-b.Min.Y]++
				colGreen[x-b.Min.X]++
				greenPx++
			}
		}
	}
	if greenPx == 0 {
		return BoxCorners{}, fmt.Errorf("no green outline pixels found in image")
	}

	// A border stroke spans most of the image's width (for rows) or height
	// (for columns). Threshold at 60% of the sampled extent.
	rowThresh := int(0.6 * float64(w) / 2)
	colThresh := int(0.6 * float64(h) / 2)

	rows := findBands(rowGreen, rowThresh)
	cols := findBands(colGreen, colThresh)

	if len(rows) != 2 || len(cols) != 2 {
		return BoxCorners{}, fmt.Errorf("expected 2 horizontal + 2 vertical green bands, got %d rows / %d cols",
			len(rows), len(cols))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].center < rows[j].center })
	sort.Slice(cols, func(i, j int) bool { return cols[i].center < cols[j].center })

	// Box interior: just inside the four strokes (exclude the border itself).
	return BoxCorners{
		MinX: cols[0].max + 2,
		MinY: rows[0].max + 2,
		MaxX: cols[1].min - 2,
		MaxY: rows[1].min - 2,
	}, nil
}

// band is a contiguous run of indices whose coverage exceeds the threshold.
type band struct{ min, max, center int }

// findBands scans a 1-D coverage histogram and clusters indices above the
// threshold, tolerating small gaps (up to bandGap) so a 2-3px border stroke
// sampled every other pixel still reads as one band. Returns the two widest
// clusters — the top/bottom (or left/right) border strokes — plus up to two
// runner-up bands so callers can sanity-check against noise.
func findBands(cov []int, thresh int) []band {
	const bandGap = 4
	var clusters []band
	for i, c := range cov {
		if c < thresh {
			continue
		}
		if len(clusters) > 0 && i-clusters[len(clusters)-1].max <= bandGap {
			last := &clusters[len(clusters)-1]
			if i > last.max {
				last.max = i
			}
		} else {
			clusters = append(clusters, band{min: i, max: i})
		}
	}
	for i := range clusters {
		clusters[i].center = (clusters[i].min + clusters[i].max) / 2
	}
	sort.Slice(clusters, func(i, j int) bool {
		return (clusters[i].max - clusters[i].min) > (clusters[j].max - clusters[j].min)
	})
	if len(clusters) > 2 {
		clusters = clusters[:2]
	}
	sort.Slice(clusters, func(i, j int) bool { return clusters[i].min < clusters[j].min })
	return clusters
}
