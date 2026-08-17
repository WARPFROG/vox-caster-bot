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
	// MarkSeen remembers an item, refreshing the timestamp of one that is
	// already known.
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
	Seen []seenItem     `json:"seen"`
	idx  map[string]int // item ID -> position in Seen
}

type seenItem struct {
	ID     string    `json:"id"`
	SeenAt time.Time `json:"seen_at"`
}

// NewFileStore creates a store backed by a JSON file.
// Items not seen for maxAge are discarded on load and save.
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

// HasFeed reports whether the feed has at least one remembered item. A feed
// whose items have all expired must count as unknown: treating the bare key
// as "known" would make every current item look new and flood the channel
// with the feed's entire backlog.
func (s *fileStore) HasFeed(feedURL string) bool {
	fs, ok := s.data.Feeds[feedURL]
	return ok && len(fs.idx) > 0
}

func (s *fileStore) IsNew(feedURL, itemID string) bool {
	fs, ok := s.data.Feeds[feedURL]
	if !ok {
		return true
	}
	_, seen := fs.idx[itemID]
	return !seen
}

// MarkSeen records an item as sent, refreshing the timestamp if it is already
// known. Feeds capped by item count (Special:NewPages) keep entries for months,
// far longer than maxAge; without the refresh such an item expires out of the
// state while still listed, looks new again and gets re-posted.
func (s *fileStore) MarkSeen(feedURL, itemID string) {
	fs, ok := s.data.Feeds[feedURL]
	if !ok {
		fs = &feedState{idx: make(map[string]int)}
		s.data.Feeds[feedURL] = fs
	}

	if i, exists := fs.idx[itemID]; exists {
		fs.Seen[i].SeenAt = time.Now()
		return
	}

	fs.idx[itemID] = len(fs.Seen)
	fs.Seen = append(fs.Seen, seenItem{ID: itemID, SeenAt: time.Now()})
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
		fs.reindex()
	}

	s.purge()
	return nil
}

// purge drops entries not seen for maxAge from all feeds. Items still listed in
// a feed are refreshed on every poll, so only ones that dropped out of the feed
// age out.
func (s *fileStore) purge() {
	cutoff := time.Now().Add(-s.maxAge)
	for _, fs := range s.data.Feeds {
		kept := fs.Seen[:0]
		for _, item := range fs.Seen {
			if item.SeenAt.After(cutoff) {
				kept = append(kept, item)
			}
		}
		fs.Seen = kept
		fs.reindex()
	}
}

// reindex rebuilds the ID lookup after Seen has been rewritten.
func (fs *feedState) reindex() {
	fs.idx = make(map[string]int, len(fs.Seen))
	for i, item := range fs.Seen {
		fs.idx[item.ID] = i
	}
}
