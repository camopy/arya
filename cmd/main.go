package main

import (
	"context"
	"errors"
	"os"
	"strconv"

	"github.com/camopy/rss_everything/bot"
	"github.com/camopy/rss_everything/db"
	. "github.com/camopy/rss_everything/util/run"
	"github.com/camopy/rss_everything/zaplog"
)

type Config struct {
	RedisURI       string
	ChatId         int
	TelegramApiKey string
	DiscordApiKey  string
	RedditClientId string
	RedditApiKey   string
	RedditUsername string
	RedditPassword string
}

func main() {
	cfg, err := decodeEnv()
	if err != nil {
		panic(err)
	}
	logger := zaplog.Configure()
	defer zaplog.Recover()

	ctx := NewContext(context.Background(), logger.Named("run"), "main")

	ctx.Go("monitoring-server", func(ctx context.Context) error {
		startMonitoringServer(logger)
		return nil
	})

	if cfg.DiscordApiKey != "" {
		discordBot := bot.NewDiscordBot(
			logger.Named("discord-bot"),
			db.NewRedis(cfg.RedisURI),
			bot.DiscordConfig{
				DiscordApiKey:  cfg.DiscordApiKey,
				RedditClientId: cfg.RedditClientId,
				RedditApiKey:   cfg.RedditApiKey,
				RedditUsername: cfg.RedditUsername,
				RedditPassword: cfg.RedditPassword,
			},
		)
		ctx.Start(discordBot)
	} else if cfg.TelegramApiKey != "" {
		telegramBot := bot.NewTelegramBot(
			logger.Named("telegram-bot"),
			db.NewRedis(cfg.RedisURI),
			bot.TelegramConfig{
				TelegramApiKey: cfg.TelegramApiKey,
				ChatId:         cfg.ChatId,
				RedditClientId: cfg.RedditClientId,
				RedditApiKey:   cfg.RedditApiKey,
				RedditUsername: cfg.RedditUsername,
				RedditPassword: cfg.RedditPassword,
			},
		)
		ctx.Start(telegramBot)
	}
}

func decodeEnv() (*Config, error) {
	cfg := &Config{}
	redisURI, err := lookupEnv("REDIS_URL")
	if err != nil {
		return nil, err
	}
	cfg.RedisURI = redisURI

	telegramChatId, err := lookupEnv("TELEGRAM_CHAT_ID")
	if err != nil {
		return nil, err
	}
	chatId, err := strconv.Atoi(telegramChatId)
	if err != nil {
		return nil, err
	}
	cfg.ChatId = chatId

	telegramApiKey, err := lookupEnv("TELEGRAM_API_KEY")
	if err != nil {
		return nil, err
	}
	cfg.TelegramApiKey = telegramApiKey

	discordApiKey, err := lookupEnv("DISCORD_API_KEY")
	if err != nil {
		return nil, err
	}
	cfg.DiscordApiKey = discordApiKey

	redditClientId, err := lookupEnv("REDDIT_CLIENT_ID")
	if err != nil {
		return nil, err
	}
	cfg.RedditClientId = redditClientId

	redditApiKey, err := lookupEnv("REDDIT_API_KEY")
	if err != nil {
		return nil, err
	}
	cfg.RedditApiKey = redditApiKey

	redditUsername, err := lookupEnv("REDDIT_USERNAME")
	if err != nil {
		return nil, err
	}
	cfg.RedditUsername = redditUsername

	redditPassword, err := lookupEnv("REDDIT_PASSWORD")
	if err != nil {
		return nil, err
	}
	cfg.RedditPassword = redditPassword

	return cfg, nil
}

func lookupEnv(key string) (string, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", errors.New("missing env var " + key)
	}
	return v, nil
}
