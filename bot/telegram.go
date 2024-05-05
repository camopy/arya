package bot

import (
	"context"
	"os"
	"os/signal"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"

	"github.com/camopy/rss_everything/bot/commands"
	feeds "github.com/camopy/rss_everything/bot/feeds"
	"github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/zaplog"
)

const (
	hackerNewsThreadId = 6
	cryptoThreadId     = 9
)

type TelegramConfig struct {
	ChatId          int
	TelegramApiKey  string
	ChatGPTApiKey   string
	ChatGPTUserName string
	RedditClientId  string
	RedditApiKey    string
	RedditUsername  string
	RedditPassword  string
}

type Telegram struct {
	cfg          TelegramConfig
	client       *bot.Bot
	logger       *zaplog.Logger
	db           db.DB
	updatesCh    chan *models.Update
	contentsChan chan []commands.Content
	chatGPT      *feeds.ChatGPT
	hackerNews   *feeds.HackerNews
	cryptoFeed   *feeds.CryptoFeed
	reddit       *feeds.Reddit
	rss          *feeds.RSS
}

func NewTelegramBot(logger *zaplog.Logger, db db.DB, cfg TelegramConfig) *Telegram {
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
		panic(err)
	}

	return &Telegram{
		cfg:          cfg,
		client:       api,
		logger:       logger,
		db:           db,
		contentsChan: make(chan []commands.Content),
		updatesCh:    updatesCh,
	}
}

func (b *Telegram) Start() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	go b.handleFeedUpdates(ctx)
	go b.handleMessages(ctx)

	b.initFeeds(ctx, b.cfg)
	b.client.Start(ctx)
}

func (b *Telegram) initFeeds(ctx context.Context, cfg TelegramConfig) {
	b.chatGPT = feeds.NewChatGPT(b.logger.Named("chat-gpt"), b.contentsChan, cfg.ChatGPTApiKey, cfg.ChatGPTUserName)
	b.hackerNews = feeds.NewHackerNews(b.logger.Named("hacker-news"), b.contentsChan, b.db, hackerNewsThreadId)
	b.cryptoFeed = feeds.NewCryptoFeed(b.logger.Named("crypto"), b.contentsChan, cryptoThreadId)
	b.reddit = feeds.NewReddit(b.logger.Named("reddit"), b.contentsChan, b.db, cfg.RedditClientId, cfg.RedditApiKey, cfg.RedditUsername, cfg.RedditPassword)
	b.rss = feeds.NewRSS(b.logger.Named("rss"), b.contentsChan, b.db)

	go b.hackerNews.StartHackerNews()
	go b.chatGPT.StartChatGPT()
	go b.cryptoFeed.StartCryptoFeed()
	go b.reddit.StartReddit(ctx)
	go b.rss.StartRSS(ctx)
}

func (b *Telegram) handleFeedUpdates(ctx context.Context) {
	for {
		select {
		case contents := <-b.contentsChan:
			for _, c := range contents {
				_, err := b.client.SendMessage(
					ctx, &bot.SendMessageParams{
						ChatID:          b.cfg.ChatId,
						Text:            c.Text,
						MessageThreadID: c.ThreadId,
					},
				)
				if err != nil {
					b.logger.Error(
						"failed to send message",
						zap.Error(err),
						zap.Int("threadId", c.ThreadId),
						zap.String("msg", c.Text),
					)
				}
			}
		}
	}
}

func (b *Telegram) handleMessages(ctx context.Context) {
	isCommand := func(m *models.Message) bool {
		if m.Entities == nil || len(m.Entities) == 0 {
			return false
		}
		entity := m.Entities[0]
		return entity.Offset == 0 && entity.Type == "bot_command"
	}

	for update := range b.updatesCh {
		if update.Message == nil {
			continue
		}
		b.logger.Info(
			"message received",
			zap.Int("threadId", update.Message.MessageThreadID),
			zap.String("msg", update.Message.Text),
		)
		if !b.isValidChatId(update.Message.Chat.ID) {
			b.logger.Info("invalid chat id", zap.Int64("chatId", update.Message.Chat.ID))
			continue
		}
		if isCommand(update.Message) {
			b.handleCommand(ctx, update)
			continue
		}
		b.chatGPT.Ask(commands.Content{
			Text:     update.Message.Text,
			ThreadId: update.Message.MessageThreadID,
		})
	}
}

func (b *Telegram) isValidChatId(id int64) bool {
	return id == int64(b.cfg.ChatId)
}

func (b *Telegram) handleCommand(ctx context.Context, update *models.Update) {
	b.logger.Info("command received", zap.String("cmd", update.Message.Text))
	entity := update.Message.Entities[0]
	cmd := commands.Command{
		Name:     update.Message.Text[:entity.Length],
		ChatId:   update.Message.Chat.ID,
		ThreadId: update.Message.MessageThreadID,
		Text:     strings.Trim(update.Message.Text[entity.Length:], " "),
	}

	switch cmd.Name {
	case "/reddit":
		err := b.reddit.HandleCommand(ctx, cmd)
		if err != nil {
			b.logger.Error("reddit command failed", zap.Error(err))
		}
	case "/rss":
		err := b.rss.HandleCommand(ctx, cmd)
		if err != nil {
			b.logger.Error("rss command failed", zap.Error(err))
		}
	}
}
