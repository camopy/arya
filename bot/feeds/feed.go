package feeds

import (
	"context"
	"encoding/json"
	"fmt"
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

const defaultFetchInterval = 24 * time.Hour

type Feeder interface {
	Name() string
	TableName() string

	Fetch(ctx context.Context, sub *Subscription) ([]commands.Content, error)
	ParseCommand(cmd commands.Command) (Command, error)
}

type Command interface {
	Action() string
	Interval() time.Duration
	ThreadId() int
	SubName() string
	Url() string
}

type Feed struct {
	feeder        Feeder
	logger        *zaplog.Logger
	db            db.DB
	subscriptions map[string]*Subscription

	contentPublisher psub.Publisher[[]commands.Content]
}

type Subscription struct {
	Id       string        `json:"id"`
	Name     string        `json:"name"`
	Interval time.Duration `json:"interval"`
	ThreadId int           `json:"thread_id"`
	Url      string        `json:"url"`

	cancelFunc context.CancelFunc
}

func New(logger *zaplog.Logger, contentPublisher psub.Publisher[[]commands.Content], db db.DB, feeder Feeder) *Feed {
	return &Feed{
		feeder:        feeder,
		logger:        logger,
		db:            db,
		subscriptions: make(map[string]*Subscription),

		contentPublisher: contentPublisher,
	}
}

func (h *Feed) Name() string {
	return h.feeder.Name()
}

func (h *Feed) Start(ctx run.Context) error {
	h.logger.Info("starting", zap.String("feed", h.feeder.Name()))
	subs, err := h.getSubscriptions(ctx)
	if err != nil && !h.db.IsErrNotFound(err) {
		panic(err)
	}

	for i := range subs {
		sub := subs[i]
		h.addSubscription(&sub)
		h.pollFeed(ctx, &sub)
	}

	return nil
}

func (h *Feed) HandleCommand(ctx context.Context, cmd commands.Command) error {
	c, err := h.feeder.ParseCommand(cmd)
	if err != nil {
		return err
	}
	switch c.Action() {
	case "add":
		err = h.add(ctx, c)
	case "list":
		err = h.list(ctx, c)
	case "remove":
		err = h.remove(ctx, c)
	}
	return err
}

func (h *Feed) add(ctx context.Context, c Command) error {
	h.logger.Info(
		"adding subscription",
		zap.String("feed", h.feeder.Name()),
		zap.String("name", c.SubName()),
		zap.Int("threadId", c.ThreadId()),
	)

	sub := Subscription{
		Name:     c.SubName(),
		Interval: ge.DefaultIfZero(c.Interval(), defaultFetchInterval),
		ThreadId: c.ThreadId(),
		Url:      c.Url(),
	}
	if err := h.saveSubscription(ctx, &sub); err != nil {
		return err
	}
	h.logger.Info(
		"subscription added",
		zap.String("feed", h.feeder.Name()),
		zap.String("name", c.SubName()),
		zap.Int("threadId", c.ThreadId()),
	)

	h.pollFeed(ctx, &sub)
	return nil
}

func (h *Feed) saveSubscription(ctx context.Context, sub *Subscription) error {
	b, err := json.Marshal(sub)
	if err != nil {
		return err
	}
	sub.Id, err = h.db.Add(ctx, h.feeder.TableName(), b)
	if err != nil {
		return err
	}
	h.addSubscription(sub)
	return nil
}

func (h *Feed) list(ctx context.Context, c Command) error {
	h.logger.Info("listing subscriptions", zap.Int("threadId", c.ThreadId()))

	if len(h.subscriptions) == 0 {
		h.logger.Info("no subscriptions")
		return h.contentPublisher.SendData(ctx, []commands.Content{
			{
				ThreadId: c.ThreadId(),
				Text:     "No subscriptions",
			},
		})
	}

	var messages []string
	var msg string
	for _, sub := range h.subscriptions {
		newEntry := fmt.Sprintf("%s: %s\n", sub.Name, sub.Interval)

		if len(msg)+len(newEntry) > 1000 {
			messages = append(messages, msg)
			msg = ""
		}
		msg += newEntry
	}
	if len(msg) > 0 {
		messages = append(messages, msg)
	}

	return h.contentPublisher.SendData(ctx, ge.Map(messages, func(message string) commands.Content {
		return commands.Content{
			Text:     message,
			ThreadId: c.ThreadId(),
		}
	}))
}

func (h *Feed) getSubscriptions(ctx context.Context) ([]Subscription, error) {
	b, err := h.db.List(ctx, h.feeder.TableName())
	if err != nil {
		return nil, err
	}
	var subs []Subscription
	for id, v := range b {
		var sub Subscription
		err = json.Unmarshal([]byte(v), &sub)
		if err != nil {
			return nil, err
		}
		sub.Id = id
		subs = append(subs, sub)
	}
	return subs, nil
}

func (h *Feed) remove(ctx context.Context, cmd Command) error {
	h.logger.Info(
		"removing subscription",
		zap.String("feed", h.feeder.Name()),
		zap.String("name", cmd.SubName()),
		zap.Int("threadId", cmd.ThreadId()),
	)
	if err := h.removeSubscription(ctx, cmd); err != nil {
		return h.contentPublisher.SendData(ctx, []commands.Content{
			{
				ThreadId: cmd.ThreadId(),
				Text:     err.Error(),
			},
		})
	}
	return nil
}

func (h *Feed) removeSubscription(ctx context.Context, cmd Command) error {
	sub := h.findSubscription(cmd.SubName())
	if sub == nil {
		return fmt.Errorf("%s: subscription %s not found", h.feeder.Name(), cmd.SubName())
	}

	err := h.db.Del(ctx, h.feeder.TableName(), sub.Id)
	if err != nil {
		return err
	}
	delete(h.subscriptions, cmd.SubName())
	sub.cancelFunc()

	h.logger.Info(
		"subscription removed",
		zap.String("Feed", h.feeder.Name()),
		zap.String("name", cmd.SubName()),
		zap.Int("threadId", cmd.ThreadId()),
	)
	return h.contentPublisher.SendData(ctx, []commands.Content{
		{
			ThreadId: cmd.ThreadId(),
			Text:     fmt.Sprintf("%s: removed %s", h.feeder.Name(), cmd.SubName()),
		},
	})
}

func (h *Feed) addSubscription(sub *Subscription) {
	key := strings.ToLower(sub.Name)
	h.subscriptions[key] = sub
}

func (h *Feed) findSubscription(name string) *Subscription {
	key := strings.ToLower(name)
	return h.subscriptions[key]
}

func (h *Feed) pollFeed(ctx context.Context, sub *Subscription) {
	ctx, cancel := context.WithCancel(ctx)
	sub.cancelFunc = cancel
	go h.poll(ctx, sub)
}

func (h *Feed) poll(ctx context.Context, sub *Subscription) {
	h.logger.Info(
		"polling",
		zap.String("feed", h.feeder.Name()),
		zap.String("name", sub.Name),
		zap.Int("threadId", sub.ThreadId),
	)
	fetch := func(ctx context.Context, sub *Subscription) {
		stories, err := h.feeder.Fetch(ctx, sub)
		if err != nil {
			h.logger.Error("error fetching contents", zap.Error(err))
		} else if len(stories) > 0 {
			h.logger.Info("sending content", zap.Int("count", len(stories)), zap.Int("threadId", sub.ThreadId))
			_ = h.contentPublisher.SendData(ctx, stories)
		}
		h.logger.Info(
			"finished polling",
			zap.String("feed", h.feeder.Name()),
			zap.String("name", sub.Name),
			zap.Int("threadId", sub.ThreadId),
			zap.Int("new stories", len(stories)),
		)
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
