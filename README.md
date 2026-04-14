# Vox Caster Bot

<img width="256" height="256" alt="vox-caster-art" src="https://github.com/user-attachments/assets/6173c6eb-f602-4d0d-8442-3ffcac8a01bc" />

Vibe-coded Telegram bot that polls MediaWiki RSS feeds and forwards new or updated pages to a Telegram channel with cover images.

## Features

- Polls RSS/Atom feeds on a configurable interval
- Two built-in message templates: **new page** and **page update**
- Custom [Go templates](#custom-templates) per feed
- Cover images fetched from the MediaWiki `pageimages` API
- Cover image upload with automatic text-only fallback
- First-run safety: existing items are marked as seen without sending
- Time-based state expiry (default 30 days)
- One-shot mode for testing and cron-based scheduling
- TLS skip option for wikis with self-signed certificates

## Quick Start

```bash
cp config.example.yaml config.yaml
# Edit config.yaml with your Telegram bot token, channel, and feed URLs

go build ./cmd/vox-caster-bot
./vox-caster-bot
```

### Docker

```bash
cp config.yaml /path/to/deploy/
cd /path/to/deploy
TELEGRAM_TOKEN=your_token docker compose up -d
```

`state.json` is saved in the same directory as `docker-compose.yml` automatically.

Pull the image manually:

```bash
docker pull ghcr.io/warpfrog/vox-caster-bot:latest
```

## Configuration

```yaml
telegram_token: "YOUR_BOT_TOKEN"     # or set TELEGRAM_TOKEN env var
channel_id: "@yourchannel"
poll_interval: "5m"                  # default: 5m
state_path: "state.json"             # default: state.json
state_max_age: "720h"                # default: 720h (30 days)
wiki_api: "https://wiki.example.com/api.php"  # optional, enables cover images
insecure_skip_verify: false          # skip TLS verification
# proxy_url: "http://proxy:8080"   # optional; http://, https://, socks5://

feeds:
  - url: "https://wiki.example.com/index.php?title=Special:NewPages&feed=rss"
    type: "new_page"
  - url: "https://wiki.example.com/api.php?action=feedrecentchanges&feedformat=rss"
    type: "update"
```

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `telegram_token` | yes | — | Bot API token (env `TELEGRAM_TOKEN` overrides) |
| `channel_id` | yes | — | Target Telegram channel (e.g. `@mychannel`) |
| `poll_interval` | no | `5m` | Go duration between polls |
| `state_path` | no | `state.json` | Path to the state file |
| `state_max_age` | no | `720h` | How long to remember seen items |
| `wiki_api` | no | — | MediaWiki API endpoint for cover images |
| `insecure_skip_verify` | no | `false` | Skip TLS certificate verification |
| `proxy_url` | no | — | Proxy URL (`http://`, `https://`, `socks5://`). Falls back to `HTTP_PROXY`/`HTTPS_PROXY` env vars |
| `feeds[].url` | yes | — | RSS/Atom feed URL |
| `feeds[].type` | yes | — | `new_page` or `update` |
| `feeds[].template` | no | — | Custom Go template string |

## CLI Flags

```
./vox-caster-bot [flags]
  -config string   path to config file (default "config.yaml")
  -once            poll once and exit
```

## Custom Templates

Each feed can define a custom Go `text/template` that overrides the built-in formatter. The template receives a `MessageData` struct:

| Field | Type | Description |
|-------|------|-------------|
| `.Title` | string | Page title |
| `.Author` | string | Author / creator name |
| `.Content` | string | Raw HTML content from the feed |
| `.Link` | string | RSS item link (diff link for updates) |
| `.PageURL` | string | Direct page URL |
| `.FeedTitle` | string | Feed channel title |
| `.Published` | time.Time | Publication timestamp |

Available template functions:

| Function | Description |
|----------|-------------|
| `html` | HTML-escape a string (built-in) |
| `striphtml` | Strip HTML tags from a string |

Example:

```yaml
feeds:
  - url: "https://wiki.example.com/feed"
    type: "new_page"
    template: |
      <b><a href="{{ html .PageURL }}">{{ html .Title }}</a></b>
      by {{ html .Author }}
```

Messages use Telegram's [HTML parse mode](https://core.telegram.org/bots/api#html-style) — supported tags: `<b>`, `<i>`, `<a>`, `<code>`, `<pre>`.

## Architecture

```
cmd/vox-caster-bot/         Entrypoint, signal handling, dependency wiring
internal/
  config/            YAML config loading and validation
  feed/              RSS/Atom fetching via gofeed
  state/             JSON file-backed store of seen item GUIDs
  wiki/              MediaWiki API client (page images, URL helpers)
  telegram/          Telegram Bot API (sendPhoto / sendMessage)
  bot/               Orchestrator: poll -> check state -> fetch image -> send
```

### Processing Flow

1. For each configured feed, fetch the RSS/Atom items
2. Reverse items to send oldest first (preserves chronological order)
3. For each unseen item:
   - Extract page title from the item link
   - Format the message text (custom template or built-in)
   - Fetch cover image from MediaWiki API (if `wiki_api` configured)
   - Send cover image to Telegram (falls back to text-only on failure)
   - Mark item as seen, save state immediately
4. On first run for a feed, all items are marked as seen without sending

### Error Handling

- **Feed fetch error**: logged, skipped, other feeds continue
- **Send error**: state is saved, processing stops for that feed (retries next poll)
- **Image fetch error**: message sent without image
- **At-least-once delivery**: state saved after each successful send

## Development

```bash
go test ./...                                    # all tests
go test ./internal/bot -run TestPoll_NewItems -v # single test
go build ./cmd/vox-caster-bot                           # build binary
```

All major components use interfaces — tests use mocks with no network calls required.

## License

MIT
