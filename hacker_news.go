package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	name            = "hacker_news"
	fetchInterval   = 30 * time.Minute
	topStoriesLimit = 20

	topStoriesEndpoint = "https://hacker-news.firebaseio.com/v0/topstories.json"
	storyEndpoint      = "https://hacker-news.firebaseio.com/v0/item/%d.json"
)

type HackerNews struct {
	*http.Client
	db        DB
	contentCh chan []string
}

func NewHackerNews(contentCh chan []string, db DB) *HackerNews {
	return &HackerNews{
		Client:    http.DefaultClient,
		db:        db,
		contentCh: contentCh,
	}
}

func (h *HackerNews) StartHackerNews() {
	for {
		stories, err := h.fetch(context.Background())
		if err != nil {
			log.Println(err)
		} else if len(stories) > 0 {
			h.contentCh <- stories
		}
		time.Sleep(fetchInterval)
	}
}

func (h *HackerNews) fetch(ctx context.Context) ([]string, error) {
	ids, err := h.fetchTopStoriesIds()
	if err != nil {
		return nil, err
	}
	stories := make([]string, 0, topStoriesLimit)
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
			log.Println(err)
			continue
		}
		s := story.String()
		value, err := json.Marshal(s)
		if err != nil {
			return nil, err
		}
		err = h.db.Put(ctx, fmt.Sprintf("%s:%s", name, strconv.Itoa(id)), value)
		if err != nil {
			return nil, err
		}
		stories = append(stories, s)
	}
	return stories, nil
}

func (h *HackerNews) isDuplicateStory(ctx context.Context, id string) (bool, error) {
	s, err := h.db.Get(ctx, fmt.Sprintf("%s:%s", name, id))
	if err != nil && !h.db.IsErrNotFound(err) {
		return false, err
	}
	return s != nil, nil
}

func (h *HackerNews) fetchTopStoriesIds() ([]int, error) {
	resp, err := h.Get(topStoriesEndpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close().Error()

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
	resp, err := h.Get(fmt.Sprintf(storyEndpoint, id))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close().Error()

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