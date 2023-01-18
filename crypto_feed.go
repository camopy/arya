package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"
)

const (
	cryptoFetchInterval        = 15 * time.Minute
	cryptoPriceChangeThreshold = 1.0
)

var coinEndpoint = "https://api.coingecko.com/api/v3/coins/%s?localization=false&community_data=false&developer_data=false"

var trackedCoins = []string{
	"bitcoin",
	"ethereum",
	"binancecoin",
	"avalanche-2",
	"matic-network",
	"aave",
	"chainlink",
	"solana",
	"uniswap",
	"cosmos",
	"cardano",
}

type CryptoFeed struct {
	client    *http.Client
	contentCh chan []string
}

func NewCryptoFeed(contentCh chan []string) *CryptoFeed {
	return &CryptoFeed{client: http.DefaultClient, contentCh: contentCh}
}

func (f *CryptoFeed) StartCryptoFeed() {
	for {
		var coins = make([]Coin, 0, len(trackedCoins))
		for _, coinId := range trackedCoins {
			coin, err := f.fetchCoin(coinId)
			if err != nil {
				log.Println(err)
			} else if math.Abs(coin.PriceChange1h) > cryptoPriceChangeThreshold {
				coins = append(coins, *coin)
			}
		}
		if len(coins) > 0 {
			f.contentCh <- []string{Coins(coins).String()}
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
	start := time.Now()
	resp, err := f.client.Get(fmt.Sprintf(coinEndpoint, coinId))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	trackExternalRequest(http.MethodGet, resp.Request.RequestURI, resp.StatusCode, time.Since(start))

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
