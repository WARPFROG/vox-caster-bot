package feed

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mmcdole/gofeed"
)

// Item represents a single feed entry.
type Item struct {
	GUID      string
	Title     string
	Content   string
	Link      string
	Published time.Time
	FeedTitle string
	Author    string
}

// Fetcher retrieves and parses RSS/Atom feeds.
type Fetcher interface {
	Fetch(ctx context.Context, url string) ([]Item, error)
}

// HTTPFetcher fetches feeds over HTTP.
type HTTPFetcher struct {
	parser *gofeed.Parser
	client *http.Client
}

func NewHTTPFetcher(insecureSkipVerify bool) *HTTPFetcher {
	client := &http.Client{Timeout: 30 * time.Second}
	if insecureSkipVerify {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}
	return &HTTPFetcher{parser: gofeed.NewParser(), client: client}
}

func (f *HTTPFetcher) Fetch(ctx context.Context, feedURL string) ([]Item, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request for %s: %w", feedURL, err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", feedURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", feedURL, err)
	}

	parsed, err := f.parser.ParseString(string(body))
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", feedURL, err)
	}

	items := make([]Item, 0, len(parsed.Items))
	for _, entry := range parsed.Items {
		item := Item{
			GUID:      itemID(entry),
			Title:     entry.Title,
			Content:   itemContent(entry),
			Link:      entry.Link,
			FeedTitle: parsed.Title,
			Author:    itemAuthor(entry),
		}
		if entry.PublishedParsed != nil {
			item.Published = *entry.PublishedParsed
		} else if entry.UpdatedParsed != nil {
			item.Published = *entry.UpdatedParsed
		}
		items = append(items, item)
	}

	return items, nil
}

func itemID(entry *gofeed.Item) string {
	if entry.GUID != "" {
		return entry.GUID
	}
	if entry.Link != "" {
		return entry.Link
	}
	return entry.Title
}

func itemContent(entry *gofeed.Item) string {
	if entry.Description != "" {
		return entry.Description
	}
	return entry.Content
}

func itemAuthor(entry *gofeed.Item) string {
	if entry.Author != nil && entry.Author.Name != "" {
		return entry.Author.Name
	}
	if len(entry.Authors) > 0 && entry.Authors[0].Name != "" {
		return entry.Authors[0].Name
	}
	// dc:creator is mapped by gofeed into DublinCoreExt
	if entry.DublinCoreExt != nil && len(entry.DublinCoreExt.Creator) > 0 {
		return entry.DublinCoreExt.Creator[0]
	}
	return ""
}
