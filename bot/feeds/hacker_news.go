package feeds

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"

	"github.com/camopy/rss_everything/bot/commands"
	"github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/metrics"
	"github.com/camopy/rss_everything/zaplog"
)

const (
	hackerNews      = "hacker_news"
	fetchInterval   = 30 * time.Minute
	topStoriesLimit = 20

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
	logger    *zaplog.Logger
	db        db.DB
	contentCh chan []commands.Content
	threadId  int
}

func NewHackerNews(logger *zaplog.Logger, contentCh chan []commands.Content, db db.DB, threadId int) *HackerNews {
	return &HackerNews{
		Client:    http.DefaultClient,
		logger:    logger,
		db:        db,
		contentCh: contentCh,
		threadId:  threadId,
	}
}

func (h *HackerNews) StartHackerNews() {
	for {
		stories, err := h.fetch(context.Background())
		if err != nil {
			h.logger.Error("fetch error", zap.Error(err))
		} else if len(stories) > 0 {
			h.logger.Info("sending stories", zap.Int("count", len(stories)), zap.Int("threadId", h.threadId))
			h.contentCh <- stories
		}
		time.Sleep(fetchInterval)
	}
}

func (h *HackerNews) fetch(ctx context.Context) ([]commands.Content, error) {
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
		stories = append(stories, commands.Content{
			Text:     s,
			ThreadId: h.threadId,
		})
	}
	trackLoadedStories(len(stories))
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
	return fmt.Sprintf(`
HN: %s - ⬆️%d
%s
	`, s.Title, s.Score, s.Url)
}
