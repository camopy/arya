package reddit

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
	"github.com/camopy/rss_everything/bot/feeds"
	"github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/zaplog"
)

const (
	redditFetchLimit         = 10
	redditSubscriptionsTable = "reddit:subscriptions:"
)

type Reddit struct {
	client reddit.Bot
	logger *zaplog.Logger
	db     db.DB
}

func New(logger *zaplog.Logger, db db.DB, id string, key string, username string, password string) feeds.Feeder {
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
		client: bot,
		logger: logger,
		db:     db,
	}
}

func (r *Reddit) Name() string {
	return "reddit"
}

func (r *Reddit) TableName() string {
	return redditSubscriptionsTable
}

type redditCommand struct {
	threadId  int
	action    string
	subreddit string
	interval  time.Duration
	args      []string
}

func (r redditCommand) Action() string {
	return r.action
}

func (r redditCommand) Interval() time.Duration {
	return r.interval
}

func (r redditCommand) ThreadId() int {
	return r.threadId
}

func (r redditCommand) SubName() string {
	return r.subreddit
}

func (r *Reddit) ParseCommand(cmd commands.Command) (feeds.Command, error) {
	s := strings.Split(cmd.Text, " ")

	c := &redditCommand{
		threadId: cmd.ThreadId,
		action:   s[0],
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

func (r *Reddit) Fetch(ctx context.Context, sub *feeds.Subscription) ([]commands.Content, error) {
	r.logger.Info("fetching posts", zap.String("subreddit", sub.Name), zap.Int("threadId", sub.ThreadId))
	harvest, err := r.client.ListingWithParams(sub.Name, map[string]string{
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

		isNewPost, err := r.isNewPost(ctx, p)
		if err != nil {
			return nil, err
		}
		if !isNewPost {
			continue
		}

		if err := r.savePost(ctx, p); err != nil {
			return nil, err
		}

		r.logger.Info("saved new post", zap.String("post", p.String()))

		posts = append(posts, commands.Content{
			ThreadId: sub.ThreadId,
			Text:     p.String(),
		})
	}

	return posts, nil
}

func (r *Reddit) isNewPost(ctx context.Context, post redditPost) (bool, error) {
	isDubplicate, err := r.isDuplicatePost(ctx, post.ID)
	if err != nil || isDubplicate {
		return false, err
	}
	return !r.isOlderThanADay(post), nil
}

func (r *Reddit) isDuplicatePost(ctx context.Context, id string) (bool, error) {
	s, err := r.db.Get(ctx, fmt.Sprintf("%s:%s", "reddit:posts", id))
	if err != nil && !r.db.IsErrNotFound(err) {
		return false, err
	}
	return s != nil, nil
}

func (r *Reddit) isOlderThanADay(post redditPost) bool {
	createdAt := time.UnixMilli(int64(post.CreatedUTC) * 1000)
	return time.Now().Sub(createdAt) > 24*time.Hour
}

func (r *Reddit) savePost(ctx context.Context, post redditPost) error {
	value, err := json.Marshal(post)
	if err != nil {
		return err
	}
	return r.db.Set(ctx, fmt.Sprintf("%s:%s", "reddit:posts", post.ID), value, 7*24*time.Hour)
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
