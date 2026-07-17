# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build ./cmd/vox-caster-bot     # build binary
go test ./...                      # run all tests
go test ./internal/bot -run TestPoll_NewItems -v  # run a single test
go run ./cmd/vox-caster-bot -once  # single poll cycle, then exit (useful for manual testing)
TELEGRAM_TOKEN=x go run ./cmd/vox-caster-bot -validate  # validate config.yaml, then exit
```

CLI flags: `-config <path>` (default `config.yaml`), `-once`, `-validate`.

**CI/CD** (GitHub Actions, Go from `go-version: stable`):
- `ci.yml` (PR + main): tests, golangci-lint (`.golangci.yml`), govulncheck, config validation via `-validate`, Docker build check on PRs
- `release.yml` (`v*` tags) is the only path to prod: tests → linux/amd64 binary + GitHub release with generated notes → `ghcr.io/warpfrog/vox-caster-bot` image (semver + `latest`) → SSH deploy to the VPS (scp `docker-compose.yml`+`config.yaml`, `docker compose pull && up -d`, smoke check). Deploy is skipped until `DEPLOY_HOST`/`DEPLOY_USER`/`DEPLOY_SSH_KEY` (+ optional `DEPLOY_PORT`, `DEPLOY_PATH`, default `/opt/vox-caster-bot`) repo secrets are set. Roll back by re-running the workflow on an old tag: `gh workflow run release.yml --ref vX.Y.Z`

**Docker:**
```bash
TELEGRAM_TOKEN=xxx docker compose up -d
```

## Architecture

Telegram bot that polls MediaWiki RSS feeds and forwards new/updated pages to a Telegram channel with cover images. Single-binary Go app with a polling loop.

**Packages:**
- `cmd/vox-caster-bot` — entrypoint, signal handling, dependency wiring. Builds two `http.Client`s: `wikiClient` (no proxy, for wiki API and feeds) and `telegramClient` (with SOCKS5 proxy from `proxy_url`, for Telegram API)
- `internal/config` — YAML config loading, `TELEGRAM_TOKEN` env var override. Feeds are typed (`new_page` or `update`); an optional per-feed Go `text/template` is compiled at load time into `FeedConfig.Compiled` with funcs from `config.TemplateFuncs`
- `internal/feed` — RSS/Atom fetching via `gofeed` library. Author resolved from `author` → `authors[0]` → `dc:creator`; GUID falls back to link, then title
- `internal/state` — JSON file-backed store of seen item GUIDs per feed with time-based expiry
- `internal/wiki` — MediaWiki API client. Fetches page cover images via `pageimages` prop. URL helpers extract page title and direct URL from diff links
- `internal/telegram` — Telegram Bot API via direct HTTP. Sends photos (`sendPhoto`) with text fallback (`sendMessage`). All message formatting lives here: built-in new-page/update templates and custom template rendering (`FormatMessage` / `MessageData`)
- `internal/bot` — orchestrator: poll feeds → check state → fetch wiki image → format message → send to Telegram

**Key design decisions:**
- Interfaces (`Fetcher`, `Store`, `Client`, `wiki.Client`) used throughout — bot tests use mocks; feed/telegram/wiki tests use `httptest` servers (`telegram.NewClientWithBase` points the client at a test URL). No real network needed
- Feed type selects the built-in message template; both render linked title + author and differ only in header. For `update` feeds the RSS item link is a diff link — `wiki.DirectPageURL` strips `diff`/`oldid` params to recover the canonical page URL
- `update` feeds only post edits to existing pages (`wiki.IsEditURL`: `diff` param + non-zero `oldid`); other items are marked seen without sending. Needed because MediaWiki's `feedrecentchanges` API ignores `hidenewpages`/`hidelog` (those are Special:RecentChanges web-UI params), so page creations arrive in the feed and would be double-posted alongside the `new_page` feed
- Per-feed custom templates (`feeds[].template`, Go `text/template`) receive `telegram.MessageData` (`.Title`, `.Author`, `.Content`, `.Link`, `.PageURL`, `.FeedTitle`, `.Published`); funcs: `html` (escape), `striphtml`. Execution errors fall back to the built-in template. To expose a new field, thread it through `feed.Item` → `MessageData` → `FormatMessage`
- All messages sent with `parse_mode=HTML` — user-derived text must be HTML-escaped
- `config.yaml` is committed and contains no secrets: the token comes only from the `TELEGRAM_TOKEN` env var (`.env` on the host). Deploys ship `config.yaml` to the VPS, so feed/template changes go live with the next release
- Cover images fetched from MediaWiki `pageimages` API; page title extracted from RSS link's `?title=` param. The bot downloads the image bytes and uploads them multipart, so Telegram never needs direct access to the wiki
- `sendPhoto` with automatic fallback to `sendMessage` if photo delivery fails
- First run for a feed marks all existing items as seen without sending (prevents spam on startup)
- Items sent oldest-first (feeds reversed) to preserve chronological order
- On send failure, processing stops for that feed and retries next poll. State saved after each successful send (at-least-once delivery)
- `insecure_skip_verify` config option for wikis with self-signed certificates
- `proxy_url` config option routes Telegram traffic through a proxy (SOCKS5 or HTTP); wiki/feed traffic always goes direct
- `fetch_timeout` (default 300s) and `request_timeout` (default 120s) configurable in YAML; passed into `NewHTTPFetcher` and `wiki.NewClient` respectively
