package bot

import (
	"context"
	"errors"
	"testing"

	"vox-caster-bot/internal/config"
	"vox-caster-bot/internal/feed"
	"vox-caster-bot/internal/telegram"
)

// --- Mock fetcher ---

type mockFetcher struct {
	items map[string][]feed.Item
	err   error
}

func (m *mockFetcher) Fetch(_ context.Context, url string) ([]feed.Item, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.items[url], nil
}

// --- Mock state ---

type mockState struct {
	feeds map[string]map[string]bool
	saved int
}

func newMockState() *mockState {
	return &mockState{feeds: make(map[string]map[string]bool)}
}

func (m *mockState) HasFeed(feedURL string) bool {
	_, ok := m.feeds[feedURL]
	return ok
}

func (m *mockState) IsNew(feedURL, itemID string) bool {
	f, ok := m.feeds[feedURL]
	if !ok {
		return true
	}
	return !f[itemID]
}

func (m *mockState) MarkSeen(feedURL, itemID string) {
	if m.feeds[feedURL] == nil {
		m.feeds[feedURL] = make(map[string]bool)
	}
	m.feeds[feedURL][itemID] = true
}

func (m *mockState) Save() error {
	m.saved++
	return nil
}

// --- Mock telegram ---

type mockTelegram struct {
	sent []telegram.Message
	err  error
}

func (m *mockTelegram) Send(_ context.Context, _ string, msg telegram.Message) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, msg)
	return nil
}

// --- Mock wiki ---

type mockWiki struct {
	images    map[string]string
	imageData []byte
	err       error
}

func (m *mockWiki) FetchPageImage(_ context.Context, title string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.images[title], nil
}

func (m *mockWiki) DownloadImage(_ context.Context, _ string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	data := m.imageData
	if data == nil {
		data = []byte("fake-image-data")
	}
	return data, nil
}

// --- Tests ---

func feedURL() string { return "https://example.com/feed" }

func TestPoll_NewItems(t *testing.T) {
	items := []feed.Item{
		{GUID: "1", Title: "First", Link: "https://wiki.example.com/index.php?title=First&diff=1&oldid=0"},
		{GUID: "2", Title: "Second", Link: "https://wiki.example.com/index.php?title=Second&diff=2&oldid=1"},
	}

	st := newMockState()
	st.feeds[feedURL()] = map[string]bool{}

	tg := &mockTelegram{}

	b := &Bot{
		Feeds:     []config.FeedConfig{{URL: feedURL(), Type: config.FeedUpdate}},
		ChannelID: "@test",
		Fetcher:   &mockFetcher{items: map[string][]feed.Item{feedURL(): items}},
		State:     st,
		Telegram:  tg,
		Wiki:      &mockWiki{images: map[string]string{"Second": "https://img/2.jpg"}},
	}

	b.Poll(context.Background())

	if len(tg.sent) != 2 {
		t.Fatalf("sent %d items, want 2", len(tg.sent))
	}
	// Items are reversed: "Second" (GUID 2) sent first, "First" (GUID 1) sent second
	if tg.sent[0].ImageData == nil {
		t.Error("first sent (Second) should have image data")
	}
	if tg.sent[1].ImageData != nil {
		t.Error("second sent (First) should have no image data")
	}
}

func TestPoll_AllSeen(t *testing.T) {
	items := []feed.Item{
		{GUID: "1", Title: "First", Link: "https://example.com/1"},
	}

	st := newMockState()
	st.feeds[feedURL()] = map[string]bool{"1": true}

	tg := &mockTelegram{}

	b := &Bot{
		Feeds:     []config.FeedConfig{{URL: feedURL(), Type: config.FeedUpdate}},
		ChannelID: "@test",
		Fetcher:   &mockFetcher{items: map[string][]feed.Item{feedURL(): items}},
		State:     st,
		Telegram:  tg,
	}

	b.Poll(context.Background())

	if len(tg.sent) != 0 {
		t.Errorf("sent %d items, want 0 (all seen)", len(tg.sent))
	}
}

func TestPoll_FirstRun_MarksSeenWithoutSending(t *testing.T) {
	items := []feed.Item{
		{GUID: "1", Title: "First", Link: "https://example.com/1"},
		{GUID: "2", Title: "Second", Link: "https://example.com/2"},
	}

	st := newMockState()
	tg := &mockTelegram{}

	b := &Bot{
		Feeds:     []config.FeedConfig{{URL: feedURL(), Type: config.FeedNewPage}},
		ChannelID: "@test",
		Fetcher:   &mockFetcher{items: map[string][]feed.Item{feedURL(): items}},
		State:     st,
		Telegram:  tg,
	}

	b.Poll(context.Background())

	if len(tg.sent) != 0 {
		t.Errorf("sent %d items on first run, want 0", len(tg.sent))
	}

	if st.IsNew(feedURL(), "1") {
		t.Error("item 1 should be marked seen")
	}
	if st.IsNew(feedURL(), "2") {
		t.Error("item 2 should be marked seen")
	}
}

func TestPoll_FetchError_ContinuesNextFeed(t *testing.T) {
	goodURL := "https://good.com/feed"
	badURL := "https://bad.com/feed"

	st := newMockState()
	st.feeds[goodURL] = map[string]bool{}

	tg := &mockTelegram{}

	b := &Bot{
		Feeds: []config.FeedConfig{
			{URL: badURL, Type: config.FeedUpdate},
			{URL: goodURL, Type: config.FeedUpdate},
		},
		ChannelID: "@test",
		Fetcher: &selectiveFetcher{
			good: map[string][]feed.Item{
				goodURL: {{GUID: "1", Title: "Good", Link: "https://good.com/1"}},
			},
			bad: map[string]error{
				badURL: errors.New("connection refused"),
			},
		},
		State:    st,
		Telegram: tg,
	}

	b.Poll(context.Background())

	if len(tg.sent) != 1 {
		t.Fatalf("sent %d items, want 1 (good feed only)", len(tg.sent))
	}
}

func TestPoll_SendError_StopsProcessingFeed(t *testing.T) {
	items := []feed.Item{
		{GUID: "1", Title: "First", Link: "https://example.com/1"},
		{GUID: "2", Title: "Second", Link: "https://example.com/2"},
	}

	st := newMockState()
	st.feeds[feedURL()] = map[string]bool{}

	tg := &mockTelegram{err: errors.New("rate limited")}

	b := &Bot{
		Feeds:     []config.FeedConfig{{URL: feedURL(), Type: config.FeedUpdate}},
		ChannelID: "@test",
		Fetcher:   &mockFetcher{items: map[string][]feed.Item{feedURL(): items}},
		State:     st,
		Telegram:  tg,
	}

	b.Poll(context.Background())

	if !st.IsNew(feedURL(), "1") {
		t.Error("item 1 should NOT be marked seen after send failure")
	}
	if !st.IsNew(feedURL(), "2") {
		t.Error("item 2 should NOT be marked seen after send failure")
	}
}

func TestPoll_UsesCorrectTemplate(t *testing.T) {
	st := newMockState()
	st.feeds[feedURL()] = map[string]bool{}

	tg := &mockTelegram{}

	b := &Bot{
		Feeds:     []config.FeedConfig{{URL: feedURL(), Type: config.FeedNewPage}},
		ChannelID: "@test",
		Fetcher: &mockFetcher{items: map[string][]feed.Item{
			feedURL(): {{GUID: "1", Title: "New Thing", Link: "https://wiki.example.com/index.php?title=New_Thing"}},
		}},
		State:    st,
		Telegram: tg,
	}

	b.Poll(context.Background())

	if len(tg.sent) != 1 {
		t.Fatalf("sent %d, want 1", len(tg.sent))
	}
	if msg := tg.sent[0].Text; !containsStr(msg, "новый перевод") {
		t.Errorf("expected new page template, got:\n%s", msg)
	}
}

func TestPoll_WikiError_StillSends(t *testing.T) {
	st := newMockState()
	st.feeds[feedURL()] = map[string]bool{}

	tg := &mockTelegram{}

	b := &Bot{
		Feeds:     []config.FeedConfig{{URL: feedURL(), Type: config.FeedUpdate}},
		ChannelID: "@test",
		Fetcher: &mockFetcher{items: map[string][]feed.Item{
			feedURL(): {{GUID: "1", Title: "Page", Link: "https://wiki.example.com/index.php?title=Page&diff=1&oldid=0"}},
		}},
		State:    st,
		Telegram: tg,
		Wiki:     &mockWiki{err: errors.New("wiki down")},
	}

	b.Poll(context.Background())

	if len(tg.sent) != 1 {
		t.Fatalf("sent %d, want 1 (should send even if wiki image fails)", len(tg.sent))
	}
	if tg.sent[0].ImageData != nil {
		t.Error("image data should be nil on wiki error")
	}
}

// --- Helpers ---

type selectiveFetcher struct {
	good map[string][]feed.Item
	bad  map[string]error
}

func (f *selectiveFetcher) Fetch(_ context.Context, url string) ([]feed.Item, error) {
	if err, ok := f.bad[url]; ok {
		return nil, err
	}
	return f.good[url], nil
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && searchStr(s, sub)
}

func searchStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
