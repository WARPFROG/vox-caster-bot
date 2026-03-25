package feed

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testRSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>Test Feed</title>
    <item>
      <guid>1</guid>
      <title>First Post</title>
      <description>Hello world</description>
      <link>https://example.com/1</link>
      <pubDate>Mon, 01 Jan 2024 00:00:00 UTC</pubDate>
      <dc:creator>Alice</dc:creator>
    </item>
    <item>
      <guid>2</guid>
      <title>Second Post</title>
      <description>&lt;p&gt;Some HTML&lt;/p&gt;</description>
      <link>https://example.com/2</link>
      <pubDate>Tue, 02 Jan 2024 00:00:00 UTC</pubDate>
      <dc:creator>Bob</dc:creator>
    </item>
  </channel>
</rss>`

const testAtom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Atom Feed</title>
  <entry>
    <id>atom-1</id>
    <title>Atom Entry</title>
    <summary>Atom summary</summary>
    <link href="https://example.com/atom/1"/>
    <updated>2024-01-01T00:00:00Z</updated>
    <author><name>Charlie</name></author>
  </entry>
</feed>`

func TestFetch_RSS(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(testRSS))
	}))
	defer srv.Close()

	f := NewHTTPFetcher(false)
	items, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("got %d items, want 2", len(items))
	}

	if items[0].GUID != "1" {
		t.Errorf("item[0].GUID = %q, want %q", items[0].GUID, "1")
	}
	if items[0].Title != "First Post" {
		t.Errorf("item[0].Title = %q, want %q", items[0].Title, "First Post")
	}
	if items[0].Content != "Hello world" {
		t.Errorf("item[0].Content = %q, want %q", items[0].Content, "Hello world")
	}
	if items[0].Link != "https://example.com/1" {
		t.Errorf("item[0].Link = %q, want %q", items[0].Link, "https://example.com/1")
	}
	if items[0].FeedTitle != "Test Feed" {
		t.Errorf("item[0].FeedTitle = %q, want %q", items[0].FeedTitle, "Test Feed")
	}
	if items[0].Author != "Alice" {
		t.Errorf("item[0].Author = %q, want %q", items[0].Author, "Alice")
	}
	if items[0].Published.IsZero() {
		t.Error("item[0].Published is zero")
	}
	if items[1].Author != "Bob" {
		t.Errorf("item[1].Author = %q, want %q", items[1].Author, "Bob")
	}
}

func TestFetch_Atom(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(testAtom))
	}))
	defer srv.Close()

	f := NewHTTPFetcher(false)
	items, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}

	if items[0].GUID != "atom-1" {
		t.Errorf("GUID = %q, want %q", items[0].GUID, "atom-1")
	}
	if items[0].FeedTitle != "Atom Feed" {
		t.Errorf("FeedTitle = %q, want %q", items[0].FeedTitle, "Atom Feed")
	}
	if items[0].Author != "Charlie" {
		t.Errorf("Author = %q, want %q", items[0].Author, "Charlie")
	}
}

func TestFetch_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "broken", http.StatusInternalServerError)
	}))
	defer srv.Close()

	f := NewHTTPFetcher(false)
	_, err := f.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestFetch_InvalidXML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not xml at all"))
	}))
	defer srv.Close()

	f := NewHTTPFetcher(false)
	_, err := f.Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}
