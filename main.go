package main

import (
	"errors"
	"os"
	"strconv"
)

type Config struct {
	RedisURI  string
	BotConfig BotConfig
}

func main() {
	cfg, err := decodeEnv()
	if err != nil {
		panic(err)
	}
	db := NewRedis(cfg.RedisURI)
	bot := NewBot(db, cfg.BotConfig)
	bot.Start()
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

	return cfg, nil
}

func lookupEnv(key string) (string, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return "", errors.New("missing env var " + key)
	}
	return v, nil
}
