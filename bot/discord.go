package bot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/avast/retry-go/v4"
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"

	"github.com/camopy/rss_everything/bot/commands"
	"github.com/camopy/rss_everything/bot/feeds"
	"github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/util/psub"
	"github.com/camopy/rss_everything/util/run"
	"github.com/camopy/rss_everything/zaplog"
)

type DiscordConfig struct {
	DiscordApiKey   string
	ChatGPTApiKey   string
	ChatGPTUserName string
	RedditClientId  string
	RedditApiKey    string
	RedditUsername  string
	RedditPassword  string
}

type Discord struct {
	client *discordgo.Session
	cfg    DiscordConfig
	logger *zaplog.Logger
	db     db.DB

	chatGPT    *feeds.ChatGPT
	hackerNews *feeds.HackerNews
	cryptoFeed *feeds.CryptoFeed
	reddit     *feeds.Reddit
	rss        *feeds.RSS

	discordSubscriber psub.Subscriber[*discordgo.MessageCreate]
	discordPublisher  psub.Publisher[*discordgo.MessageCreate]
	contentSubscriber psub.Subscriber[[]commands.Content]
	contentPublisher  psub.Publisher[[]commands.Content]
}

func NewDiscordBot(logger *zaplog.Logger, db db.DB, cfg DiscordConfig) *Discord {
	discordSubscriber, discordPublisher := psub.NewSubscriber[*discordgo.MessageCreate](
		psub.WithSubscriberName("discord-updates"),
		psub.WithSubscriberSubscriptionOptions(psub.WithSubscriptionBlocking(true)),
	)

	contentSubscriber, contentPublisher := psub.NewSubscriber[[]commands.Content](
		psub.WithSubscriberName("content-updates"),
		psub.WithSubscriberSubscriptionOptions(psub.WithSubscriptionBlocking(true)),
	)

	handler := func(discord *discordgo.Session, message *discordgo.MessageCreate) {
		/* prevent bot responding to its own message
		this is achived by looking into the message author id
		if message.author.id is same as bot.author.id then just return
		*/
		if message.Author.ID == discord.State.User.ID {
			return
		}

		_ = discordPublisher.SendData(context.Background(), message)
	}

	client, err := discordgo.New("Bot " + cfg.DiscordApiKey)
	if err != nil {
		panic(err)
	}

	client.AddHandler(handler)

	return &Discord{
		cfg:    cfg,
		client: client,
		logger: logger,
		db:     db,

		discordSubscriber: discordSubscriber,
		discordPublisher:  discordPublisher,
		contentSubscriber: contentSubscriber,
		contentPublisher:  contentPublisher,
	}
}

func (b *Discord) Name() string {
	return "discord-service"
}

func (b *Discord) Start(ctx run.Context) error {
	ctx.Go("handle-content-updates", b.handleContentUpdates)
	ctx.Go("handle-messages", b.handleMessages)

	b.initFeeds(ctx, b.cfg)
	err := b.client.Open()
	if err != nil {
		return err
	}
	defer b.client.Close()

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	<-c

	return nil
}

func (b *Discord) handleContentUpdates(ctx context.Context) error {
	isTooManyRequestsError := func(err error) bool {
		var rateLimitError *discordgo.RateLimitError
		return errors.As(err, &rateLimitError)
	}
	return psub.ProcessWithContext(ctx, b.contentSubscriber.Subscribe(ctx), func(ctx context.Context, contents []commands.Content) error {
		for _, c := range contents {
			attempt := 0
			err := retry.Do(
				func() error {
					_, err := b.client.ChannelMessageSend(strconv.Itoa(c.ThreadId), c.Text)
					return err
				},
				retry.RetryIf(isTooManyRequestsError),
				retry.LastErrorOnly(true),
				retry.Context(ctx),
				retry.Attempts(maxRetries),
				retry.DelayType(func(n uint, err error, config *retry.Config) time.Duration {
					var rateLimitError *discordgo.RateLimitError
					if errors.As(err, &rateLimitError) {
						return rateLimitError.RetryAfter * time.Second
					}
					return retry.BackOffDelay(n, err, config)
				}),
				retry.OnRetry(func(n uint, err error) {
					attempt++
					b.logger.Warn(fmt.Sprintf("failed to send content update to discord, retrying..."), zap.Error(err), zap.Uint("attempt", n))
				}),
			)
			if err != nil {
				b.logger.Error(fmt.Sprintf("failed to send content update to discord: %v", err))
			}
		}
		return nil
	})
}

func (b *Discord) handleMessages(ctx context.Context) error {
	return psub.ProcessWithContext(ctx, b.discordSubscriber.Subscribe(ctx), func(ctx context.Context, update *discordgo.MessageCreate) error {
		b.logger.Info(
			"message received",
			zap.String("threadId", update.Message.ChannelID),
			zap.String("msg", update.Message.Content),
		)

		//if !b.isValidChatId(update.Message.Chat.ID) {
		//	b.logger.Info("invalid chat id", zap.Int64("chatId", update.Message.Chat.ID))
		//	return nil

		b.handleCommand(ctx, update)
		return nil
	})
}

//func (b *Discord) isValidChatId(id int64) bool {
//	return id == int64(b.cfg.ChatId)
//}

func (b *Discord) handleCommand(ctx context.Context, update *discordgo.MessageCreate) {
	b.logger.Info("command received", zap.String("cmd", update.Content))
	threadId, err := strconv.Atoi(update.Message.ChannelID)
	if err != nil {
		b.logger.Error("failed to parse thread id", zap.Error(err))
		return
	}
	name := strings.Split(update.Message.Content, " ")[0]
	cmd := commands.Command{
		Name:     name,
		ThreadId: threadId,
		Text:     update.Message.Content[len(name)+1:],
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
	case "/hn":
		err := b.hackerNews.HandleCommand(ctx, cmd)
		if err != nil {
			b.logger.Error("hacker news command failed", zap.Error(err))
		}
	}
}

func (b *Discord) initFeeds(ctx run.Context, cfg DiscordConfig) {
	b.hackerNews = feeds.NewHackerNews(b.logger.Named("hacker-news"), b.contentPublisher, b.db)
	//b.cryptoFeed = feeds.NewCryptoFeed(b.logger.Named("crypto"), b.contentPublisher, cryptoThreadId)
	b.reddit = feeds.NewReddit(b.logger.Named("reddit"), b.contentPublisher, b.db, cfg.RedditClientId, cfg.RedditApiKey, cfg.RedditUsername, cfg.RedditPassword)
	b.rss = feeds.NewRSS(b.logger.Named("rss"), b.contentPublisher, b.db)

	ctx.Start(b.hackerNews)
	//ctx.Start(b.cryptoFeed)
	ctx.Start(b.reddit)
	ctx.Start(b.rss)
}
