package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
telegram_token: "tok123"
channel_id: "@test"
poll_interval: "10m"
state_path: "/tmp/state.json"
wiki_api: "https://wiki.example.com/api.php"
insecure_skip_verify: true
feeds:
  - url: "https://example.com/feed"
    type: "new_page"
  - url: "https://example.com/changes"
    type: "update"
`), 0o644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.TelegramToken != "tok123" {
		t.Errorf("token = %q, want %q", cfg.TelegramToken, "tok123")
	}
	if cfg.ChannelID != "@test" {
		t.Errorf("channel = %q, want %q", cfg.ChannelID, "@test")
	}
	if cfg.PollInterval != 10*time.Minute {
		t.Errorf("interval = %v, want %v", cfg.PollInterval, 10*time.Minute)
	}
	if cfg.StatePath != "/tmp/state.json" {
		t.Errorf("state_path = %q, want %q", cfg.StatePath, "/tmp/state.json")
	}
	if cfg.WikiAPI != "https://wiki.example.com/api.php" {
		t.Errorf("wiki_api = %q, want %q", cfg.WikiAPI, "https://wiki.example.com/api.php")
	}
	if !cfg.InsecureSkipVerify {
		t.Error("insecure_skip_verify = false, want true")
	}
	if len(cfg.Feeds) != 2 {
		t.Fatalf("feeds count = %d, want 2", len(cfg.Feeds))
	}
	if cfg.Feeds[0].Type != FeedNewPage {
		t.Errorf("feeds[0].type = %q, want %q", cfg.Feeds[0].Type, FeedNewPage)
	}
	if cfg.Feeds[1].Type != FeedUpdate {
		t.Errorf("feeds[1].type = %q, want %q", cfg.Feeds[1].Type, FeedUpdate)
	}
}

func TestLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
telegram_token: "tok"
channel_id: "@ch"
feeds:
  - url: "https://example.com/feed"
    type: "new_page"
`), 0o644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.PollInterval != 5*time.Minute {
		t.Errorf("default interval = %v, want %v", cfg.PollInterval, 5*time.Minute)
	}
	if cfg.StatePath != "state.json" {
		t.Errorf("default state_path = %q, want %q", cfg.StatePath, "state.json")
	}
	if cfg.InsecureSkipVerify {
		t.Error("default insecure_skip_verify should be false")
	}
}

func TestLoad_EnvOverridesToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
telegram_token: "file_token"
channel_id: "@ch"
feeds:
  - url: "https://example.com/feed"
    type: "update"
`), 0o644)

	t.Setenv("TELEGRAM_TOKEN", "env_token")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.TelegramToken != "env_token" {
		t.Errorf("token = %q, want %q", cfg.TelegramToken, "env_token")
	}
}

func TestLoad_MissingToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
channel_id: "@ch"
feeds:
  - url: "https://example.com/feed"
    type: "new_page"
`), 0o644)

	t.Setenv("TELEGRAM_TOKEN", "")

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing token")
	}
}

func TestLoad_MissingFeeds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
telegram_token: "tok"
channel_id: "@ch"
`), 0o644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing feeds")
	}
}

func TestLoad_InvalidFeedType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
telegram_token: "tok"
channel_id: "@ch"
feeds:
  - url: "https://example.com/feed"
    type: "bogus"
`), 0o644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid feed type")
	}
}

func TestLoad_InvalidFile(t *testing.T) {
	_, err := Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoad_FeedTemplate_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
telegram_token: "tok"
channel_id: "@ch"
feeds:
  - url: "https://example.com/feed"
    type: "new_page"
    template: "{{ html .Title }} by {{ html .Author }}"
`), 0o644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Feeds[0].Compiled == nil {
		t.Fatal("expected compiled template, got nil")
	}
}

func TestLoad_FeedTemplate_Invalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
telegram_token: "tok"
channel_id: "@ch"
feeds:
  - url: "https://example.com/feed"
    type: "new_page"
    template: "{{ .Title"
`), 0o644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid template syntax")
	}
}

func TestLoad_FeedTemplate_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(`
telegram_token: "tok"
channel_id: "@ch"
feeds:
  - url: "https://example.com/feed"
    type: "update"
`), 0o644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Feeds[0].Compiled != nil {
		t.Error("expected nil compiled template when no template configured")
	}
}
