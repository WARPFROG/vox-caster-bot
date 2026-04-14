package wiki

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Client fetches metadata from a MediaWiki API.
type Client interface {
	FetchPageImage(ctx context.Context, pageTitle string) (imageURL string, err error)
	DownloadImage(ctx context.Context, imageURL string) ([]byte, error)
}

type httpClient struct {
	apiBase string
	http    *http.Client
}

const requestTimeout = 15 * time.Second

func NewClient(apiBase string, client *http.Client) Client {
	return &httpClient{apiBase: apiBase, http: client}
}

func (c *httpClient) FetchPageImage(ctx context.Context, pageTitle string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	params := url.Values{
		"action":      {"query"},
		"titles":      {pageTitle},
		"prop":        {"pageimages"},
		"format":      {"json"},
		"pithumbsize": {"800"},
	}

	endpoint := c.apiBase + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", "vox-caster-bot/1.0 (https://github.com/WARPFROG/vox-caster-bot)")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch page image for %q: %w", pageTitle, err)
	}
	defer resp.Body.Close()

	var result queryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	for _, page := range result.Query.Pages {
		if page.Thumbnail.Source != "" {
			return page.Thumbnail.Source, nil
		}
	}

	return "", nil // no image found
}

func (c *httpClient) DownloadImage(ctx context.Context, imageURL string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download image: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read image: %w", err)
	}

	return data, nil
}

type queryResult struct {
	Query struct {
		Pages map[string]struct {
			Thumbnail struct {
				Source string `json:"source"`
			} `json:"thumbnail"`
		} `json:"pages"`
	} `json:"query"`
}

// PageTitleFromURL extracts the page title from a MediaWiki URL
// (e.g. "index.php?title=Some_Page&diff=123&oldid=456" -> "Some_Page").
func PageTitleFromURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Query().Get("title")
}

// DirectPageURL returns the canonical page URL by stripping diff/oldid params.
func DirectPageURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	title := u.Query().Get("title")
	if title == "" {
		return rawURL
	}

	q := url.Values{"title": {title}}
	u.RawQuery = q.Encode()
	return u.String()
}
