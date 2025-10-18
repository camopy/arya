package hacker_news

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
	"github.com/camopy/rss_everything/bot/feeder"
	"github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/metrics"
	"github.com/camopy/rss_everything/zaplog"
)

const (
	hackerNews                   = "hacker_news"
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

func New(logger *zaplog.Logger, db db.DB) feeder.Feeder {
	return &HackerNews{
		Client: http.DefaultClient,
		logger: logger,
		db:     db,
	}
}

type HackerNews struct {
	*http.Client
	logger *zaplog.Logger
	db     db.DB
}

func (h *HackerNews) Name() string {
	return "hacker-news"
}

func (h *HackerNews) TableName() string {
	return hackerNewsSubscriptionsTable
}

type hackerNewsCommand struct {
	threadId int
	action   string
	subName  string
	interval time.Duration
}

func (c hackerNewsCommand) ThreadId() int {
	return c.threadId
}

func (c hackerNewsCommand) Action() string {
	return c.action
}

func (c hackerNewsCommand) SubName() string {
	return c.subName
}

func (c hackerNewsCommand) Interval() time.Duration {
	return c.interval
}

func (c hackerNewsCommand) Url() string {
	return ""
}

func (h *HackerNews) ParseCommand(cmd commands.Command) (feeder.Command, error) {
	s := strings.Split(cmd.Text, " ")

	c := &hackerNewsCommand{
		threadId: cmd.ThreadId,
		action:   s[0],
	}

	if len(s) > 1 {
		c.subName = s[1]
	}

	return c, nil
}

func (h *HackerNews) Fetch(ctx context.Context, sub *feeder.Subscription) ([]commands.Content, error) {
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
