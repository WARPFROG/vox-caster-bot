package bot

import (
	"context"
	"log"
	"time"

	"vox-caster-bot/internal/config"
	"vox-caster-bot/internal/feed"
	"vox-caster-bot/internal/state"
	"vox-caster-bot/internal/telegram"
	"vox-caster-bot/internal/wiki"
)

// maxSendsPerPoll caps how many items a single feed may post per poll cycle.
// Anything beyond the cap stays unseen and goes out on following cycles, so a
// backlog drains gradually instead of flooding the channel and hitting
// Telegram rate limits.
const maxSendsPerPoll = 10

// Bot polls RSS feeds and sends new items to Telegram.
type Bot struct {
	Feeds     []config.FeedConfig
	ChannelID string
	Interval  time.Duration
	Fetcher   feed.Fetcher
	State     state.Store
	Telegram  telegram.Client
	Wiki      wiki.Client // optional, may be nil
}

// Run starts the polling loop, blocking until ctx is cancelled.
func (b *Bot) Run(ctx context.Context) error {
	log.Println("starting bot, polling every", b.Interval)

	// Poll immediately on start
	b.Poll(ctx)

	ticker := time.NewTicker(b.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("shutting down")
			return nil
		case <-ticker.C:
			b.Poll(ctx)
		}
	}
}

// Poll checks all feeds once and sends new items.
func (b *Bot) Poll(ctx context.Context) {
	for _, fc := range b.Feeds {
		if err := b.processFeed(ctx, fc); err != nil {
			log.Printf("error processing %s: %v", fc.URL, err)
		}
	}
}

func (b *Bot) processFeed(ctx context.Context, fc config.FeedConfig) error {
	items, err := b.Fetcher.Fetch(ctx, fc.URL)
	if err != nil {
		return err
	}

	firstRun := !b.State.HasFeed(fc.URL)

	// Reverse to send oldest first (feeds typically list newest first)
	reversed := make([]feed.Item, len(items))
	for i, item := range items {
		reversed[len(items)-1-i] = item
	}

	sent := 0
	for _, item := range reversed {
		if !b.State.IsNew(fc.URL, item.GUID) {
			continue
		}

		if firstRun {
			b.State.MarkSeen(fc.URL, item.GUID)
			continue
		}

		if sent == maxSendsPerPoll {
			log.Printf("feed %s: sent %d items this poll, deferring the rest to the next cycle", fc.URL, sent)
			break
		}

		// feedrecentchanges ignores hidenewpages/hidelog (those are
		// Special:RecentChanges web-UI params), so the update feed also
		// carries page creations and log entries. Drop everything that is
		// not an edit to an existing page — creations are already covered
		// by the new_page feed.
		if fc.Type == config.FeedUpdate && !wiki.IsEditURL(item.Link) {
			log.Printf("update feed: skipping non-edit item %q (%s)", item.Title, item.Link)
			b.State.MarkSeen(fc.URL, item.GUID)
			if err := b.State.Save(); err != nil {
				log.Printf("error saving state: %v", err)
			}
			continue
		}

		msg := b.buildMessage(ctx, fc, item)

		if err := b.Telegram.Send(ctx, b.ChannelID, msg); err != nil {
			if saveErr := b.State.Save(); saveErr != nil {
				log.Printf("error saving state: %v", saveErr)
			}
			return err
		}

		b.State.MarkSeen(fc.URL, item.GUID)
		sent++

		if err := b.State.Save(); err != nil {
			log.Printf("error saving state: %v", err)
		}
	}

	if firstRun {
		log.Printf("first run for %s: marked %d items as seen", fc.URL, len(items))
		if err := b.State.Save(); err != nil {
			log.Printf("error saving state: %v", err)
		}
	}

	return nil
}

func (b *Bot) buildMessage(ctx context.Context, fc config.FeedConfig, item feed.Item) telegram.Message {
	pageURL := wiki.DirectPageURL(item.Link)

	msg := telegram.Message{
		Text: telegram.FormatMessage(fc.Compiled, fc.Type, item, pageURL),
	}

	if b.Wiki != nil {
		pageTitle := wiki.PageTitleFromURL(item.Link)
		if pageTitle != "" {
			imageURL, err := b.Wiki.FetchPageImage(ctx, pageTitle)
			if err != nil {
				log.Printf("error fetching image for %q: %v", pageTitle, err)
				return msg
			}
			if imageURL != "" {
				data, err := b.Wiki.DownloadImage(ctx, imageURL)
				if err != nil {
					log.Printf("error downloading image %s: %v", imageURL, err)
					return msg
				}
				msg.ImageData = data
			}
		}
	}

	return msg
}
