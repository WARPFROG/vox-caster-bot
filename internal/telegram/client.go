package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"text/template"
	"time"

	"rss-bot/internal/config"
	"rss-bot/internal/feed"
)

const defaultAPIBase = "https://api.telegram.org"

// Message holds everything needed to send a Telegram post.
type Message struct {
	Text      string
	ImageURL  string // used only if ImageData is nil (Telegram fetches the URL)
	ImageData []byte // if non-nil, uploaded as file to Telegram
}

// Client sends messages to a Telegram channel.
type Client interface {
	Send(ctx context.Context, channelID string, msg Message) error
}

type httpClient struct {
	token   string
	apiBase string
	http    *http.Client
}

func NewClient(token string) Client {
	return &httpClient{
		token:   token,
		apiBase: defaultAPIBase,
		http:    &http.Client{},
	}
}

// NewClientWithBase creates a client pointing at a custom API base URL (for testing).
func NewClientWithBase(token, apiBase string) Client {
	return &httpClient{
		token:   token,
		apiBase: apiBase,
		http:    &http.Client{},
	}
}

func (c *httpClient) Send(ctx context.Context, channelID string, msg Message) error {
	if msg.ImageData != nil {
		err := c.sendPhotoUpload(ctx, channelID, msg.ImageData, msg.Text)
		if err == nil {
			return nil
		}
		// Fallback to text-only on upload failure
	} else if msg.ImageURL != "" {
		err := c.sendPhoto(ctx, channelID, msg.ImageURL, msg.Text)
		if err == nil {
			return nil
		}
		// Fallback to text-only on photo URL failure
	}
	return c.sendMessage(ctx, channelID, msg.Text)
}

func (c *httpClient) sendMessage(ctx context.Context, channelID, text string) error {
	params := url.Values{
		"chat_id":    {channelID},
		"text":       {text},
		"parse_mode": {"HTML"},
	}

	return c.apiCall(ctx, "sendMessage", params)
}

func (c *httpClient) sendPhoto(ctx context.Context, channelID, photoURL, caption string) error {
	params := url.Values{
		"chat_id":    {channelID},
		"photo":      {photoURL},
		"caption":    {caption},
		"parse_mode": {"HTML"},
	}

	return c.apiCall(ctx, "sendPhoto", params)
}

func (c *httpClient) sendPhotoUpload(ctx context.Context, channelID string, imageData []byte, caption string) error {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	w.WriteField("chat_id", channelID)
	w.WriteField("caption", caption)
	w.WriteField("parse_mode", "HTML")

	part, err := w.CreateFormFile("photo", "cover.jpg")
	if err != nil {
		return fmt.Errorf("create form file: %w", err)
	}
	if _, err := io.Copy(part, bytes.NewReader(imageData)); err != nil {
		return fmt.Errorf("write image data: %w", err)
	}
	w.Close()

	endpoint := fmt.Sprintf("%s/bot%s/sendPhoto", c.apiBase, c.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("sendPhoto upload: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	if !result.OK {
		return fmt.Errorf("telegram API error: %s", result.Description)
	}
	return nil
}

func (c *httpClient) apiCall(ctx context.Context, method string, params url.Values) error {
	endpoint := fmt.Sprintf("%s/bot%s/%s", c.apiBase, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	defer resp.Body.Close()

	var result struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if !result.OK {
		return fmt.Errorf("telegram API error: %s", result.Description)
	}

	return nil
}

// FormatNewPage builds a caption/message for a newly created wiki page.
func FormatNewPage(item feed.Item, pageURL string) string {
	var b strings.Builder
	b.WriteString("🔥 Warpopedia — новый перевод\n\n")
	if pageURL != "" {
		b.WriteString(fmt.Sprintf("<b><a href=\"%s\">%s</a></b>\n", escapeHTML(pageURL), escapeHTML(item.Title)))
	} else {
		b.WriteString("<b>")
		b.WriteString(escapeHTML(item.Title))
		b.WriteString("</b>\n")
	}
	if item.Author != "" {
		b.WriteString("👤 <i>")
		b.WriteString(escapeHTML(item.Author))
		b.WriteString("</i>\n")
	}
	b.WriteString("\n🏷 #vox #new")
	return b.String()
}

// FormatUpdate builds a caption/message for a wiki page update.
func FormatUpdate(item feed.Item, pageURL string) string {
	var b strings.Builder
	b.WriteString("📢 Warpopedia — обновление перевода\n\n")
	if pageURL != "" {
		b.WriteString(fmt.Sprintf("<b><a href=\"%s\">%s</a></b>\n", escapeHTML(pageURL), escapeHTML(item.Title)))
	} else {
		b.WriteString("<b>")
		b.WriteString(escapeHTML(item.Title))
		b.WriteString("</b>\n")
	}
	if item.Author != "" {
		b.WriteString("👤 <i>")
		b.WriteString(escapeHTML(item.Author))
		b.WriteString("</i>\n")
	}
	b.WriteString("\n🏷 #vox #update")
	return b.String()
}

// MessageData is the data passed to a custom Go template when formatting a message.
type MessageData struct {
	Title     string
	Author    string
	Content   string    // raw HTML content from the feed item
	Link      string    // RSS item link (diff link for updates)
	PageURL   string    // direct page URL
	FeedTitle string
	Published time.Time
}

// FormatMessage formats a feed item as a Telegram message text.
// If tmpl is non-nil it is executed with MessageData; on error it falls back to
// the built-in template for the given feedType.
func FormatMessage(tmpl *template.Template, feedType config.FeedType, item feed.Item, pageURL string) string {
	if tmpl != nil {
		data := MessageData{
			Title:     item.Title,
			Author:    item.Author,
			Content:   item.Content,
			Link:      item.Link,
			PageURL:   pageURL,
			FeedTitle: item.FeedTitle,
			Published: item.Published,
		}
		var buf strings.Builder
		if err := tmpl.Execute(&buf, data); err != nil {
			log.Printf("template execution error: %v", err)
		} else {
			return buf.String()
		}
	}
	switch feedType {
	case config.FeedNewPage:
		return FormatNewPage(item, pageURL)
	default:
		return FormatUpdate(item, pageURL)
	}
}

func stripHTML(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
