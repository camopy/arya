package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

const (
	hackerNewsThreadId = 6
	cryptoThreadId     = 9
	redditThreadId     = 452
)

type BotConfig struct {
	ChatId          int
	TelegramApiKey  string
	ChatGPTApiKey   string
	ChatGPTUserName string
	RedditClientId  string
	RedditApiKey    string
	RedditUsername  string
	RedditPassword  string
}

type Bot struct {
	cfg          BotConfig
	client       *bot.Bot
	db           DB
	updatesCh    chan *models.Update
	contentsChan chan []Content
	chatGPT      *ChatGPT
	hackerNews   *HackerNews
	cryptoFeed   *CryptoFeed
	rssFeed      *Reddit
}

type Content struct {
	text     string
	threadId int
}

func NewBot(db DB, cfg BotConfig) *Bot {
	updatesCh := make(chan *models.Update)

	handler := func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message == nil {
			return
		}
		updatesCh <- update
	}

	opts := []bot.Option{
		bot.WithDefaultHandler(handler),
	}
	api, err := bot.New(cfg.TelegramApiKey, opts...)
	if err != nil {
		log.Panic(err)
	}

	return &Bot{
		cfg:          cfg,
		client:       api,
		db:           db,
		contentsChan: make(chan []Content),
		updatesCh:    updatesCh,
	}
}

func (b *Bot) Start() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	b.initFeeds(b.cfg)
	go b.handleFeedUpdates(ctx)
	go b.handleMessages()
	b.client.Start(ctx)
}

func (b *Bot) initFeeds(cfg BotConfig) {
	b.chatGPT = NewChatGPT(b.contentsChan, cfg.ChatGPTApiKey, cfg.ChatGPTUserName)
	b.hackerNews = NewHackerNews(b.contentsChan, b.db, hackerNewsThreadId)
	b.cryptoFeed = NewCryptoFeed(b.contentsChan, cryptoThreadId)
	b.rssFeed = NewReddit(b.contentsChan, b.db, cfg.RedditClientId, cfg.RedditApiKey, cfg.RedditUsername, cfg.RedditPassword, redditThreadId)

	go b.hackerNews.StartHackerNews()
	go b.chatGPT.StartChatGPT()
	go b.cryptoFeed.StartCryptoFeed()
	go b.rssFeed.StartReddit()
}

func (b *Bot) handleFeedUpdates(ctx context.Context) {
	for {
		select {
		case contents := <-b.contentsChan:
			for _, c := range contents {
				_, err := b.client.SendMessage(
					ctx, &bot.SendMessageParams{
						ChatID:          b.cfg.ChatId,
						Text:            c.text,
						MessageThreadID: c.threadId,
					},
				)
				if err != nil {
					log.Println(err)
				}
			}
		}
	}
}

func (b *Bot) handleMessages() {
	isCommand := func(m *models.Message) bool {
		if m.Entities == nil || len(m.Entities) == 0 {
			return false
		}
		entity := m.Entities[0]
		return entity.Offset == 0 && entity.Type == "bot_command"
	}

	for update := range b.updatesCh {
		if update.Message == nil || isCommand(update.Message) {
			continue
		}
		b.chatGPT.Ask(Content{
			text:     update.Message.Text,
			threadId: update.Message.MessageThreadID,
		})
	}
}

func (b *Bot) isValidChatId(id int64) bool {
	return id == int64(b.cfg.ChatId)
}
