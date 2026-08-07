// Package taifex implements quote.Provider against the Taiwan Futures Exchange's
// free, public, unauthenticated open-data API. It only publishes end-of-day
// reports (updated once per trading day, not tick-by-tick), so this provider is a
// stopgap: same QuoteProvider interface as later, lower-latency sources, but
// good enough to get the rest of the engine running before paying for real-time data.
package taifex

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tx-signal-engine/internal/quote"
)

const dailyMarketReportURL = "https://openapi.taifex.com.tw/v1/DailyMarketReportFut"

// row mirrors one entry of the DailyMarketReportFut response. Only the fields
// the engine needs are decoded; the rest of the payload is ignored.
type row struct {
	Date           string `json:"Date"`
	Contract       string `json:"Contract"`
	ContractMonth  string `json:"ContractMonth(Week)"`
	Last           string `json:"Last"`
	Volume         string `json:"Volume"`
	BestBid        string `json:"BestBid"`
	BestAsk        string `json:"BestAsk"`
	TradingSession string `json:"TradingSession"`
}

// Provider fetches TX/MTX quotes from TAIFEX's daily market report.
type Provider struct {
	httpClient *http.Client
	url        string // overridable in tests; defaults to dailyMarketReportURL
}

func NewProvider() *Provider {
	return &Provider{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		url:        dailyMarketReportURL,
	}
}

func (p *Provider) GetQuote(symbol quote.Symbol) (quote.Quote, error) {
	rows, err := p.fetchRows()
	if err != nil {
		return quote.Quote{}, err
	}

	match, err := frontMonthRow(rows, string(symbol))
	if err != nil {
		return quote.Quote{}, fmt.Errorf("taifex: %s: %w", symbol, err)
	}

	return rowToQuote(symbol, match)
}

func (p *Provider) fetchRows() ([]row, error) {
	resp, err := p.httpClient.Get(p.url)
	if err != nil {
		return nil, fmt.Errorf("taifex: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("taifex: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var rows []row
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("taifex: decode response: %w", err)
	}
	return rows, nil
}

// frontMonthRow picks the nearest-expiry contract month for the given product,
// preferring the regular ("一般") session over the after-hours ("盤後") one.
func frontMonthRow(rows []row, contract string) (row, error) {
	var candidates []row
	for _, r := range rows {
		if r.Contract == contract {
			candidates = append(candidates, r)
		}
	}
	if len(candidates) == 0 {
		return row{}, fmt.Errorf("no rows found for contract %q", contract)
	}

	frontMonth := candidates[0].ContractMonth
	for _, r := range candidates {
		if r.ContractMonth < frontMonth {
			frontMonth = r.ContractMonth
		}
	}

	var picked *row
	for i, r := range candidates {
		if r.ContractMonth != frontMonth {
			continue
		}
		if r.TradingSession == "一般" {
			picked = &candidates[i]
			break
		}
		if picked == nil {
			picked = &candidates[i]
		}
	}
	return *picked, nil
}

func rowToQuote(symbol quote.Symbol, r row) (quote.Quote, error) {
	date, err := time.Parse("20060102", r.Date)
	if err != nil {
		return quote.Quote{}, fmt.Errorf("parse date %q: %w", r.Date, err)
	}

	price, err := parseFloat(r.Last)
	if err != nil {
		return quote.Quote{}, fmt.Errorf("parse Last %q: %w", r.Last, err)
	}
	bid1, err := parseFloat(r.BestBid)
	if err != nil {
		return quote.Quote{}, fmt.Errorf("parse BestBid %q: %w", r.BestBid, err)
	}
	ask1, err := parseFloat(r.BestAsk)
	if err != nil {
		return quote.Quote{}, fmt.Errorf("parse BestAsk %q: %w", r.BestAsk, err)
	}
	volume, err := parseInt(r.Volume)
	if err != nil {
		return quote.Quote{}, fmt.Errorf("parse Volume %q: %w", r.Volume, err)
	}

	return quote.Quote{
		Symbol: symbol,
		Ask1:   ask1,
		Bid1:   bid1,
		Price:  price,
		Volume: volume,
		Time:   date,
	}, nil
}

// parseFloat treats TAIFEX's "-"/"NULL"/"" placeholders as zero rather than an error.
func parseFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" || s == "NULL" {
		return 0, nil
	}
	return strconv.ParseFloat(s, 64)
}

func parseInt(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" || s == "NULL" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}
