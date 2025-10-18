package scrapper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gocolly/colly/v2"
	"github.com/imroc/req/v3"
	"go.uber.org/zap"

	"github.com/camopy/rss_everything/bot/commands"
	"github.com/camopy/rss_everything/db"
	"github.com/camopy/rss_everything/zaplog"
)

type Olx struct {
	logger *zaplog.Logger
	db     db.DB
}

func NewOlx(logger *zaplog.Logger, db db.DB) *Olx {
	return &Olx{
		logger: logger,
		db:     db,
	}
}

type olx struct {
	Image    string `json:"image"`
	Title    string `json:"title"`
	Price    string `json:"price"`
	Link     string `json:"link"`
	Location string `json:"location"`
}

func (z olx) String() string {
	return fmt.Sprintf(`%s 
%s 
️%s

%s`, z.Title, z.Location, z.Price, z.Link)
}

func (z *Olx) scrap(ctx context.Context, threadId int, url string) ([]commands.Content, error) {
	fakeChrome := req.DefaultClient().ImpersonateChrome()

	c := colly.NewCollector(func(collector *colly.Collector) {
		collector.Context = ctx
		collector.UserAgent = fakeChrome.Headers.Get("user-agent")
	})
	c.SetClient(&http.Client{
		Transport: fakeChrome.Transport,
	})

	c.OnRequest(func(r *colly.Request) {
		z.logger.Info("Visiting", zap.String("url", r.URL.String()))
	})

	c.OnError(func(r *colly.Response, err error) {
		z.logger.Error("Something went wrong", zap.Error(err), zap.String("url", r.Request.URL.String()), zap.Any("response", r))
	})

	items := make([]olx, 0, 10)
	var err error

	c.OnHTML("section", func(e *colly.HTMLElement) {
		if err == nil {
			item := olx{
				Image:    e.ChildAttr("div.AdCard_media__0T37N div picture source", "srcset"),
				Title:    e.ChildAttr("div div a", "title"),
				Price:    e.ChildText("div.olx-adcard__mediumbody h3"),
				Location: e.ChildText("div.olx-adcard__bottombody div p.typo-caption.olx-adcard__location"),
				Link:     e.ChildAttr("div div a", "href"),
			}

			isNewPost, duplicateErr := z.isNewItem(ctx, item)
			if duplicateErr != nil {
				err = duplicateErr
			}
			if isNewPost {
				if saveErr := z.savePost(ctx, item); saveErr != nil {
					err = saveErr
				}
				z.logger.Info("saved new item", zap.String("item", item.String()))
				items = append(items, item)
			}
		}
	})

	if err != nil {
		z.logger.Warn("Error visiting", zap.Error(err), zap.String("url", url))
		return nil, err
	}

	err = c.Visit(url)
	if err != nil {
		z.logger.Warn("Error visiting", zap.Error(err), zap.String("url", url))
	}

	return parseOlx(threadId, items), nil
}

func parseOlx(threadId int, items []olx) []commands.Content {
	content := make([]commands.Content, 0, len(items))
	for _, item := range items {
		content = append(content, commands.Content{
			ThreadId: threadId,
			Text:     item.String(),
		})
	}
	return content
}

func (z *Olx) isNewItem(ctx context.Context, post olx) (bool, error) {
	isDuplicate, err := z.isDuplicatePost(ctx, post.Link)
	if err != nil || isDuplicate {
		return false, err
	}
	return true, nil
}

func (z *Olx) isDuplicatePost(ctx context.Context, id string) (bool, error) {
	s, err := z.db.Get(ctx, fmt.Sprintf("%s:%s", "scrapper:items", id))
	if err != nil && !z.db.IsErrNotFound(err) {
		return false, err
	}
	return s != nil, nil
}

func (z *Olx) savePost(ctx context.Context, post olx) error {
	value, err := json.Marshal(post)
	if err != nil {
		return err
	}
	return z.db.Set(ctx, fmt.Sprintf("%s:%s", "scrapper:items", post.Link), value, 30*24*time.Hour)
}
