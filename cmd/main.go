package main

import (
	"errors"
	"flag"

	rss "github.com/camopy/rss_everything"
	"github.com/camopy/rss_everything/database"
)

func main() {
	db := database.NewRedis("localhost:17379")
	cfg, err := parseFlags()
	if err != nil {
		panic(err)
	}
	bot := rss.NewBot(db, cfg)
	bot.Start()
}

func parseFlags() (rss.Config, error) {
	cfg := rss.Config{}
	flag.StringVar(&cfg.TelegramApiKey, "telegram.api-key", "", "Telegram API key")
	flag.StringVar(&cfg.ChatGPTApiKey, "chatgpt.api-key", "", "ChatGPT API key")
	flag.Parse()
	if cfg.TelegramApiKey == "" {
		return cfg, errors.New("missing telegram api key")
	}
	if cfg.ChatGPTApiKey == "" {
		return cfg, errors.New("missing chatgpt api key")
	}
	return cfg, nil
}
