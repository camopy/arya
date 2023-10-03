package feeds

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
	"github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/zaplog"
)

const (
	rssFetchLimit         = 10
	rssSubscriptionsTable = "rss:subscriptions:"
)

type RSS struct {
	client        *gofeed.Parser
	logger        *zaplog.Logger
	db            db.DB
	subscriptions []rssSubscription
	contentCh     chan []commands.Content
}

func NewRSS(logger *zaplog.Logger, contentCh chan []commands.Content, db db.DB) *RSS {
	return &RSS{
		client:    gofeed.NewParser(),
		logger:    logger,
		db:        db,
		contentCh: contentCh,
	}
}

func (u *RSS) HandleCommand(ctx context.Context, cmd commands.Command) error {
	u.logger.Info("handling command", zap.String("command", cmd.Text))
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

func (u *RSS) parseCommand(cmd commands.Command) (*rssCommand, error) {
	s := strings.Split(cmd.Text, " ")

	c := &rssCommand{
		threadId: cmd.ThreadId,
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
	u.logger.Info(
		"adding subscription",
		zap.String("feedTitle", c.feedTitle),
		zap.String("url", c.args[0]),
		zap.Duration("interval", c.interval),
		zap.Int("threadId", c.threadId),
	)
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
	u.logger.Info("subscription added", zap.String("subscription", string(b)))
	u.subscriptions = append(u.subscriptions, *sub)
	return nil
}

func (u *RSS) list(ctx context.Context, c *rssCommand) error {
	u.logger.Info("listing subscriptions")
	subs, err := u.getSubscriptions(ctx)
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		u.logger.Info("no subscriptions")
		u.contentCh <- []commands.Content{
			{
				ThreadId: c.threadId,
				Text:     "No subscriptions",
			},
		}
		return nil
	}
	var msg string
	for _, sub := range subs {
		msg += fmt.Sprintf("%s: %s\n", sub.Url, sub.Interval)
	}
	u.logger.Info("retrieved subscriptions", zap.String("subscriptions", msg))
	u.contentCh <- []commands.Content{
		{
			ThreadId: c.threadId,
			Text:     msg,
		},
	}
	return nil
}

func (u *RSS) getSubscriptions(ctx context.Context) ([]rssSubscription, error) {
	u.logger.Info("retrieving subscriptions")
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
	u.logger.Info("removing subscription", zap.String("feedTitle", cmd.feedTitle))
	for _, sub := range u.subscriptions {
		if sub.FeedTitle == cmd.feedTitle {
			err := u.db.Del(ctx, rssSubscriptionsTable, sub.Id)
			if err != nil {
				return err
			}
			u.logger.Info("subscription removed", zap.String("feedTitle", cmd.feedTitle))
			u.contentCh <- []commands.Content{
				{
					ThreadId: cmd.threadId,
					Text:     fmt.Sprintf("reddit: removed %s", cmd.feedTitle),
				},
			}
			return nil
		}
	}
	return nil
}

func (u *RSS) StartRSS(ctx context.Context) {
	u.logger.Info("starting rss")
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
	u.logger.Info("polling", zap.String("url", sub.Url))
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
		u.logger.Info("polling done", zap.String("url", sub.Url), zap.Int("threadId", sub.ThreadId), zap.Int("new posts", len(posts)))
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

func (u *RSS) fetch(ctx context.Context, sub rssSubscription) ([]commands.Content, error) {
	u.logger.Info("fetching", zap.String("url", sub.Url))
	feed, err := u.client.ParseURLWithContext(sub.Url, ctx)
	if err != nil {
		return nil, err
	}
	posts := make([]commands.Content, 0, rssFetchLimit)
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

		u.logger.Info("saved new post", zap.String("post", p.Title))

		posts = append(posts, commands.Content{
			ThreadId: sub.ThreadId,
			Text:     p.String(),
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
