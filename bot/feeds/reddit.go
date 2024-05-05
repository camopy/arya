package feeds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/turnage/graw/reddit"
	"go.uber.org/zap"

	"github.com/camopy/rss_everything/bot/commands"
	"github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/zaplog"
)

const (
	redditFetchLimit         = 10
	redditSubscriptionsTable = "reddit:subscriptions:"
)

type Reddit struct {
	client        reddit.Bot
	logger        *zaplog.Logger
	db            db.DB
	subscriptions []redditSubscription
	contentCh     chan []commands.Content
}

func NewReddit(logger *zaplog.Logger, contentCh chan []commands.Content, db db.DB, id string, key string, username string, password string) *Reddit {
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

func (u *Reddit) HandleCommand(ctx context.Context, cmd commands.Command) error {
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

func (u *Reddit) parseCommand(cmd commands.Command) (*redditCommand, error) {
	s := strings.Split(cmd.Text, " ")

	c := &redditCommand{
		threadId: cmd.ThreadId,
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

			if c.interval.Minutes() < 60 {
				return nil, commands.ErrInvalidIntervalDuration
			}

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
	u.logger.Info("adding subreddit", zap.String("subreddit", c.subreddit), zap.Int("threadId", c.threadId))
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
	u.logger.Info("subreddit added", zap.String("subreddit", c.subreddit), zap.Int("threadId", c.threadId))
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
	u.logger.Info("listing subscriptions", zap.Int("threadId", c.threadId))
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

	var messages []string
	var msg string
	for _, sub := range subs {
		newEntry := fmt.Sprintf("%s: %s\n", sub.Subreddit, sub.Interval)

		if len(msg)+len(newEntry) > 1000 {
			messages = append(messages, msg)
			msg = ""
		}
		msg += newEntry
	}
	if len(msg) > 0 {
		messages = append(messages, msg)
	}

	u.logger.Info("retrieved subscriptions", zap.String("subscriptions", msg))
	for _, m := range messages {
		u.contentCh <- []commands.Content{
			{
				ThreadId: c.threadId,
				Text:     m,
			},
		}
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
	u.logger.Info("removing subreddit", zap.String("subreddit", cmd.subreddit), zap.Int("threadId", cmd.threadId))
	for _, sub := range u.subscriptions {
		if sub.Subreddit == cmd.subreddit {
			err := u.db.Del(ctx, redditSubscriptionsTable, sub.Id)
			if err != nil {
				return err
			}
			u.logger.Info("subreddit removed", zap.String("subreddit", cmd.subreddit), zap.Int("threadId", cmd.threadId))
			u.contentCh <- []commands.Content{
				{
					ThreadId: cmd.threadId,
					Text:     fmt.Sprintf("reddit: removed %s", cmd.subreddit),
				},
			}
			return nil
		}
	}
	return nil
}

func (u *Reddit) StartReddit(ctx context.Context) {
	u.logger.Info("starting reddit")
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
	u.logger.Info("polling subreddit", zap.String("subreddit", sub.Subreddit), zap.Int("threadId", sub.ThreadId))
	fetch := func(ctx context.Context, sub redditSubscription) {
		posts, err := u.fetch(ctx, sub)
		if err != nil {
			u.logger.Error("error fetching posts", zap.Error(err))
		} else if len(posts) > 0 {
			u.logger.Info("sending posts", zap.Int("count", len(posts)), zap.Int("threadId", sub.ThreadId))
			u.contentCh <- posts
		}
		u.logger.Info("finished polling subreddit", zap.String("subreddit", sub.Subreddit), zap.Int("threadId", sub.ThreadId), zap.Int("new posts", len(posts)))
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

func (u *Reddit) fetch(ctx context.Context, sub redditSubscription) ([]commands.Content, error) {
	u.logger.Info("fetching posts", zap.String("subreddit", sub.Subreddit), zap.Int("threadId", sub.ThreadId))
	harvest, err := u.client.ListingWithParams(sub.Subreddit, map[string]string{
		"limit": strconv.Itoa(redditFetchLimit),
	})
	if err != nil {
		return nil, err
	}
	posts := make([]commands.Content, 0, redditFetchLimit)
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

		u.logger.Info("saved new post", zap.String("post", p.String()))

		posts = append(posts, commands.Content{
			ThreadId: sub.ThreadId,
			Text:     p.String(),
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
