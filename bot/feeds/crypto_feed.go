package feeds

import (
	"encoding/json"
	"fmt"
	"go.uber.org/zap"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/camopy/rss_everything/bot/commands"
	"github.com/camopy/rss_everything/metrics"
	"github.com/camopy/rss_everything/zaplog"
)

const (
	cryptoFetchInterval = 15 * time.Minute
)

var coinEndpoint = "https://api.coingecko.com/api/v3/coins/%s?localization=false&community_data=false&developer_data=false"

var trackedCoins = map[string]float64{
	"bitcoin":       1.0,
	"ethereum":      1.0,
	"binancecoin":   3.0,
	"avalanche-2":   3.0,
	"matic-network": 3.0,
	"aave":          3.0,
	"chainlink":     3.0,
	"solana":        3.0,
	"uniswap":       3.0,
	"cosmos":        3.0,
	"cardano":       3.0,
}

type CryptoFeed struct {
	client    *http.Client
	logger    *zaplog.Logger
	contentCh chan []commands.Content
	threadId  int
}

func NewCryptoFeed(logger *zaplog.Logger, contentCh chan []commands.Content, threadId int) *CryptoFeed {
	return &CryptoFeed{client: http.DefaultClient, logger: logger, contentCh: contentCh, threadId: threadId}
}

func (f *CryptoFeed) StartCryptoFeed() {
	f.logger.Info("starting crypto feed")
	for {
		var coins = make([]Coin, 0, len(trackedCoins))
		for coinId, threshold := range trackedCoins {
			coin, err := f.fetchCoin(coinId)
			if err != nil {
				f.logger.Error("error fetching coin", zap.Error(err), zap.String("coinId", coinId))
			} else if math.Abs(coin.PriceChange1h) > threshold {
				coins = append(coins, *coin)
			}
		}
		if len(coins) > 0 {
			f.logger.Info("sending coins", zap.Int("count", len(coins)), zap.Int("threadId", f.threadId))
			f.contentCh <- []commands.Content{
				{
					Text:     Coins(coins).String(),
					ThreadId: f.threadId,
				},
			}
		}
		time.Sleep(cryptoFetchInterval)
	}
}

type Coins []Coin

func (cs Coins) String() string {
	var sb strings.Builder
	for _, c := range cs {
		sb.WriteString(c.String())
		sb.WriteString("\n\n")
	}
	return sb.String()
}

type Coin struct {
	Id            string
	Symbol        string
	Name          string
	Price         float64
	PriceChange1h float64
}

func (c *Coin) String() string {
	if c.PriceChange1h > 0 {
		return fmt.Sprintf("🟢 %s (%s) is up %.2f%% in the last hour. Currently at %.2f USD", c.Name, strings.ToUpper(c.Symbol), c.PriceChange1h, c.Price)
	}
	return fmt.Sprintf("🔴 %s (%s) is down %.2f%% in the last hour. Currently at %.2f USD", c.Name, strings.ToUpper(c.Symbol), c.PriceChange1h, c.Price)
}

func (f *CryptoFeed) fetchCoin(coinId string) (*Coin, error) {
	f.logger.Info("fetching coin", zap.String("coinId", coinId))
	start := time.Now()
	resp, err := f.client.Get(fmt.Sprintf(coinEndpoint, coinId))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	metrics.TrackExternalRequest(http.MethodGet, resp.Request.URL.Host, resp.StatusCode, time.Since(start))

	type coinResp struct {
		Id         string `json:"id"`
		Symbol     string `json:"symbol"`
		Name       string `json:"name"`
		MarketData struct {
			CurrentPrice struct {
				Usd float64 `json:"usd"`
				Brl float64 `json:"brl"`
			} `json:"current_price"`
			PriceChangePercentage1hInCurrency struct {
				Usd float64 `json:"usd"`
				Brl float64 `json:"brl"`
			} `json:"price_change_percentage_1h_in_currency"`
		} `json:"market_data"`
	}

	var coin coinResp
	if err := json.NewDecoder(resp.Body).Decode(&coin); err != nil {
		return nil, err
	}

	return &Coin{
		Id:            coin.Id,
		Symbol:        coin.Symbol,
		Name:          coin.Name,
		Price:         coin.MarketData.CurrentPrice.Usd,
		PriceChange1h: coin.MarketData.PriceChangePercentage1hInCurrency.Usd,
	}, nil
}
