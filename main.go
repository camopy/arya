package main

import (
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net/http"
	"os"
	"strconv"
)

const metricsServerAddr = "0.0.0.0:9091"

type Config struct {
	RedisURI  string
	BotConfig BotConfig
}

func main() {
	cfg, err := decodeEnv()
	if err != nil {
		panic(err)
	}
	go startMetricsServer()
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

func startMetricsServer() {
	var mux http.ServeMux
	mux.Handle("/metrics", promhttp.Handler())
	log.Printf("starting metrics server %s", fmt.Sprintf("http://%s/metrics", metricsServerAddr))
	if err := http.ListenAndServe(metricsServerAddr, &mux); err != nil {
		log.Fatalf("failed to start metrics server: %v", err)
	}
}
