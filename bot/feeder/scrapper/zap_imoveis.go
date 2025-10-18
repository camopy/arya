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

type ZapImoveis struct {
	logger *zaplog.Logger
	db     db.DB
}

func NewZapImoveis(logger *zaplog.Logger, db db.DB) *ZapImoveis {
	return &ZapImoveis{
		logger: logger,
		db:     db,
	}
}

type zapImovel struct {
	Image    string `json:"image"`
	Title    string `json:"title"`
	Price    string `json:"price"`
	Link     string `json:"link"`
	Location string `json:"location"`
}

func (z zapImovel) String() string {
	return fmt.Sprintf(`%s 
%s 
️%s
️%s

%s`, z.Title, z.Location, z.Price, z.Image, z.Link)
}

func (z *ZapImoveis) scrap(ctx context.Context, threadId int, url string) ([]commands.Content, error) {
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

	items := make([]zapImovel, 0, 10)
	var err error

	c.OnHTML("div.listings-wrapper li a", func(e *colly.HTMLElement) {
		if err != nil {
			return
		}

		locationSelection := e.DOM.Find("h2[data-cy='rp-cardProperty-location-txt']").Clone()
		locationSelection.Find("span").Remove()

		item := zapImovel{
			Title:    e.Attr("title"),
			Price:    e.ChildText("div div div div p.text-2-25.text-neutral-120.font-semibold"),
			Image:    e.ChildAttr("div div div div div div img", "src"),
			Link:     e.Attr("href"),
			Location: locationSelection.Text(),
		}

		if item.Price == "" {
			item.Price = e.ChildText("div div div div p.text-2-25.text-feedback-success-110.font-semibold")
		}

		if item.Link == "" || item.Title == "" {
			return
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
	})

	if err != nil {
		z.logger.Warn("Error visiting", zap.Error(err), zap.String("url", url))
		return nil, err
	}

	err = c.Visit(url)
	if err != nil {
		z.logger.Warn("Error visiting", zap.Error(err), zap.String("url", url))
	}

	return parseZapImoveis(threadId, items), nil
}

func parseZapImoveis(threadId int, items []zapImovel) []commands.Content {
	content := make([]commands.Content, 0, len(items))
	for _, item := range items {
		content = append(content, commands.Content{
			ThreadId: threadId,
			Text:     item.String(),
		})
	}
	return content
}

func (z *ZapImoveis) isNewItem(ctx context.Context, item zapImovel) (bool, error) {
	isDuplicate, err := z.isDuplicatePost(ctx, item.Link)
	if err != nil || isDuplicate {
		return false, err
	}
	return true, nil
}

func (z *ZapImoveis) isDuplicatePost(ctx context.Context, id string) (bool, error) {
	s, err := z.db.Get(ctx, fmt.Sprintf("%s:%s", "scrapper:items", id))
	if err != nil && !z.db.IsErrNotFound(err) {
		return false, err
	}
	return s != nil, nil
}

func (z *ZapImoveis) savePost(ctx context.Context, post zapImovel) error {
	value, err := json.Marshal(post)
	if err != nil {
		return err
	}
	return z.db.Set(ctx, fmt.Sprintf("%s:%s", "scrapper:items", post.Link), value, 30*24*time.Hour)
}
