package main

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type BotConfig struct {
	ChatId         int
	TelegramApiKey string
	ChatGPTApiKey  string
}

type Bot struct {
	cfg          BotConfig
	client       *tgbotapi.BotAPI
	db           DB
	contentsChan chan []string
	chatGPT      *ChatGPT
	hackerNews   *HackerNews
}

func NewBot(db DB, cfg BotConfig) *Bot {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramApiKey)
	if err != nil {
		log.Panic(err)
	}
	api.Debug = true
	log.Printf("Authorized on account %s", api.Self.UserName)
	return &Bot{
		cfg:          cfg,
		client:       api,
		db:           db,
		contentsChan: make(chan []string),
	}
}

func (b *Bot) Start() {
	b.initFeeds(b.cfg)
	go b.handleContentUpdates()
	b.handleMessages()
}

func (b *Bot) initFeeds(cfg BotConfig) {
	b.chatGPT = NewChatGPT(b.contentsChan, cfg.ChatGPTApiKey)
	b.hackerNews = NewHackerNews(b.contentsChan, b.db)

	go b.hackerNews.StartHackerNews()
	go b.chatGPT.StartChatGPT()
}

func (b *Bot) handleContentUpdates() {
	for {
		select {
		case contents := <-b.contentsChan:
			for _, c := range contents {
				msg := tgbotapi.NewMessage(int64(b.cfg.ChatId), c)
				_, err := b.client.Send(msg)
				if err != nil {
					log.Println(err)
				}
			}
		}
	}
}

func (b *Bot) handleMessages() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.client.GetUpdatesChan(u)
	for update := range updates {
		if update.Message != nil && b.isValidChatId(update.Message.Chat.ID) {
			if !update.Message.IsCommand() {
				b.chatGPT.Ask(update.Message.Text)
			}
		}
	}
}

func (b *Bot) isValidChatId(id int64) bool {
	return id == int64(b.cfg.ChatId)
}
