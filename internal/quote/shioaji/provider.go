// Package shioaji implements quote.Provider against a local HTTP adapter
// (adapter/shioaji_adapter.py) that bridges the Python-only Shioaji SDK:
// the adapter logs into Shioaji, subscribes to TX/MTX tick + best-bid/ask
// data, and exposes the latest snapshot per symbol as JSON. This provider
// just polls that local endpoint, the same way taifex.Provider polls
// TAIFEX's REST API — only the URL and payload shape differ.
package shioaji

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"tx-signal-engine/internal/quote"
)

const defaultBaseURL = "http://127.0.0.1:8787"

// quoteResponse mirrors the JSON the adapter's GET /quote endpoint returns.
type quoteResponse struct {
	Symbol string  `json:"symbol"`
	Ask1   float64 `json:"ask1"`
	Bid1   float64 `json:"bid1"`
	Price  float64 `json:"price"`
	Volume int64   `json:"volume"`
	Time   string  `json:"time"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Provider fetches TX/MTX quotes from the local Shioaji adapter process.
type Provider struct {
	httpClient *http.Client
	baseURL    string
}

// NewProvider builds a Provider against the adapter at baseURL. An empty
// baseURL defaults to http://127.0.0.1:8787, the adapter's default listen
// address.
func NewProvider(baseURL string) *Provider {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Provider{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    baseURL,
	}
}

func (p *Provider) GetQuote(symbol quote.Symbol) (quote.Quote, error) {
	reqURL := fmt.Sprintf("%s/quote?symbol=%s", p.baseURL, url.QueryEscape(string(symbol)))

	resp, err := p.httpClient.Get(reqURL)
	if err != nil {
		return quote.Quote{}, fmt.Errorf("shioaji: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return quote.Quote{}, fmt.Errorf("shioaji: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp errorResponse
		if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
			return quote.Quote{}, fmt.Errorf("shioaji: %s: %s", symbol, errResp.Error)
		}
		return quote.Quote{}, fmt.Errorf("shioaji: unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var qr quoteResponse
	if err := json.Unmarshal(body, &qr); err != nil {
		return quote.Quote{}, fmt.Errorf("shioaji: decode response: %w", err)
	}

	t, err := time.Parse(time.RFC3339, qr.Time)
	if err != nil {
		return quote.Quote{}, fmt.Errorf("shioaji: parse time %q: %w", qr.Time, err)
	}

	return quote.Quote{
		Symbol: symbol,
		Ask1:   qr.Ask1,
		Bid1:   qr.Bid1,
		Price:  qr.Price,
		Volume: qr.Volume,
		Time:   t,
	}, nil
}
