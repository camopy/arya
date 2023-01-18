package trss

import (
	"github.com/camopy/rss_everything/database"
	"github.com/camopy/rss_everything/networks"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const (
	startCommand   = "start"
	chatGPTCommand = "chatgpt"
)

type Config struct {
	TelegramApiKey string
	ChatGPTApiKey  string
}

type Bot struct {
	cfg          Config
	client       *tgbotapi.BotAPI
	db           database.DB
	chatId       int64
	contentsChan chan []string
	chatGPT      *networks.ChatGPT
	hackerNews   *networks.HackerNews
}

func NewBot(db database.DB, cfg Config) *Bot {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramApiKey)
	if err != nil {
		log.Panic(err)
	}
	api.Debug = true
	log.Printf("Authorized on account %s", api.Self.UserName)
	return &Bot{
		cfg:          cfg,
		client:       api,
		db:           db,
		contentsChan: make(chan []string),
	}
}

func (b *Bot) Start() {
	b.initFeeds(b.cfg)
	go b.handleContentUpdates()
	b.handleMessages()
}

func (b *Bot) initFeeds(cfg Config) {
	b.chatGPT = networks.NewChatGPT(b.contentsChan, cfg.ChatGPTApiKey)
	b.hackerNews = networks.NewHackerNews(b.contentsChan, b.db)
}

func (b *Bot) handleContentUpdates() {
	for {
		select {
		case contents := <-b.contentsChan:
			for _, c := range contents {
				msg := tgbotapi.NewMessage(b.chatId, c)
				_, err := b.client.Send(msg)
				if err != nil {
					log.Println(err)
				}
			}
		}
	}
}

func (b *Bot) handleMessages() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates := b.client.GetUpdatesChan(u)
	for update := range updates {
		if update.Message != nil && update.Message.IsCommand() {
			b.handleCommand(update.Message)
		}
	}
}

func (b *Bot) handleCommand(msg *tgbotapi.Message) {
	switch msg.Command() {
	case startCommand:
		b.chatId = msg.Chat.ID
		go b.hackerNews.StartHackerNews()
		go b.chatGPT.StartChatGPT()
	case chatGPTCommand:
		go b.chatGPT.Ask(msg.Text)
	}
}
