package bot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"go.uber.org/zap"

	"github.com/camopy/rss_everything/bot/commands"
	"github.com/camopy/rss_everything/bot/feeds"
	"github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/util/psub"
	"github.com/camopy/rss_everything/util/run"
	"github.com/camopy/rss_everything/zaplog"
)

const (
	hackerNewsThreadId = 6
	cryptoThreadId     = 9
	maxRetries         = 4
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
	cfg    TelegramConfig
	client *bot.Bot
	logger *zaplog.Logger
	db     db.DB

	chatGPT    *feeds.ChatGPT
	hackerNews *feeds.HackerNews
	cryptoFeed *feeds.CryptoFeed
	reddit     *feeds.Reddit
	rss        *feeds.RSS

	telegramSubscriber psub.Subscriber[*models.Update]
	telegramPublisher  psub.Publisher[*models.Update]
	contentSubscriber  psub.Subscriber[[]commands.Content]
	contentPublisher   psub.Publisher[[]commands.Content]
}

func NewTelegramBot(logger *zaplog.Logger, db db.DB, cfg TelegramConfig) *Telegram {
	telegramSubscriber, telegramPublisher := psub.NewSubscriber[*models.Update](
		psub.WithSubscriberName("telegram-updates"),
		psub.WithSubscriberSubscriptionOptions(psub.WithSubscriptionBlocking(true)),
	)

	contentSubscriber, contentPublisher := psub.NewSubscriber[[]commands.Content](
		psub.WithSubscriberName("content-updates"),
		psub.WithSubscriberSubscriptionOptions(psub.WithSubscriptionBlocking(true)),
	)

	handler := func(ctx context.Context, b *bot.Bot, update *models.Update) {
		if update.Message == nil {
			return
		}
		_ = telegramPublisher.SendData(ctx, update)
	}
	opts := []bot.Option{
		bot.WithDefaultHandler(handler),
	}
	client, err := bot.New(cfg.TelegramApiKey, opts...)
	if err != nil {
		panic(err)
	}

	return &Telegram{
		cfg:    cfg,
		client: client,
		logger: logger,
		db:     db,

		telegramSubscriber: telegramSubscriber,
		telegramPublisher:  telegramPublisher,
		contentSubscriber:  contentSubscriber,
		contentPublisher:   contentPublisher,
	}
}

func (b *Telegram) Name() string {
	return "telegram-service"
}

func (b *Telegram) Start(ctx run.Context) error {
	ctx.Go("handle-content-updates", b.handleContentUpdates)
	ctx.Go("handle-messages", b.handleMessages)

	b.initFeeds(ctx, b.cfg)
	b.client.Start(ctx)

	return nil
}

func (b *Telegram) handleContentUpdates(ctx context.Context) error {
	return psub.ProcessWithContext(ctx, b.contentSubscriber.Subscribe(ctx), func(ctx context.Context, contents []commands.Content) error {
		for _, c := range contents {
			attempt := 0
			err := retry.Do(
				func() error {
					_, err := b.client.SendMessage(ctx, &bot.SendMessageParams{
						ChatID:          b.cfg.ChatId,
						Text:            c.Text,
						MessageThreadID: c.ThreadId,
					})
					return err
				},
				retry.RetryIf(bot.IsTooManyRequestsError),
				retry.LastErrorOnly(true),
				retry.Context(ctx),
				retry.Attempts(maxRetries),
				retry.DelayType(func(n uint, err error, config *retry.Config) time.Duration {
					if bot.IsTooManyRequestsError(err) {
						return time.Duration(err.(*bot.TooManyRequestsError).RetryAfter) * time.Second
					}
					return retry.BackOffDelay(n, err, config)
				}),
				retry.OnRetry(func(n uint, err error) {
					attempt++
					b.logger.Warn(fmt.Sprintf("failed to send content update to telegram, retrying..."), zap.Error(err), zap.Uint("attempt", n))
				}),
			)
			if err != nil {
				b.logger.Error(fmt.Sprintf("failed to send content update to telegram: %v", err))
			}
		}
		return nil
	})
}

func (b *Telegram) handleMessages(ctx context.Context) error {
	isCommand := func(m *models.Message) bool {
		if m.Entities == nil || len(m.Entities) == 0 {
			return false
		}
		entity := m.Entities[0]
		return entity.Offset == 0 && entity.Type == "bot_command"
	}

	return psub.ProcessWithContext(ctx, b.telegramSubscriber.Subscribe(ctx), func(ctx context.Context, update *models.Update) error {
		b.logger.Info(
			"message received",
			zap.Int("threadId", update.Message.MessageThreadID),
			zap.String("msg", update.Message.Text),
		)
		if !b.isValidChatId(update.Message.Chat.ID) {
			b.logger.Info("invalid chat id", zap.Int64("chatId", update.Message.Chat.ID))
			return nil
		}
		if isCommand(update.Message) {
			b.handleCommand(ctx, update)
			return nil
		}
		b.chatGPT.ProcessPrompt(ctx, commands.Content{
			Text:     update.Message.Text,
			ThreadId: update.Message.MessageThreadID,
		})
		return nil
	})
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

func (b *Telegram) initFeeds(ctx run.Context, cfg TelegramConfig) {
	b.chatGPT = feeds.NewChatGPT(b.logger.Named("chat-gpt"), b.contentPublisher, cfg.ChatGPTApiKey, cfg.ChatGPTUserName)
	b.hackerNews = feeds.NewHackerNews(b.logger.Named("hacker-news"), b.contentPublisher, b.db, hackerNewsThreadId)
	b.cryptoFeed = feeds.NewCryptoFeed(b.logger.Named("crypto"), b.contentPublisher, cryptoThreadId)
	b.reddit = feeds.NewReddit(b.logger.Named("reddit"), b.contentPublisher, b.db, cfg.RedditClientId, cfg.RedditApiKey, cfg.RedditUsername, cfg.RedditPassword)
	b.rss = feeds.NewRSS(b.logger.Named("rss"), b.contentPublisher, b.db)

	ctx.Start(b.hackerNews)
	ctx.Start(b.chatGPT)
	ctx.Start(b.cryptoFeed)
	ctx.Start(b.reddit)
	ctx.Start(b.rss)
}
