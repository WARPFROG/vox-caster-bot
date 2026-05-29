package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/net/proxy"
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

	wikiClient, err := buildHTTPClient(cfg.InsecureSkipVerify, "")
	if err != nil {
		log.Fatalf("build wiki http client: %v", err)
	}
	telegramClient, err := buildHTTPClient(cfg.InsecureSkipVerify, cfg.ProxyURL)
	if err != nil {
		log.Fatalf("build telegram http client: %v", err)
	}

	store, err := state.NewFileStore(cfg.StatePath, cfg.StateMaxAge)
	if err != nil {
		log.Fatalf("load state: %v", err)
	}

	b := &bot.Bot{
		Feeds:     cfg.Feeds,
		ChannelID: cfg.ChannelID,
		Interval:  cfg.PollInterval,
		Fetcher:   feed.NewHTTPFetcher(wikiClient, cfg.FetchTimeout),
		State:     store,
		Telegram:  telegram.NewClient(cfg.TelegramToken, telegramClient),
	}

	if cfg.WikiAPI != "" {
		b.Wiki = wiki.NewClient(cfg.WikiAPI, wikiClient, cfg.RequestTimeout)
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

func buildHTTPClient(insecureSkipVerify bool, proxyURL string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSHandshakeTimeout = 30 * time.Second

	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("parse proxy_url: %w", err)
		}
		switch u.Scheme {
		case "socks5", "socks5h":
			dialer, err := proxy.FromURL(u, proxy.Direct)
			if err != nil {
				return nil, fmt.Errorf("create socks5 dialer: %w", err)
			}
			if cd, ok := dialer.(proxy.ContextDialer); ok {
				transport.DialContext = cd.DialContext
			} else {
				transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
					return dialer.Dial(network, addr)
				}
			}
		default:
			transport.Proxy = http.ProxyURL(u)
		}
	}

	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return &http.Client{Transport: transport}, nil
}
