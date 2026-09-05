package media

import (
	"os"
	"path/filepath"
	"testing"
)

// Short real-network download: first ~60s of the test video, proving
// DownloadVideo + ProbeDuration produce media + info JSON under the project.
func TestShortDownload(t *testing.T) {
	if os.Getenv("NET_TEST") != "1" {
		t.Skip("set NET_TEST=1 to run the real download test")
	}
	dir := filepath.Join("outputs", ".tmp", "mtWN6oPIi1Y")
	_ = os.RemoveAll(dir)
	media, info, err := DownloadVideo(dir, "mtWN6oPIi1Y", "1080", "*0:00-1:00")
	if err != nil {
		t.Fatalf("DownloadVideo: %v", err)
	}
	if info.ID != "mtWN6oPIi1Y" {
		t.Fatalf("info id = %q", info.ID)
	}
	if info.Duration <= 0 {
		t.Fatalf("duration not parsed: %v", info.Duration)
	}
	d, err := ProbeDuration(media)
	if err != nil {
		t.Fatalf("ProbeDuration: %v", err)
	}
	if d <= 0 {
		t.Fatalf("probed duration = %v", d)
	}
	t.Logf("media=%s title=%q duration=%.1fs probed=%.1fs", media, info.Title, info.Duration, d)
}
