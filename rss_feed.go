package main

import (
	"context"
	"encoding/json"
	"fmt"
	db2 "github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/zaplog"
	"github.com/mmcdole/gofeed"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"time"
)

const (
	rssFetchLimit         = 10
	rssSubscriptionsTable = "rss:subscriptions:"
)

type RSS struct {
	client        *gofeed.Parser
	logger        *zaplog.Logger
	db            db2.DB
	subscriptions []rssSubscription
	contentCh     chan []Content
}

func NewRSS(logger *zaplog.Logger, contentCh chan []Content, db db2.DB) *RSS {
	return &RSS{
		client:    gofeed.NewParser(),
		logger:    logger,
		db:        db,
		contentCh: contentCh,
	}
}

func (u *RSS) HandleCommand(ctx context.Context, cmd command) error {
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

type rssCommand struct {
	threadId  int
	name      string
	feedTitle string
	interval  time.Duration
	args      []string
}

func (u *RSS) parseCommand(cmd command) (*rssCommand, error) {
	s := strings.Split(cmd.text, " ")

	c := &rssCommand{
		threadId: cmd.threadId,
		name:     s[0],
	}

	if len(s) > 1 {
		c.feedTitle = s[1]

		if len(s) > 2 {
			interval, err := strconv.Atoi(s[2])
			if err != nil {
				return nil, fmt.Errorf("reddit: invalid interval")
			}
			c.interval = time.Duration(interval) * time.Minute

			if len(s) > 3 {
				c.args = s[3:]
			}
		}
	}

	return c, nil
}

type rssSubscription struct {
	Id        string        `json:"id"`
	FeedTitle string        `json:"feed_title"`
	Url       string        `json:"url"`
	Interval  time.Duration `json:"interval"`
	ThreadId  int           `json:"thread_id"`
}

func (u *RSS) add(ctx context.Context, c *rssCommand) error {
	sub := &rssSubscription{
		Url:       c.args[0],
		FeedTitle: c.feedTitle,
		Interval:  c.interval,
		ThreadId:  c.threadId,
	}
	if err := u.saveSubscription(ctx, sub); err != nil {
		return err
	}
	go u.poll(ctx, *sub)
	return nil
}

func (u *RSS) saveSubscription(ctx context.Context, sub *rssSubscription) error {
	b, err := json.Marshal(sub)
	if err != nil {
		return err
	}
	err = u.db.Add(ctx, rssSubscriptionsTable, b)
	if err != nil {
		return err
	}
	u.subscriptions = append(u.subscriptions, *sub)
	return nil
}

func (u *RSS) list(ctx context.Context, c *rssCommand) error {
	subs, err := u.getSubscriptions(ctx)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		u.logger.Info("no subscriptions")
		u.contentCh <- []Content{
			{
				threadId: c.threadId,
				text:     "No subscriptions",
			},
		}
		return nil
	}
	var msg string
	for _, sub := range subs {
		msg += fmt.Sprintf("%s: %s\n", sub.Url, sub.Interval)
	}
	u.logger.Info("retrieved subscriptions", zap.String("subscriptions", msg))
	u.contentCh <- []Content{
		{
			threadId: c.threadId,
			text:     msg,
		},
	}
	return nil
}

func (u *RSS) getSubscriptions(ctx context.Context) ([]rssSubscription, error) {
	b, err := u.db.List(ctx, rssSubscriptionsTable)
	if err != nil {
		return nil, err
	}
	var subs []rssSubscription
	for id, v := range b {
		var sub rssSubscription
		err = json.Unmarshal([]byte(v), &sub)
		if err != nil {
			return nil, err
		}
		sub.Id = id
		subs = append(subs, sub)
	}
	return subs, nil
}

func (u *RSS) remove(ctx context.Context, cmd *rssCommand) error {
	for _, sub := range u.subscriptions {
		if sub.FeedTitle == cmd.feedTitle {
			err := u.db.Del(ctx, rssSubscriptionsTable, sub.Id)
			if err != nil {
				return err
			}
			u.contentCh <- []Content{
				{
					threadId: cmd.threadId,
					text:     fmt.Sprintf("reddit: removed %s", cmd.feedTitle),
				},
			}
			return nil
		}
	}
	return nil
}

func (u *RSS) StartRSS(ctx context.Context) {
	subs, err := u.getSubscriptions(ctx)
	if err != nil && !u.db.IsErrNotFound(err) {
		panic(err)
	}
	u.subscriptions = subs
	for _, sub := range subs {
		go u.poll(ctx, sub)
	}
}

func (u *RSS) poll(ctx context.Context, sub rssSubscription) {
	fetch := func(ctx context.Context, sub rssSubscription) {
		posts, err := u.fetch(ctx, sub)
		if err != nil {
			fmt.Println(err)
			u.logger.Error("fetch error", zap.Error(err))
		} else if len(posts) > 0 {
			fmt.Printf("rss: sending %d posts to thread %d\n", len(posts), sub.ThreadId)
			u.logger.Info("sending posts", zap.Int("count", len(posts)), zap.Int("threadId", sub.ThreadId))
			u.contentCh <- posts
		}
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

type rssPost struct {
	FeedTitle  string
	ID         string
	Title      string
	Permalink  string
	CreatedUTC uint64
}

func (p rssPost) String() string {
	return fmt.Sprintf(`%s
%s`, p.Title, p.Permalink)
}

func (u *RSS) fetch(ctx context.Context, sub rssSubscription) ([]Content, error) {
	feed, err := u.client.ParseURLWithContext(sub.Url, ctx)
	if err != nil {
		return nil, err
	}
	posts := make([]Content, 0, rssFetchLimit)
	for _, post := range feed.Items {
		p := rssPost{
			FeedTitle:  sub.FeedTitle,
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

		posts = append(posts, Content{
			threadId: sub.ThreadId,
			text:     p.String(),
		})
	}

	return posts, nil
}

func (u *RSS) isNewPost(ctx context.Context, post rssPost) (bool, error) {
	isDubplicate, err := u.isDuplicatePost(ctx, post.FeedTitle, post.ID)
	if err != nil || isDubplicate {
		return false, err
	}
	return !u.isOlderThanADay(post), nil
}

func (u *RSS) isDuplicatePost(ctx context.Context, feedId, id string) (bool, error) {
	s, err := u.db.Get(ctx, fmt.Sprintf("rss:%s:posts:%s", feedId, id))
	if err != nil && !u.db.IsErrNotFound(err) {
		return false, err
	}
	return s != nil, nil
}

func (u *RSS) isOlderThanADay(post rssPost) bool {
	createdAt := time.UnixMilli(int64(post.CreatedUTC) * 1000)
	return time.Now().Sub(createdAt) > 24*time.Hour
}

func (u *RSS) savePost(ctx context.Context, post rssPost) error {
	value, err := json.Marshal(post)
	if err != nil {
		return err
	}
	return u.db.Set(ctx, fmt.Sprintf("rss:%s:posts:%s", post.FeedTitle, post.ID), value, 7*24*time.Hour)
}
