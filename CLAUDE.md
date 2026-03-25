# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build ./cmd/rss-bot            # build binary
go test ./...                      # run all tests
go test ./internal/bot -run TestPoll_NewItems -v  # run a single test
```

**Docker:**
```bash
TELEGRAM_TOKEN=xxx docker compose up --build
```

## Architecture

Telegram bot that polls MediaWiki RSS feeds and forwards new/updated pages to a Telegram channel with cover images. Single-binary Go app with a polling loop.

**Packages:**
- `cmd/rss-bot` — entrypoint, signal handling, dependency wiring
- `internal/config` — YAML config loading, `TELEGRAM_TOKEN` env var override. Feeds are typed (`new_page` or `update`) for different message templates
- `internal/feed` — RSS/Atom fetching via `gofeed` library with custom HTTP client (for TLS skip). Extracts `dc:creator` as Author
- `internal/state` — JSON file-backed store of seen item GUIDs per feed. Capped at 500 per feed
- `internal/wiki` — MediaWiki API client. Fetches page cover images via `pageimages` prop. URL helpers extract page title and direct URL from diff links
- `internal/telegram` — Telegram Bot API via direct HTTP. Sends photos (`sendPhoto`) with text fallback (`sendMessage`). Separate format templates for new pages vs updates
- `internal/bot` — orchestrator: poll feeds → check state → fetch wiki image → format message → send to Telegram

**Key design decisions:**
- Interfaces (`Fetcher`, `Store`, `Client`, `wiki.Client`) used throughout — bot tests use mocks, no network needed
- Two feed types with different templates: `new_page` (title + author + link) and `update` (title + author + edit summary + page link + diff link)
- Cover images fetched from MediaWiki `pageimages` API; page title extracted from RSS link's `?title=` param
- `sendPhoto` with automatic fallback to `sendMessage` if photo delivery fails
- First run for a feed marks all existing items as seen without sending (prevents spam on startup)
- Items sent oldest-first (feeds reversed) to preserve chronological order
- On send failure, processing stops for that feed and retries next poll. State saved after each successful send (at-least-once delivery)
- `insecure_skip_verify` config option for wikis with self-signed certificates
