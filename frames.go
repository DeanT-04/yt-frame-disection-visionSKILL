package main

// extractFrames grabs one JPEG per second from the media into outDir, using
// threads (80% of logical CPUs) and quality 2. startSec seeks into the media
// first (only frames from that point onward are decoded — saves time on long
// videos when we only want the coding section). Frame files are named
// frame_000001.jpg, frame_000002.jpg, ... Returns the frame count.
func extractFrames(mediaPath, outDir, threads, quality string, startSec float64) (int, error) {
	return extractScaled(mediaPath, outDir, "fps=1", threads, quality, startSec)
}
