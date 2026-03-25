package config

import (
	"fmt"
	"os"
	"strings"
	"text/template"
	"time"

	"gopkg.in/yaml.v3"
)

type FeedType string

const (
	FeedNewPage FeedType = "new_page"
	FeedUpdate  FeedType = "update"
)

// TemplateFuncs are the custom functions available in feed message templates.
var TemplateFuncs = template.FuncMap{
	// striphtml removes HTML tags from a string.
	"striphtml": func(s string) string {
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
	},
}

type FeedConfig struct {
	URL      string `yaml:"url"`
	Type     FeedType `yaml:"type"`
	Template string   `yaml:"template"`
	// Compiled is set by Load() when Template is non-empty.
	Compiled *template.Template `yaml:"-"`
}

type Config struct {
	TelegramToken      string        `yaml:"-"`
	ChannelID          string        `yaml:"-"`
	PollInterval       time.Duration `yaml:"-"`
	Feeds              []FeedConfig  `yaml:"-"`
	StatePath          string        `yaml:"-"`
	StateMaxAge        time.Duration `yaml:"-"`
	WikiAPI            string        `yaml:"-"`
	InsecureSkipVerify bool          `yaml:"-"`
}

type rawConfig struct {
	TelegramToken      string       `yaml:"telegram_token"`
	ChannelID          string       `yaml:"channel_id"`
	PollInterval       string       `yaml:"poll_interval"`
	Feeds              []FeedConfig `yaml:"feeds"`
	StatePath          string       `yaml:"state_path"`
	StateMaxAge        string       `yaml:"state_max_age"`
	WikiAPI            string       `yaml:"wiki_api"`
	InsecureSkipVerify bool         `yaml:"insecure_skip_verify"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	// Env var override for token
	if env := os.Getenv("TELEGRAM_TOKEN"); env != "" {
		raw.TelegramToken = env
	}

	if raw.TelegramToken == "" {
		return nil, fmt.Errorf("telegram_token is required (set in config or TELEGRAM_TOKEN env var)")
	}
	if raw.ChannelID == "" {
		return nil, fmt.Errorf("channel_id is required")
	}
	if len(raw.Feeds) == 0 {
		return nil, fmt.Errorf("at least one feed is required")
	}

	for i, f := range raw.Feeds {
		if f.URL == "" {
			return nil, fmt.Errorf("feed[%d]: url is required", i)
		}
		if f.Type != FeedNewPage && f.Type != FeedUpdate {
			return nil, fmt.Errorf("feed[%d]: type must be %q or %q", i, FeedNewPage, FeedUpdate)
		}
		if f.Template != "" {
			tmpl, err := template.New("").Funcs(TemplateFuncs).Parse(f.Template)
			if err != nil {
				return nil, fmt.Errorf("feed[%d]: parse template: %w", i, err)
			}
			raw.Feeds[i].Compiled = tmpl
		}
	}

	interval := 5 * time.Minute
	if raw.PollInterval != "" {
		interval, err = time.ParseDuration(raw.PollInterval)
		if err != nil {
			return nil, fmt.Errorf("parse poll_interval: %w", err)
		}
	}

	statePath := raw.StatePath
	if statePath == "" {
		statePath = "state.json"
	}

	stateMaxAge := 30 * 24 * time.Hour
	if raw.StateMaxAge != "" {
		stateMaxAge, err = time.ParseDuration(raw.StateMaxAge)
		if err != nil {
			return nil, fmt.Errorf("parse state_max_age: %w", err)
		}
	}

	return &Config{
		TelegramToken:      raw.TelegramToken,
		ChannelID:          raw.ChannelID,
		PollInterval:       interval,
		Feeds:              raw.Feeds,
		StatePath:          statePath,
		StateMaxAge:        stateMaxAge,
		WikiAPI:            raw.WikiAPI,
		InsecureSkipVerify: raw.InsecureSkipVerify,
	}, nil
}
