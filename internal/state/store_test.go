package state

import (
	"path/filepath"
	"testing"
	"time"
)

const testMaxAge = 24 * time.Hour

func TestIsNew_EmptyStore(t *testing.T) {
	s, err := NewFileStore(filepath.Join(t.TempDir(), "state.json"), testMaxAge)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !s.IsNew("feed1", "item1") {
		t.Error("expected new item to be new")
	}
}

func TestMarkSeen_ThenNotNew(t *testing.T) {
	s, err := NewFileStore(filepath.Join(t.TempDir(), "state.json"), testMaxAge)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s.MarkSeen("feed1", "item1")

	if s.IsNew("feed1", "item1") {
		t.Error("expected seen item to not be new")
	}
	// Different feed should still be new
	if !s.IsNew("feed2", "item1") {
		t.Error("expected item in different feed to be new")
	}
}

func TestHasFeed(t *testing.T) {
	s, err := NewFileStore(filepath.Join(t.TempDir(), "state.json"), testMaxAge)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if s.HasFeed("feed1") {
		t.Error("expected HasFeed=false for unknown feed")
	}

	s.MarkSeen("feed1", "item1")

	if !s.HasFeed("feed1") {
		t.Error("expected HasFeed=true after MarkSeen")
	}
}

func TestSaveLoad_Roundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	s1, err := NewFileStore(path, testMaxAge)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s1.MarkSeen("feed1", "a")
	s1.MarkSeen("feed1", "b")
	s1.MarkSeen("feed2", "x")

	if err := s1.Save(); err != nil {
		t.Fatalf("save error: %v", err)
	}

	s2, err := NewFileStore(path, testMaxAge)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if s2.IsNew("feed1", "a") {
		t.Error("expected 'a' to be seen after reload")
	}
	if s2.IsNew("feed1", "b") {
		t.Error("expected 'b' to be seen after reload")
	}
	if s2.IsNew("feed2", "x") {
		t.Error("expected 'x' to be seen after reload")
	}
	if !s2.IsNew("feed1", "c") {
		t.Error("expected 'c' to still be new")
	}
}

func TestExpiry_PurgesOldItems(t *testing.T) {
	s, err := NewFileStore(filepath.Join(t.TempDir(), "state.json"), 24*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s.MarkSeen("feed1", "old-item")
	s.MarkSeen("feed1", "new-item")

	// Backdate the first item beyond the maxAge window
	fs := s.(*fileStore).data.Feeds["feed1"]
	fs.Seen[0].SeenAt = time.Now().Add(-48 * time.Hour)

	s.(*fileStore).purge()

	if !s.IsNew("feed1", "old-item") {
		t.Error("old item should have been purged")
	}
	if s.IsNew("feed1", "new-item") {
		t.Error("new item should still be seen")
	}
}

func TestExpiry_PurgedOnLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	s1, err := NewFileStore(path, 24*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s1.MarkSeen("feed1", "stale")
	s1.MarkSeen("feed1", "fresh")

	// Backdate "stale" before saving
	fs := s1.(*fileStore).data.Feeds["feed1"]
	fs.Seen[0].SeenAt = time.Now().Add(-48 * time.Hour)

	if err := s1.Save(); err != nil {
		t.Fatalf("save error: %v", err)
	}

	s2, err := NewFileStore(path, 24*time.Hour)
	if err != nil {
		t.Fatalf("load error: %v", err)
	}

	if !s2.IsNew("feed1", "stale") {
		t.Error("stale item should have been purged on load")
	}
	if s2.IsNew("feed1", "fresh") {
		t.Error("fresh item should still be seen after load")
	}
}

func TestMarkSeen_Idempotent(t *testing.T) {
	s, err := NewFileStore(filepath.Join(t.TempDir(), "state.json"), testMaxAge)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	s.MarkSeen("feed1", "item1")
	s.MarkSeen("feed1", "item1")
	s.MarkSeen("feed1", "item1")

	fs := s.(*fileStore).data.Feeds["feed1"]
	if len(fs.Seen) != 1 {
		t.Errorf("seen list length = %d, want 1 (should be idempotent)", len(fs.Seen))
	}
}
