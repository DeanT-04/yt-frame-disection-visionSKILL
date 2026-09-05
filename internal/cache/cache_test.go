package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DeanT-04/yt-code-vision-skill/internal/paths"
)

// These tests run against t.TempDir() / a controlled temp CWD only — they must
// never touch the real outputs/ tree.

// TestPacingAndTrash exercises WaitForPacing/MarkDownloaded against a temp
// pacing file by chdir-ing to a temp sandbox.
func TestPacingAndTrash(t *testing.T) {
	oldCacheRoot, oldTrashRoot := cacheRootName, trashRootName
	tmp := t.TempDir()
	// CacheRoot()/TrashRoot() are hardcoded to "outputs/<x>" (project-relative),
	// so run this test from a temp CWD.
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	_ = oldCacheRoot
	_ = oldTrashRoot

	// Fresh: no pacing file -> WaitForPacing returns quickly (nothing to wait).
	if err := WaitForPacing(); err != nil {
		t.Fatalf("WaitForPacing on empty state: %v", err)
	}
	if err := MarkDownloaded(); err != nil {
		t.Fatalf("MarkDownloaded: %v", err)
	}
	// Use a small gap so the test is fast but still exercises the hold.
	if err := os.Setenv("VISION_MIN_DOWNLOAD_GAP", "2"); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Unsetenv("VISION_MIN_DOWNLOAD_GAP") }()
	// Immediately after a mark, pacing must enforce the gap.
	start := time.Now()
	if err := WaitForPacing(); err != nil {
		t.Fatalf("WaitForPacing after mark: %v", err)
	}
	if elapsed := time.Since(start); elapsed < time.Duration(EnvMinDownloadGap()*float64(time.Second))-500*time.Millisecond {
		t.Errorf("WaitForPacing returned after %v; expected ~%.0fs hold", elapsed, EnvMinDownloadGap())
	}

	// Trash: moving a dir must relocate it, not delete it.
	src := filepath.Join(tmp, "dummy-id")
	if err := os.MkdirAll(filepath.Join(src, "frames"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "x.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := MoveToTrash(src); err != nil {
		t.Fatalf("MoveToTrash: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("source still exists after trash: %v", err)
	}
	entries, err := filepath.Glob(filepath.Join(TrashRoot(), "*dummy-id*"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one trashed dummy-id, got %v (%v)", entries, err)
	}
	b, err := os.ReadFile(filepath.Join(entries[0], "x.txt"))
	if err != nil || string(b) != "keep" {
		t.Fatalf("trashed file content lost: %q (%v)", b, err)
	}
	// Trashing a nonexistent dir is a no-op.
	if err := MoveToTrash(filepath.Join(tmp, "nope")); err != nil {
		t.Fatalf("MoveToTrash nonexistent: %v", err)
	}
}

func TestPurgeCacheRemovesBothTrees(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{CacheRoot(), TrashRoot()} {
		if err := os.MkdirAll(filepath.Join(d, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := PurgeCache(); err != nil {
		t.Fatalf("PurgeCache: %v", err)
	}
	for _, d := range []string{CacheRoot(), TrashRoot()} {
		if _, err := os.Stat(d); !os.IsNotExist(err) {
			t.Errorf("PurgeCache left %s behind", d)
		}
	}
}

func TestDoneMarkerRoundTrip(t *testing.T) {
	wd, _ := os.Getwd()
	defer func() { _ = os.Chdir(wd) }()
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}
	id := "mtWN6oPIi1Y"
	if err := os.MkdirAll(paths.IDDir(id), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(DoneMarkerPath(id)); !os.IsNotExist(err) {
		t.Fatal("marker should not exist yet")
	}
	if err := os.WriteFile(DoneMarkerPath(id), []byte("now\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(DoneMarkerPath(id)); err != nil {
		t.Fatalf("marker missing after write: %v", err)
	}
}
