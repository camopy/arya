package rss

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"go.uber.org/zap"

	"github.com/camopy/rss_everything/bot/commands"
	"github.com/camopy/rss_everything/bot/feeder"
	"github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/zaplog"
)

const (
	rssFetchLimit         = 10
	rssSubscriptionsTable = "rss:subscriptions:"
)

type RSS struct {
	client *gofeed.Parser
	logger *zaplog.Logger
	db     db.DB
}

func New(logger *zaplog.Logger, db db.DB) feeder.Feeder {
	return &RSS{
		client: gofeed.NewParser(),
		logger: logger,
		db:     db,
	}
}

func (u *RSS) Name() string {
	return "rss"
}

func (u *RSS) TableName() string {
	return rssSubscriptionsTable
}

type rssCommand struct {
	threadId  int
	action    string
	feedTitle string
	interval  time.Duration
	url       string
}

func (r rssCommand) Action() string {
	return r.action
}

func (r rssCommand) Interval() time.Duration {
	return r.interval
}

func (r rssCommand) ThreadId() int {
	return r.threadId
}

func (r rssCommand) SubName() string {
	return r.feedTitle
}

func (r rssCommand) Url() string {
	return r.url
}

func (u *RSS) ParseCommand(cmd commands.Command) (feeder.Command, error) {
	// /rss operation[add|remove|list] feed_title interval[m] url
	s := strings.Split(cmd.Text, " ")

	c := &rssCommand{
		threadId: cmd.ThreadId,
		action:   s[0],
	}

	if len(s) > 1 {
		c.feedTitle = s[1]

		if len(s) > 2 {
			interval, err := strconv.Atoi(s[2])
			if err != nil {
				return nil, fmt.Errorf("rss: invalid interval")
			}
			c.interval = time.Duration(interval) * time.Minute

			if c.interval.Minutes() < 60 {
				return nil, commands.ErrInvalidIntervalDuration
			}

			if len(s) > 3 {
				c.url = s[3]
			}
		}
	}

	return c, nil
}

func (u *RSS) Fetch(ctx context.Context, sub *feeder.Subscription) ([]commands.Content, error) {
	u.logger.Info("fetching", zap.String("url", sub.Name))
	feed, err := u.client.ParseURLWithContext(sub.Url, ctx)
	if err != nil {
		return nil, err
	}
	posts := make([]commands.Content, 0, rssFetchLimit)
	for _, post := range feed.Items {
		p := rssPost{
			FeedTitle:  sub.Name,
			ID:         post.GUID,
			Title:      post.Title,
			CreatedUTC: uint64(post.PublishedParsed.UTC().UnixMilli()),
			Permalink:  post.Link,
		}

		isNewPost, err := u.isNewPost(ctx, p)
		if err != nil {
			return nil, err
		}
		if !isNewPost {
			continue
		}

		if err := u.savePost(ctx, p); err != nil {
			return nil, err
		}

		u.logger.Info("saved new post", zap.String("post", p.Title))

		posts = append(posts, commands.Content{
			ThreadId: sub.ThreadId,
			Text:     p.String(),
		})
	}

	return posts, nil
}

func (u *RSS) isNewPost(ctx context.Context, post rssPost) (bool, error) {
	isDuplicatePost, err := u.isDuplicatePost(ctx, post.FeedTitle, post.ID)
	if err != nil || isDuplicatePost {
		return false, err
	}
	return !post.isOlderThanADay(), nil
}

func (u *RSS) isDuplicatePost(ctx context.Context, feedId, id string) (bool, error) {
	s, err := u.db.Get(ctx, fmt.Sprintf("rss:%s:posts:%s", feedId, id))
	if err != nil && !u.db.IsErrNotFound(err) {
		return false, err
	}
	return s != nil, nil
}

func (p *rssPost) isOlderThanADay() bool {
	createdAt := time.UnixMilli(int64(p.CreatedUTC))
	if createdAt.IsZero() {
		return true
	}
	return time.Now().Sub(createdAt) > 24*time.Hour
}

func (u *RSS) savePost(ctx context.Context, post rssPost) error {
	value, err := json.Marshal(post)
	if err != nil {
		return err
	}
	return u.db.Set(ctx, fmt.Sprintf("rss:%s:posts:%s", post.FeedTitle, post.ID), value, 7*24*time.Hour)
}

type rssPost struct {
	FeedTitle  string
	ID         string
	Title      string
	Permalink  string
	CreatedUTC uint64
}

func (p *rssPost) String() string {
	createdAt := time.UnixMilli(int64(p.CreatedUTC))
	return fmt.Sprintf(`%s - %s
%s`, p.Title, createdAt.Format(time.RFC822Z), p.Permalink)
}
