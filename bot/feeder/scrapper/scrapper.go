package scrapper

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/camopy/rss_everything/bot/commands"
	"github.com/camopy/rss_everything/db"
	ge "github.com/camopy/rss_everything/util/generics"
	"github.com/camopy/rss_everything/util/psub"
	"github.com/camopy/rss_everything/util/run"
	"github.com/camopy/rss_everything/zaplog"
)

const (
	scrapperSubscriptionsTable = "scrapper:subscriptions:"
	OlxPlatform                = "olx"
	ZapImoveisPlatform         = "zap_imoveis"
)

type Scrapper struct {
	logger        *zaplog.Logger
	db            db.DB
	subscriptions map[string]*subscription

	contentPublisher psub.Publisher[[]commands.Content]
}

func New(logger *zaplog.Logger, contentPublisher psub.Publisher[[]commands.Content], db db.DB) *Scrapper {
	return &Scrapper{
		logger:        logger,
		db:            db,
		subscriptions: make(map[string]*subscription),

		contentPublisher: contentPublisher,
	}
}

func (u *Scrapper) HandleCommand(ctx context.Context, cmd commands.Command) error {
	c, err := u.parseCommand(cmd)
	if err != nil {
		return err
	}
	switch c.name {
	case "add":
		err = u.add(ctx, c)
	case "list":
		err = u.list(ctx, c)
	case "remove":
		err = u.remove(ctx, c)
	}
	return err
}

type scrapperCommand struct {
	threadId int
	name     string
	platform string
	url      string
	interval time.Duration
}

func (u *Scrapper) parseCommand(cmd commands.Command) (*scrapperCommand, error) {
	s := strings.Split(cmd.Text, " ")

	switch s[0] {
	case "list":
		return parseListCommand(cmd.ThreadId, s)
	case "remove":
		return parseRemoveCommand(cmd.ThreadId, s)
	case "add":
		return parseAddCommand(cmd.ThreadId, s)
	}

	return nil, fmt.Errorf("scrapper: invalid command")
}

func parseListCommand(threadId int, s []string) (*scrapperCommand, error) {
	return &scrapperCommand{
		threadId: threadId,
		name:     s[0],
	}, nil
}

func parseRemoveCommand(threadId int, s []string) (*scrapperCommand, error) {
	if len(s) < 3 {
		return nil, fmt.Errorf("scrapper: invalid arguments")
	}

	return &scrapperCommand{
		threadId: threadId,
		name:     s[0],
		platform: s[1],
		url:      s[2],
	}, nil
}

func parseAddCommand(threadId int, s []string) (*scrapperCommand, error) {
	if len(s) < 4 {
		return nil, fmt.Errorf("scrapper: invalid arguments")
	}

	c := &scrapperCommand{
		threadId: threadId,
		name:     s[0],
		platform: s[1],
		url:      s[2],
	}

	interval, err := strconv.Atoi(s[3])
	if err != nil {
		return nil, fmt.Errorf("scrapper: invalid interval")
	}
	c.interval = time.Duration(interval) * time.Minute

	if c.interval.Minutes() < 60 {
		return nil, commands.ErrInvalidIntervalDuration
	}

	return c, nil
}

type subscription struct {
	Id       string        `json:"id"`
	Platform string        `json:"platform"`
	Url      string        `json:"url"`
	Interval time.Duration `json:"interval"`
	ThreadId int           `json:"thread_id"`

	cancelFunc context.CancelFunc
}

func (u *Scrapper) add(ctx context.Context, c *scrapperCommand) error {
	u.logger.Info(
		"adding scrapper",
		zap.String("platform", c.platform),
		zap.String("url", c.url),
		zap.Int("threadId", c.threadId),
	)

	sub := subscription{
		Url:      c.url,
		Platform: c.platform,
		Interval: c.interval,
		ThreadId: c.threadId,
	}
	if err := u.saveSubscription(ctx, &sub); err != nil {
		return err
	}
	u.addSubscription(&sub)
	u.logger.Info(
		"scrapper added",
		zap.String("platform", c.platform),
		zap.String("url", c.url),
		zap.Int("threadId", c.threadId),
	)

	u.scrapSubscription(ctx, &sub)
	return nil
}

func (u *Scrapper) saveSubscription(ctx context.Context, sub *subscription) error {
	b, err := json.Marshal(sub)
	if err != nil {
		return err
	}
	sub.Id, err = u.db.Add(ctx, scrapperSubscriptionsTable, b)
	if err != nil {
		return err
	}
	return nil
}

func (u *Scrapper) list(ctx context.Context, c *scrapperCommand) error {
	u.logger.Info("listing subscriptions", zap.Int("threadId", c.threadId))

	if len(u.subscriptions) == 0 {
		u.logger.Info("no subscriptions")
		return u.contentPublisher.SendData(ctx, []commands.Content{
			{
				ThreadId: c.threadId,
				Text:     "No subscriptions",
			},
		})
	}

	var messages []string
	var msg string
	for _, sub := range u.subscriptions {
		newEntry := fmt.Sprintf("%s: %s\n", sub.Url, sub.Interval)

		if len(msg)+len(newEntry) > 1000 {
			messages = append(messages, msg)
			msg = ""
		}
		msg += newEntry
	}
	if len(msg) > 0 {
		messages = append(messages, msg)
	}

	return u.contentPublisher.SendData(ctx, ge.Map(messages, func(message string) commands.Content {
		return commands.Content{
			Text:     message,
			ThreadId: c.threadId,
		}
	}))
}

func (u *Scrapper) getSubscriptions(ctx context.Context) ([]subscription, error) {
	b, err := u.db.List(ctx, scrapperSubscriptionsTable)
	if err != nil {
		return nil, err
	}
	var subs []subscription
	for id, v := range b {
		var sub subscription
		err = json.Unmarshal([]byte(v), &sub)
		if err != nil {
			return nil, err
		}
		sub.Id = id
		subs = append(subs, sub)
	}
	return subs, nil
}

func (u *Scrapper) remove(ctx context.Context, cmd *scrapperCommand) error {
	u.logger.Info("removing scrapper", zap.String("url", cmd.url), zap.Int("threadId", cmd.threadId))
	if err := u.removeSubscription(ctx, cmd); err != nil {
		return u.contentPublisher.SendData(ctx, []commands.Content{
			{
				ThreadId: cmd.threadId,
				Text:     err.Error(),
			},
		})
	}
	return nil
}

func (u *Scrapper) removeSubscription(ctx context.Context, cmd *scrapperCommand) error {
	sub := u.findSubscription(cmd.url)
	if sub == nil {
		return fmt.Errorf("scrapper: url %s not found", cmd.url)
	}

	err := u.db.Del(ctx, scrapperSubscriptionsTable, sub.Id)
	if err != nil {
		return err
	}
	delete(u.subscriptions, cmd.url)
	sub.cancelFunc()

	u.logger.Info("scrapper removed", zap.String("url", cmd.url), zap.Int("threadId", cmd.threadId))
	return u.contentPublisher.SendData(ctx, []commands.Content{
		{
			ThreadId: cmd.threadId,
			Text:     fmt.Sprintf("scrapper: removed %s", cmd.url),
		},
	})
}

func (u *Scrapper) Name() string {
	return "scrapper-service"
}

func (u *Scrapper) Start(ctx run.Context) error {
	u.logger.Info("starting scrapper")
	subs, err := u.getSubscriptions(ctx)
	if err != nil && !u.db.IsErrNotFound(err) {
		panic(err)
	}

	for i := range subs {
		sub := subs[i]
		u.addSubscription(&sub)
		u.scrapSubscription(ctx, &sub)
	}

	return nil
}

func (u *Scrapper) addSubscription(sub *subscription) {
	key := strings.ToLower(sub.Url)
	u.subscriptions[key] = sub
}

func (u *Scrapper) findSubscription(url string) *subscription {
	key := strings.ToLower(url)
	return u.subscriptions[key]
}

func (u *Scrapper) scrapSubscription(ctx context.Context, sub *subscription) {
	ctx, cancel := context.WithCancel(ctx)
	sub.cancelFunc = cancel
	go u.scrap(ctx, sub)
}

func (u *Scrapper) scrap(ctx context.Context, sub *subscription) {
	u.logger.Info("scrapping", zap.String("url", sub.Url), zap.Int("threadId", sub.ThreadId))
	fetch := func(ctx context.Context, sub *subscription) {
		var items []commands.Content
		var err error

		switch sub.Platform {
		case OlxPlatform:
			olxScrapper := NewOlx(u.logger.Named("olx"), u.db)
			items, err = olxScrapper.scrap(ctx, sub.ThreadId, sub.Url)
		case ZapImoveisPlatform:
			zapImoveisScrapper := NewZapImoveis(u.logger.Named("zap-imoveis"), u.db)
			items, err = zapImoveisScrapper.scrap(ctx, sub.ThreadId, sub.Url)
		}

		if err != nil {
			u.logger.Error("error fetching items", zap.Error(err))
		} else if len(items) > 0 {
			u.logger.Info("sending items", zap.Int("count", len(items)), zap.Int("threadId", sub.ThreadId))
			_ = u.contentPublisher.SendData(ctx, items)
		}

		u.logger.Info("finished scrapping", zap.String("url", sub.Url), zap.Int("threadId", sub.ThreadId), zap.Int("new items", len(items)))
	}
	fetch(ctx, sub)

	ticker := time.NewTicker(sub.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			fetch(ctx, sub)
		case <-ctx.Done():
			return
		}
	}
}
