package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"text/template"

	"rss-bot/internal/config"
	"rss-bot/internal/feed"
)

func TestSend_MessageOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendMessage") {
			t.Errorf("expected sendMessage, got %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("chat_id") != "@test" {
			t.Errorf("chat_id = %q, want %q", r.FormValue("chat_id"), "@test")
		}
		if r.FormValue("parse_mode") != "HTML" {
			t.Errorf("parse_mode = %q, want %q", r.FormValue("parse_mode"), "HTML")
		}
		w.Write([]byte(`{"ok": true}`))
	}))
	defer srv.Close()

	c := NewClientWithBase("testtoken", srv.URL)
	err := c.Send(context.Background(), "@test", Message{Text: "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSend_WithPhoto(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sendPhoto") {
			t.Errorf("expected sendPhoto, got %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.FormValue("photo") != "https://example.com/image.jpg" {
			t.Errorf("photo = %q", r.FormValue("photo"))
		}
		if r.FormValue("caption") != "hello" {
			t.Errorf("caption = %q", r.FormValue("caption"))
		}
		w.Write([]byte(`{"ok": true}`))
	}))
	defer srv.Close()

	c := NewClientWithBase("testtoken", srv.URL)
	err := c.Send(context.Background(), "@test", Message{
		Text:     "hello",
		ImageURL: "https://example.com/image.jpg",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSend_PhotoFallbackToMessage(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if strings.HasSuffix(r.URL.Path, "/sendPhoto") {
			// Simulate photo failure
			w.Write([]byte(`{"ok": false, "description": "Bad Request: wrong file identifier"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/sendMessage") {
			w.Write([]byte(`{"ok": true}`))
			return
		}
		t.Errorf("unexpected path: %s", r.URL.Path)
	}))
	defer srv.Close()

	c := NewClientWithBase("testtoken", srv.URL)
	err := c.Send(context.Background(), "@test", Message{
		Text:     "hello",
		ImageURL: "https://example.com/bad.jpg",
	})
	if err != nil {
		t.Fatalf("unexpected error (should fallback): %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 API calls (photo + message fallback), got %d", calls)
	}
}

func TestSend_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok": false, "description": "Bad Request: chat not found"}`))
	}))
	defer srv.Close()

	c := NewClientWithBase("testtoken", srv.URL)
	err := c.Send(context.Background(), "@nonexistent", Message{Text: "test"})
	if err == nil {
		t.Fatal("expected error for API error response")
	}
}

func TestFormatNewPage(t *testing.T) {
	item := feed.Item{
		Title:  "Test Page",
		Author: "Alice",
	}
	msg := FormatNewPage(item, "https://wiki.example.com/Test_Page")

	if !strings.Contains(msg, "новый перевод") {
		t.Error("missing new page header")
	}
	if !strings.Contains(msg, `<b><a href="https://wiki.example.com/Test_Page">Test Page</a></b>`) {
		t.Errorf("missing bold linked title in:\n%s", msg)
	}
	if !strings.Contains(msg, "👤 <i>Alice</i>") {
		t.Error("missing italic author")
	}
	if !strings.Contains(msg, "#vox #new") {
		t.Error("missing hashtags")
	}
}

func TestFormatUpdate(t *testing.T) {
	item := feed.Item{
		Title:  "Test Page",
		Author: "Bob",
		Link:   "https://wiki.example.com/index.php?title=Test&diff=10&oldid=9",
	}
	msg := FormatUpdate(item, "https://wiki.example.com/index.php?title=Test")

	if !strings.Contains(msg, "обновление перевода") {
		t.Error("missing update header")
	}
	if !strings.Contains(msg, `<b><a href="https://wiki.example.com/index.php?title=Test">Test Page</a></b>`) {
		t.Errorf("missing bold linked title in:\n%s", msg)
	}
	if !strings.Contains(msg, "👤 <i>Bob</i>") {
		t.Error("missing italic author")
	}
	if !strings.Contains(msg, "#vox #update") {
		t.Error("missing hashtags")
	}
}

func TestFormatMessage_PicksTemplate(t *testing.T) {
	item := feed.Item{Title: "X", Author: "Y"}

	newMsg := FormatMessage(nil, config.FeedNewPage, item, "https://example.com")
	if !strings.Contains(newMsg, "новый перевод") {
		t.Error("FeedNewPage should use new page template")
	}

	updMsg := FormatMessage(nil, config.FeedUpdate, item, "https://example.com")
	if !strings.Contains(updMsg, "обновление перевода") {
		t.Error("FeedUpdate should use update template")
	}
}

func TestFormatMessage_CustomTemplate(t *testing.T) {
	tmpl, err := template.New("").Funcs(config.TemplateFuncs).Parse(
		`{{ html .Title }} by {{ html .Author }}`,
	)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	item := feed.Item{Title: "My Page", Author: "Alice"}
	got := FormatMessage(tmpl, config.FeedNewPage, item, "https://example.com")
	if got != "My Page by Alice" {
		t.Errorf("got %q", got)
	}
}

func TestFormatMessage_CustomTemplate_StripHTML(t *testing.T) {
	tmpl, err := template.New("").Funcs(config.TemplateFuncs).Parse(
		`{{ striphtml .Content }}`,
	)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	item := feed.Item{Content: "<p>Hello <b>world</b></p>"}
	got := FormatMessage(tmpl, config.FeedUpdate, item, "")
	if got != "Hello world" {
		t.Errorf("got %q", got)
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"<p>hello</p>", "hello"},
		{"no tags", "no tags"},
		{"<b>bold</b> and <i>italic</i>", "bold and italic"},
		{"", ""},
	}
	for _, tt := range tests {
		got := stripHTML(tt.in)
		if got != tt.want {
			t.Errorf("stripHTML(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
