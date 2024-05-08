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
	RedisURI  string
	BotConfig bot.TelegramConfig
}

func main() {
	cfg, err := decodeEnv()
	if err != nil {
		panic(err)
	}
	logger := zaplog.Configure()
	defer zaplog.Recover()

	telegramBot := bot.NewTelegramBot(
		logger.Named("telegram-bot"),
		db.NewRedis(cfg.RedisURI),
		cfg.BotConfig,
	)

	ctx := NewContext(context.Background(), logger.Named("run"), "main")

	ctx.Go("monitoring-server", func(ctx context.Context) error {
		startMonitoringServer(logger)
		return nil
	})

	ctx.Start(telegramBot)
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
	cfg.BotConfig.ChatId = chatId

	telegramApiKey, err := lookupEnv("TELEGRAM_API_KEY")
	if err != nil {
		return nil, err
	}
	cfg.BotConfig.TelegramApiKey = telegramApiKey

	chatGPTApiKey, err := lookupEnv("CHATGPT_API_KEY")
	if err != nil {
		return nil, err
	}
	cfg.BotConfig.ChatGPTApiKey = chatGPTApiKey

	chatGPTUserName, err := lookupEnv("CHATGPT_USER_NAME")
	if err != nil {
		return nil, err
	}
	cfg.BotConfig.ChatGPTUserName = chatGPTUserName

	redditClientId, err := lookupEnv("REDDIT_CLIENT_ID")
	if err != nil {
		return nil, err
	}
	cfg.BotConfig.RedditClientId = redditClientId

	redditApiKey, err := lookupEnv("REDDIT_API_KEY")
	if err != nil {
		return nil, err
	}
	cfg.BotConfig.RedditApiKey = redditApiKey

	redditUsername, err := lookupEnv("REDDIT_USERNAME")
	if err != nil {
		return nil, err
	}
	cfg.BotConfig.RedditUsername = redditUsername

	redditPassword, err := lookupEnv("REDDIT_PASSWORD")
	if err != nil {
		return nil, err
	}
	cfg.BotConfig.RedditPassword = redditPassword

	return cfg, nil
}

func lookupEnv(key string) (string, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", errors.New("missing env var " + key)
	}
	return v, nil
}
