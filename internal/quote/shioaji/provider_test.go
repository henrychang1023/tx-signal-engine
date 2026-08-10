package shioaji

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"tx-signal-engine/internal/quote"
)

func TestGetQuote_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("symbol"); got != "TX" {
			t.Errorf("symbol query = %q, want TX", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"symbol":"TX","ask1":44284,"bid1":44272,"price":44280,"volume":55986,"time":"2026-08-08T09:00:12+08:00"}`))
	}))
	defer server.Close()

	p := NewProvider(server.URL)

	got, err := p.GetQuote(quote.TX)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Price != 44280 || got.Ask1 != 44284 || got.Bid1 != 44272 || got.Volume != 55986 {
		t.Fatalf("GetQuote(TX) = %+v, unexpected values", got)
	}
	if got.Symbol != quote.TX {
		t.Fatalf("GetQuote(TX).Symbol = %v, want TX", got.Symbol)
	}
}

func TestGetQuote_NoDataYet(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"error":"no data yet for TX"}`))
	}))
	defer server.Close()

	p := NewProvider(server.URL)

	_, err := p.GetQuote(quote.TX)
	if err == nil {
		t.Fatal("expected an error when the adapter has no data yet")
	}
}

func TestGetQuote_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer server.Close()

	p := NewProvider(server.URL)

	if _, err := p.GetQuote(quote.TX); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}

func TestGetQuote_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not valid json`))
	}))
	defer server.Close()

	p := NewProvider(server.URL)

	if _, err := p.GetQuote(quote.TX); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestGetQuote_InvalidTime(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"symbol":"TX","ask1":1,"bid1":1,"price":1,"volume":1,"time":"not-a-time"}`))
	}))
	defer server.Close()

	p := NewProvider(server.URL)

	if _, err := p.GetQuote(quote.TX); err == nil {
		t.Fatal("expected an error for an unparseable time field")
	}
}

func TestNewProvider_DefaultsBaseURL(t *testing.T) {
	p := NewProvider("")
	if p.baseURL != defaultBaseURL {
		t.Fatalf("baseURL = %q, want %q", p.baseURL, defaultBaseURL)
	}
}
