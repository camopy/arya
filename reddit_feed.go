package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/turnage/graw/reddit"
)

const (
	redditFetchInterval = 1 * time.Hour
	redditFetchLimit    = 10
)

var subreddits = []string{
	"/r/golang",
	"/r/investimentos",
	"/r/bitcoin",
	"/r/blockchaindeveloper",
	"/r/BogleheadsBrasil",
	"/r/coding",
	"/r/cruiserboarding",
	"/r/CryptoCurrency",
	"/r/CryptoTechnology",
	"/r/ethdev",
	"/r/ethereum",
	"/r/ExperiencedDevs",
	"/r/RepublicaDasCapivaras",
	"/r/SideProject",
	"/r/Sorare",
	"/r/startups",
}

type Reddit struct {
	client     reddit.Bot
	db         DB
	subreddits []string
	contentCh  chan []Content
	threadId   int
}

func NewReddit(contentCh chan []Content, db DB, id string, key string, username string, password string, threadId int) *Reddit {
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
		client:     bot,
		subreddits: subreddits,
		db:         db,
		contentCh:  contentCh,
		threadId:   threadId,
	}
}

func (u *Reddit) StartReddit() {
	for {
		contents := make([]Content, 0, redditFetchLimit*len(u.subreddits))
		for _, subreddit := range u.subreddits {
			posts, err := u.fetch(subreddit)
			if err != nil {
				fmt.Println(err)
			} else if len(posts) > 0 {
				contents = append(contents, posts...)
			}
		}
		if len(contents) > 0 {
			u.contentCh <- contents
		}
		time.Sleep(redditFetchInterval)
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
	return fmt.Sprintf(`/r/%s 
%s - ⬆️%d
%s

https://www.reddit.com%s
	`, p.Subreddit, p.Title, p.Score, p.URL, p.Permalink)
}

func (u *Reddit) fetch(subreddit string) ([]Content, error) {
	harvest, err := u.client.ListingWithParams(subreddit, map[string]string{
		"limit": strconv.Itoa(redditFetchLimit),
	})
	if err != nil {
		return nil, err
	}
	posts := make([]Content, 0, redditFetchLimit*len(u.subreddits))
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

		isNewPost, err := u.isNewPost(context.Background(), p)
		if err != nil {
			return nil, err
		}
		if !isNewPost {
			continue
		}

		if err := u.savePost(context.Background(), p); err != nil {
			return nil, err
		}

		posts = append(posts, Content{
			threadId: u.threadId,
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
	s, err := u.db.Get(ctx, fmt.Sprintf("%s:%s", "reddit", id))
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
	return u.db.Put(ctx, fmt.Sprintf("%s:%s", "reddit", post.ID), value)
}
