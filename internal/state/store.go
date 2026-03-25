package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Store tracks which feed items have already been sent.
type Store interface {
	HasFeed(feedURL string) bool
	IsNew(feedURL, itemID string) bool
	MarkSeen(feedURL, itemID string)
	Save() error
}

type fileStore struct {
	path   string
	maxAge time.Duration
	data   storeData
}

type storeData struct {
	Feeds map[string]*feedState `json:"feeds"`
}

type feedState struct {
	Seen []seenItem `json:"seen"`
	set  map[string]struct{}
}

type seenItem struct {
	ID     string    `json:"id"`
	SeenAt time.Time `json:"seen_at"`
}

// NewFileStore creates a store backed by a JSON file.
// Items older than maxAge are discarded on load and save.
// If the file exists, state is loaded from it.
func NewFileStore(path string, maxAge time.Duration) (Store, error) {
	s := &fileStore{
		path:   path,
		maxAge: maxAge,
		data:   storeData{Feeds: make(map[string]*feedState)},
	}

	if _, err := os.Stat(path); err == nil {
		if err := s.load(); err != nil {
			return nil, err
		}
	}

	return s, nil
}

func (s *fileStore) HasFeed(feedURL string) bool {
	_, ok := s.data.Feeds[feedURL]
	return ok
}

func (s *fileStore) IsNew(feedURL, itemID string) bool {
	fs, ok := s.data.Feeds[feedURL]
	if !ok {
		return true
	}
	_, seen := fs.set[itemID]
	return !seen
}

func (s *fileStore) MarkSeen(feedURL, itemID string) {
	fs, ok := s.data.Feeds[feedURL]
	if !ok {
		fs = &feedState{set: make(map[string]struct{})}
		s.data.Feeds[feedURL] = fs
	}

	if _, exists := fs.set[itemID]; exists {
		return
	}

	fs.Seen = append(fs.Seen, seenItem{ID: itemID, SeenAt: time.Now()})
	fs.set[itemID] = struct{}{}
}

func (s *fileStore) Save() error {
	s.purge()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0o644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}

	return nil
}

func (s *fileStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return fmt.Errorf("read state: %w", err)
	}

	if err := json.Unmarshal(data, &s.data); err != nil {
		return fmt.Errorf("parse state: %w", err)
	}

	if s.data.Feeds == nil {
		s.data.Feeds = make(map[string]*feedState)
	}

	for _, fs := range s.data.Feeds {
		fs.set = make(map[string]struct{}, len(fs.Seen))
		for _, item := range fs.Seen {
			fs.set[item.ID] = struct{}{}
		}
	}

	s.purge()
	return nil
}

// purge removes entries older than maxAge from all feeds.
func (s *fileStore) purge() {
	cutoff := time.Now().Add(-s.maxAge)
	for _, fs := range s.data.Feeds {
		kept := fs.Seen[:0]
		for _, item := range fs.Seen {
			if item.SeenAt.After(cutoff) {
				kept = append(kept, item)
			} else {
				delete(fs.set, item.ID)
			}
		}
		fs.Seen = kept
	}
}
