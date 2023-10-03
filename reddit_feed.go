package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/camopy/rss_everything/zaplog"
	"go.uber.org/zap"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/turnage/graw/reddit"
)

const (
	redditFetchLimit         = 10
	redditSubscriptionsTable = "reddit:subscriptions:"
)

type Reddit struct {
	client        reddit.Bot
	logger        *zaplog.Logger
	db            DB
	subscriptions []redditSubscription
	contentCh     chan []Content
}

func NewReddit(logger *zaplog.Logger, contentCh chan []Content, db DB, id string, key string, username string, password string) *Reddit {
	cfg := reddit.BotConfig{
		Agent: "rss_feed:1:0.1 (by /u/BurnInNoia)",
		App: reddit.App{
			ID:       id,
			Secret:   key,
			Username: username,
			Password: password,
		},
		Client: http.DefaultClient,
	}
	bot, err := reddit.NewBot(cfg)
	if err != nil {
		panic(err)
	}

	return &Reddit{
		client:    bot,
		logger:    logger,
		db:        db,
		contentCh: contentCh,
	}
}

func (u *Reddit) HandleCommand(ctx context.Context, cmd command) error {
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

type redditCommand struct {
	threadId  int
	name      string
	subreddit string
	interval  time.Duration
	args      []string
}

func (u *Reddit) parseCommand(cmd command) (*redditCommand, error) {
	s := strings.Split(cmd.text, " ")

	c := &redditCommand{
		threadId: cmd.threadId,
		name:     s[0],
	}

	if len(s) > 1 {
		c.subreddit = s[1]

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

type redditSubscription struct {
	Id        string        `json:"id"`
	Subreddit string        `json:"subreddit"`
	Interval  time.Duration `json:"interval"`
	ThreadId  int           `json:"thread_id"`
}

func (u *Reddit) add(ctx context.Context, c *redditCommand) error {
	if !strings.HasPrefix(c.subreddit, "/r/") {
		c.subreddit = "/r/" + c.subreddit
	}

	sub := &redditSubscription{
		Subreddit: c.subreddit,
		Interval:  c.interval,
		ThreadId:  c.threadId,
	}
	if err := u.saveSubscription(ctx, sub); err != nil {
		return err
	}
	go u.poll(ctx, *sub)
	return nil
}

func (u *Reddit) saveSubscription(ctx context.Context, sub *redditSubscription) error {
	b, err := json.Marshal(sub)
	if err != nil {
		return err
	}
	err = u.db.Add(ctx, redditSubscriptionsTable, b)
	if err != nil {
		return err
	}
	u.subscriptions = append(u.subscriptions, *sub)
	return nil
}

func (u *Reddit) list(ctx context.Context, c *redditCommand) error {
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
		msg += fmt.Sprintf("%s: %s\n", sub.Subreddit, sub.Interval)
	}
	fmt.Printf("reddid: %s\n", msg)
	u.logger.Info("retrieved subscriptions", zap.String("subscriptions", msg))
	u.contentCh <- []Content{
		{
			threadId: c.threadId,
			text:     msg,
		},
	}
	return nil
}

func (u *Reddit) getSubscriptions(ctx context.Context) ([]redditSubscription, error) {
	b, err := u.db.List(ctx, redditSubscriptionsTable)
	if err != nil {
		return nil, err
	}
	var subs []redditSubscription
	for id, v := range b {
		var sub redditSubscription
		err = json.Unmarshal([]byte(v), &sub)
		if err != nil {
			return nil, err
		}
		sub.Id = id
		subs = append(subs, sub)
	}
	return subs, nil
}

func (u *Reddit) remove(ctx context.Context, cmd *redditCommand) error {
	for _, sub := range u.subscriptions {
		if sub.Subreddit == cmd.subreddit {
			err := u.db.Del(ctx, redditSubscriptionsTable, sub.Id)
			if err != nil {
				return err
			}
			u.contentCh <- []Content{
				{
					threadId: cmd.threadId,
					text:     fmt.Sprintf("reddit: removed %s", cmd.subreddit),
				},
			}
			return nil
		}
	}
	return nil
}

func (u *Reddit) StartReddit(ctx context.Context) {
	subs, err := u.getSubscriptions(ctx)
	if err != nil && !u.db.IsErrNotFound(err) {
		panic(err)
	}
	u.subscriptions = subs
	for _, sub := range subs {
		go u.poll(ctx, sub)
	}
}

func (u *Reddit) poll(ctx context.Context, sub redditSubscription) {
	fetch := func(ctx context.Context, sub redditSubscription) {
		posts, err := u.fetch(ctx, sub)
		if err != nil {
			fmt.Println(err)
			u.logger.Error("error fetching posts", zap.Error(err))
		} else if len(posts) > 0 {
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

type redditPost struct {
	ID         string
	Title      string
	Permalink  string
	URL        string
	Score      int32
	Subreddit  string
	CreatedUTC uint64
}

func (p redditPost) String() string {
	redditURL := fmt.Sprintf("https://www.reddit.com%s", p.Permalink)
	var links string
	if p.URL == redditURL {
		links = p.URL
	} else {
		links = fmt.Sprintf(`%s

%s`, p.URL, redditURL)
	}
	return fmt.Sprintf(`/r/%s 
%s - ⬆️%d
%s`, p.Subreddit, p.Title, p.Score, links)
}

func (u *Reddit) fetch(ctx context.Context, sub redditSubscription) ([]Content, error) {
	harvest, err := u.client.ListingWithParams(sub.Subreddit, map[string]string{
		"limit": strconv.Itoa(redditFetchLimit),
	})
	if err != nil {
		return nil, err
	}
	posts := make([]Content, 0, redditFetchLimit)
	for _, post := range harvest.Posts {
		p := redditPost{
			ID:         post.ID,
			Title:      post.Title,
			CreatedUTC: post.CreatedUTC,
			Permalink:  post.Permalink,
			URL:        post.URL,
			Score:      post.Score,
			Subreddit:  post.Subreddit,
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

func (u *Reddit) isNewPost(ctx context.Context, post redditPost) (bool, error) {
	isDubplicate, err := u.isDuplicatePost(ctx, post.ID)
	if err != nil || isDubplicate {
		return false, err
	}
	return !u.isOlderThanADay(post), nil
}

func (u *Reddit) isDuplicatePost(ctx context.Context, id string) (bool, error) {
	s, err := u.db.Get(ctx, fmt.Sprintf("%s:%s", "reddit:posts", id))
	if err != nil && !u.db.IsErrNotFound(err) {
		return false, err
	}
	return s != nil, nil
}

func (u *Reddit) isOlderThanADay(post redditPost) bool {
	createdAt := time.UnixMilli(int64(post.CreatedUTC) * 1000)
	return time.Now().Sub(createdAt) > 24*time.Hour
}

func (u *Reddit) savePost(ctx context.Context, post redditPost) error {
	value, err := json.Marshal(post)
	if err != nil {
		return err
	}
	return u.db.Set(ctx, fmt.Sprintf("%s:%s", "reddit:posts", post.ID), value, 7*24*time.Hour)
}
