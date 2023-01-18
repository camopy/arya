package main

import (
	"errors"
	"flag"

	rss "github.com/camopy/rss_everything"
	"github.com/camopy/rss_everything/database"
)

type Config struct {
	RedisURI  string
	BotConfig rss.Config
}

func main() {
	cfg, err := parseFlags()
	if err != nil {
		panic(err)
	}
	db := database.NewRedis(cfg.RedisURI)
	bot := rss.NewBot(db, cfg.BotConfig)
	bot.Start()
}

func parseFlags() (Config, error) {
	cfg := Config{}
	flag.StringVar(&cfg.RedisURI, "redis.uri", "", "Redis URI (redis://host:post/db)")
	flag.StringVar(&cfg.BotConfig.TelegramApiKey, "telegram.api-key", "", "Telegram API key")
	flag.StringVar(&cfg.BotConfig.ChatGPTApiKey, "chatgpt.api-key", "", "ChatGPT API key")
	flag.Parse()
	if cfg.RedisURI == "" {
		return cfg, errors.New("missing redis addr")
	}
	if cfg.BotConfig.TelegramApiKey == "" {
		return cfg, errors.New("missing telegram api key")
	}
	if cfg.BotConfig.ChatGPTApiKey == "" {
		return cfg, errors.New("missing chatgpt api key")
	}
	return cfg, nil
}
