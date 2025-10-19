package scrapper

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/camopy/rss_everything/bot/feeder"
	"github.com/camopy/rss_everything/bot/models"
	"github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/zaplog"
)

const (
	scrapperSubscriptionsTable = "scrapper:subscriptions:"
	OlxPlatform                = "olx"
	ZapImoveisPlatform         = "zap_imoveis"
)

type Scrapper struct {
	logger *zaplog.Logger
	db     db.DB
}

func New(logger *zaplog.Logger, db db.DB) feeder.Feeder {
	return &Scrapper{
		logger: logger,
		db:     db,
	}
}

func (u *Scrapper) Name() string {
	return "scrapper"
}

func (u *Scrapper) TableName() string {
	return scrapperSubscriptionsTable
}

type scrapperCommand struct {
	threadId int
	action   string
	platform string
	title    string
	url      string
	interval time.Duration
}

func (s scrapperCommand) Action() string {
	return s.action
}

func (s scrapperCommand) Interval() time.Duration {
	return s.interval
}

func (s scrapperCommand) ThreadId() int {
	return s.threadId
}

func (s scrapperCommand) SubName() string {
	return s.title
}

func (s scrapperCommand) Platform() string {
	return s.platform
}

func (s scrapperCommand) Url() string {
	return s.url
}

func (u *Scrapper) ParseCommand(cmd models.Command) (models.Commander, error) {
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
		action:   s[0],
	}, nil
}

func parseRemoveCommand(threadId int, s []string) (*scrapperCommand, error) {
	if len(s) < 2 {
		return nil, fmt.Errorf("scrapper: invalid arguments")
	}

	return &scrapperCommand{
		threadId: threadId,
		action:   s[0],
		title:    s[1],
	}, nil
}

func parseAddCommand(threadId int, s []string) (*scrapperCommand, error) {
	if len(s) < 5 {
		return nil, fmt.Errorf("scrapper: invalid arguments")
	}

	c := &scrapperCommand{
		threadId: threadId,
		action:   s[0],
		platform: s[1],
		title:    s[2],
		url:      s[3],
	}

	interval, err := strconv.Atoi(s[4])
	if err != nil {
		return nil, fmt.Errorf("scrapper: invalid interval")
	}
	c.interval = time.Duration(interval) * time.Minute

	if c.interval.Minutes() < 60 {
		return nil, models.ErrInvalidIntervalDuration
	}

	return c, nil
}

func (u *Scrapper) Fetch(ctx context.Context, sub *models.Subscription) ([]models.Content, error) {
	u.logger.Info("scrapping", zap.String("url", sub.Url), zap.Int("threadId", sub.ThreadId))
	switch sub.Platform {
	case OlxPlatform:
		olxScrapper := NewOlx(u.logger.Named("olx"), u.db)
		return olxScrapper.scrap(ctx, sub.ThreadId, sub.Url)
	case ZapImoveisPlatform:
		zapImoveisScrapper := NewZapImoveis(u.logger.Named("zap-imoveis"), u.db)
		return zapImoveisScrapper.scrap(ctx, sub.ThreadId, sub.Url)
	}

	return nil, fmt.Errorf("scrapper: invalid platform")
}
