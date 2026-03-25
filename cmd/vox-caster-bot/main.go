package main

import (
	"context"
	"flag"
	"log"
	"os/signal"
	"syscall"

	"vox-caster-bot/internal/bot"
	"vox-caster-bot/internal/config"
	"vox-caster-bot/internal/feed"
	"vox-caster-bot/internal/state"
	"vox-caster-bot/internal/telegram"
	"vox-caster-bot/internal/wiki"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	once := flag.Bool("once", false, "poll once and exit instead of running the loop")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	store, err := state.NewFileStore(cfg.StatePath, cfg.StateMaxAge)
	if err != nil {
		log.Fatalf("load state: %v", err)
	}

	b := &bot.Bot{
		Feeds:     cfg.Feeds,
		ChannelID: cfg.ChannelID,
		Interval:  cfg.PollInterval,
		Fetcher:   feed.NewHTTPFetcher(cfg.InsecureSkipVerify),
		State:     store,
		Telegram:  telegram.NewClient(cfg.TelegramToken),
	}

	if cfg.WikiAPI != "" {
		b.Wiki = wiki.NewClient(cfg.WikiAPI, cfg.InsecureSkipVerify)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if *once {
		b.Poll(ctx)
		return
	}

	if err := b.Run(ctx); err != nil {
		log.Fatalf("bot error: %v", err)
	}
}
