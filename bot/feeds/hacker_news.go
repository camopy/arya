package feeds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/camopy/rss_everything/bot/commands"
	"github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/metrics"
	ge "github.com/camopy/rss_everything/util/generics"
	"github.com/camopy/rss_everything/util/psub"
	"github.com/camopy/rss_everything/util/run"
	"github.com/camopy/rss_everything/zaplog"
)

const (
	hackerNews                   = "hacker_news"
	defaultFetchInterval         = 30 * time.Minute
	topStoriesLimit              = 20
	hackerNewsSubscriptionsTable = "hackernews:subscriptions:"

	topStoriesEndpoint = "https://hacker-news.firebaseio.com/v0/topstories.json"
	storyEndpoint      = "https://hacker-news.firebaseio.com/v0/item/%d.json"
)

var hackerNewsMetrics = struct {
	storiesLoadedTotal *prometheus.GaugeVec
	loadStoriesTotal   *prometheus.CounterVec
}{
	storiesLoadedTotal: metrics.NewGaugeVec(
		hackerNews,
		"stories_loaded_total",
		"Total number of stories loaded",
		[]string{},
	),
	loadStoriesTotal: metrics.NewCounterVec(
		hackerNews,
		"load_stories_total",
		"Total number of load stories requests",
		[]string{},
	),
}

func trackLoadedStories(storiesLoaded int) {
	hackerNewsMetrics.storiesLoadedTotal.WithLabelValues().Set(float64(storiesLoaded))
	hackerNewsMetrics.loadStoriesTotal.WithLabelValues().Inc()
}

type HackerNews struct {
	*http.Client
	logger        *zaplog.Logger
	db            db.DB
	subscriptions map[string]*hackerNewsSubscription

	contentPublisher psub.Publisher[[]commands.Content]
}

type hackerNewsSubscription struct {
	Id       string        `json:"id"`
	Name     string        `json:"name"`
	Interval time.Duration `json:"interval"`
	ThreadId int           `json:"thread_id"`

	cancelFunc context.CancelFunc
}

func NewHackerNews(logger *zaplog.Logger, contentPublisher psub.Publisher[[]commands.Content], db db.DB) *HackerNews {
	return &HackerNews{
		Client:        http.DefaultClient,
		logger:        logger,
		db:            db,
		subscriptions: make(map[string]*hackerNewsSubscription),

		contentPublisher: contentPublisher,
	}
}

func (h *HackerNews) Name() string {
	return "hacker-news"
}

func (h *HackerNews) Start(ctx run.Context) error {
	h.logger.Info("starting hacker news")
	subs, err := h.getSubscriptions(ctx)
	if err != nil && !h.db.IsErrNotFound(err) {
		panic(err)
	}

	for i := range subs {
		sub := subs[i]
		h.addSubscription(&sub)
		h.pollHackerNews(ctx, &sub)
	}

	return nil
}

func (h *HackerNews) HandleCommand(ctx context.Context, cmd commands.Command) error {
	c, err := h.parseCommand(cmd)
	if err != nil {
		return err
	}
	switch c.name {
	case "add":
		err = h.add(ctx, c)
	case "list":
		err = h.list(ctx, c)
	case "remove":
		err = h.remove(ctx, c)
	}
	return err
}

type hackerNewsCommand struct {
	threadId int
	name     string
	subName  string
	interval time.Duration
	args     []string
}

func (h *HackerNews) parseCommand(cmd commands.Command) (*hackerNewsCommand, error) {
	s := strings.Split(cmd.Text, " ")

	c := &hackerNewsCommand{
		threadId: cmd.ThreadId,
		name:     s[0],
	}

	if len(s) > 1 {
		c.subName = s[1]
	}

	return c, nil
}

func (h *HackerNews) add(ctx context.Context, c *hackerNewsCommand) error {
	h.logger.Info("adding hackernews subscription", zap.String("name", c.subName), zap.Int("threadId", c.threadId))

	if c.interval == 0 {
		c.interval = defaultFetchInterval
	}

	sub := hackerNewsSubscription{
		Name:     c.subName,
		Interval: c.interval,
		ThreadId: c.threadId,
	}
	if err := h.saveSubscription(ctx, sub); err != nil {
		return err
	}
	h.addSubscription(&sub)
	h.logger.Info("hackernews subscription added", zap.String("name", c.subName), zap.Int("threadId", c.threadId))

	h.pollHackerNews(ctx, &sub)
	return nil
}

func (h *HackerNews) saveSubscription(ctx context.Context, sub hackerNewsSubscription) error {
	b, err := json.Marshal(sub)
	if err != nil {
		return err
	}
	err = h.db.Add(ctx, hackerNewsSubscriptionsTable, b)
	if err != nil {
		return err
	}
	return nil
}

func (h *HackerNews) list(ctx context.Context, c *hackerNewsCommand) error {
	h.logger.Info("listing subscriptions", zap.Int("threadId", c.threadId))

	if len(h.subscriptions) == 0 {
		h.logger.Info("no subscriptions")
		return h.contentPublisher.SendData(ctx, []commands.Content{
			{
				ThreadId: c.threadId,
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
			ThreadId: c.threadId,
		}
	}))
}

func (h *HackerNews) getSubscriptions(ctx context.Context) ([]hackerNewsSubscription, error) {
	b, err := h.db.List(ctx, hackerNewsSubscriptionsTable)
	if err != nil {
		return nil, err
	}
	var subs []hackerNewsSubscription
	for id, v := range b {
		var sub hackerNewsSubscription
		err = json.Unmarshal([]byte(v), &sub)
		if err != nil {
			return nil, err
		}
		sub.Id = id
		subs = append(subs, sub)
	}
	return subs, nil
}

func (h *HackerNews) remove(ctx context.Context, cmd *hackerNewsCommand) error {
	h.logger.Info("removing hackernews subscription", zap.String("name", cmd.subName), zap.Int("threadId", cmd.threadId))
	if err := h.removeSubscription(ctx, cmd); err != nil {
		return h.contentPublisher.SendData(ctx, []commands.Content{
			{
				ThreadId: cmd.threadId,
				Text:     err.Error(),
			},
		})
	}
	return nil
}

func (h *HackerNews) removeSubscription(ctx context.Context, cmd *hackerNewsCommand) error {
	sub := h.findSubscription(cmd.subName)
	if sub == nil {
		return fmt.Errorf("hackernews: subscription %s not found", cmd.subName)
	}

	err := h.db.Del(ctx, hackerNewsSubscriptionsTable, sub.Id)
	if err != nil {
		return err
	}
	delete(h.subscriptions, cmd.subName)
	sub.cancelFunc()

	h.logger.Info("hackernews subscription removed", zap.String("name", cmd.subName), zap.Int("threadId", cmd.threadId))
	return h.contentPublisher.SendData(ctx, []commands.Content{
		{
			ThreadId: cmd.threadId,
			Text:     fmt.Sprintf("hackernews: removed %s", cmd.subName),
		},
	})
}

func (h *HackerNews) addSubscription(sub *hackerNewsSubscription) {
	key := strings.ToLower(sub.Name)
	h.subscriptions[key] = sub
}

func (h *HackerNews) findSubscription(name string) *hackerNewsSubscription {
	key := strings.ToLower(name)
	return h.subscriptions[key]
}

func (h *HackerNews) pollHackerNews(ctx context.Context, sub *hackerNewsSubscription) {
	ctx, cancel := context.WithCancel(ctx)
	sub.cancelFunc = cancel
	go h.poll(ctx, sub)
}

func (h *HackerNews) poll(ctx context.Context, sub *hackerNewsSubscription) {
	h.logger.Info("polling hackernews", zap.String("name", sub.Name), zap.Int("threadId", sub.ThreadId))
	fetch := func(ctx context.Context, sub *hackerNewsSubscription) {
		stories, err := h.fetch(ctx, sub)
		if err != nil {
			h.logger.Error("error fetching stories", zap.Error(err))
		} else if len(stories) > 0 {
			h.logger.Info("sending stories", zap.Int("count", len(stories)), zap.Int("threadId", sub.ThreadId))
			_ = h.contentPublisher.SendData(ctx, stories)
		}
		h.logger.Info("finished polling hackernews", zap.String("name", sub.Name), zap.Int("threadId", sub.ThreadId), zap.Int("new stories", len(stories)))
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

func (h *HackerNews) fetch(ctx context.Context, sub *hackerNewsSubscription) ([]commands.Content, error) {
	h.logger.Info("fetching hacker news")
	ids, err := h.fetchTopStoriesIds()
	if err != nil {
		return nil, err
	}
	stories := make([]commands.Content, 0, topStoriesLimit)
	i := 0
	for i < len(ids) && i < topStoriesLimit {
		id := ids[i]
		i++
		isDuplicate, err := h.isDuplicateStory(ctx, strconv.Itoa(id))
		if err != nil {
			return nil, err
		}
		if isDuplicate {
			continue
		}
		story, err := h.fetchStory(id)
		if err != nil {
			h.logger.Error("fetch story error", zap.Error(err))
			continue
		}
		s := story.String()
		value, err := json.Marshal(s)
		if err != nil {
			return nil, err
		}
		err = h.db.Set(ctx, fmt.Sprintf("%s:%s", hackerNews, strconv.Itoa(id)), value, 24*7*time.Hour)
		if err != nil {
			return nil, err
		}
		h.logger.Info("saved story", zap.String("title", story.Title))
		stories = append(stories, commands.Content{
			Text:     s,
			ThreadId: sub.ThreadId,
		})
	}
	trackLoadedStories(len(stories))
	h.logger.Info("fetched hacker news", zap.Int("stories", len(stories)))
	return stories, nil
}

func (h *HackerNews) isDuplicateStory(ctx context.Context, id string) (bool, error) {
	s, err := h.db.Get(ctx, fmt.Sprintf("%s:%s", hackerNews, id))
	if err != nil && !h.db.IsErrNotFound(err) {
		return false, err
	}
	return s != nil, nil
}

func (h *HackerNews) fetchTopStoriesIds() ([]int, error) {
	start := time.Now()
	resp, err := h.Get(topStoriesEndpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	metrics.TrackExternalRequest(http.MethodGet, resp.Request.URL.Host, resp.StatusCode, time.Since(start))

	var ids []int
	if err := json.NewDecoder(resp.Body).Decode(&ids); err != nil {
		return nil, err
	}

	return ids, nil
}

type Story struct {
	By          string `json:"by"`
	Descendants int    `json:"descendants"`
	Id          int    `json:"id"`
	Kids        []int  `json:"kids"`
	Score       int    `json:"score"`
	Time        int    `json:"time"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	Url         string `json:"url"`
}

func (h *HackerNews) fetchStory(id int) (*Story, error) {
	start := time.Now()
	resp, err := h.Get(fmt.Sprintf(storyEndpoint, id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	metrics.TrackExternalRequest(http.MethodGet, resp.Request.URL.Host, resp.StatusCode, time.Since(start))

	var s Story
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}

	return &s, nil
}

func (s *Story) String() string {
	hnUrl := "https://news.ycombinator.com/item?id="
	return fmt.Sprintf(`
HN: %s - ⬆️%d
%s

%s
	`, s.Title, s.Score, s.Url, fmt.Sprintf("%s%d", hnUrl, s.Id))
}
