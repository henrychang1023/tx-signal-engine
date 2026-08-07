package taifex

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"tx-signal-engine/internal/quote"
)

func TestFrontMonthRow_PicksNearestMonthRegularSession(t *testing.T) {
	rows := []row{
		{Contract: "TX", ContractMonth: "202609", TradingSession: "一般", Last: "44300"},
		{Contract: "TX", ContractMonth: "202608", TradingSession: "盤後", Last: "44278"},
		{Contract: "TX", ContractMonth: "202608", TradingSession: "一般", Last: "44280"},
		{Contract: "MTX", ContractMonth: "202608", TradingSession: "一般", Last: "99999"},
	}

	got, err := frontMonthRow(rows, "TX")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ContractMonth != "202608" || got.TradingSession != "一般" || got.Last != "44280" {
		t.Fatalf("frontMonthRow = %+v, want front month 202608/一般/44280", got)
	}
}

func TestFrontMonthRow_FallsBackToAfterHoursSession(t *testing.T) {
	rows := []row{
		{Contract: "TX", ContractMonth: "202608", TradingSession: "盤後", Last: "44278"},
	}

	got, err := frontMonthRow(rows, "TX")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.TradingSession != "盤後" {
		t.Fatalf("expected fallback to 盤後, got %+v", got)
	}
}

func TestFrontMonthRow_NoMatchingContract(t *testing.T) {
	rows := []row{{Contract: "MTX", ContractMonth: "202608"}}

	if _, err := frontMonthRow(rows, "TX"); err == nil {
		t.Fatal("expected an error when no row matches the contract")
	}
}

func TestParseFloat(t *testing.T) {
	cases := []struct {
		in      string
		want    float64
		wantErr bool
	}{
		{"44280", 44280, false},
		{"44280.5", 44280.5, false},
		{"-", 0, false},
		{"NULL", 0, false},
		{"", 0, false},
		{"not-a-number", 0, true},
	}
	for _, c := range cases {
		got, err := parseFloat(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("parseFloat(%q) error = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("parseFloat(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseInt(t *testing.T) {
	cases := []struct {
		in      string
		want    int64
		wantErr bool
	}{
		{"55986", 55986, false},
		{"-", 0, false},
		{"NULL", 0, false},
		{"", 0, false},
		{"not-a-number", 0, true},
	}
	for _, c := range cases {
		got, err := parseInt(c.in)
		if (err != nil) != c.wantErr {
			t.Errorf("parseInt(%q) error = %v, wantErr %v", c.in, err, c.wantErr)
			continue
		}
		if !c.wantErr && got != c.want {
			t.Errorf("parseInt(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestRowToQuote_Valid(t *testing.T) {
	r := row{Date: "20260806", Last: "44280", BestBid: "44272", BestAsk: "44284", Volume: "55986"}

	got, err := rowToQuote(quote.TX, r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := quote.Quote{
		Symbol: quote.TX,
		Ask1:   44284,
		Bid1:   44272,
		Price:  44280,
		Volume: 55986,
		Time:   time.Date(2026, 8, 6, 0, 0, 0, 0, time.UTC),
	}
	if got != want {
		t.Fatalf("rowToQuote = %+v, want %+v", got, want)
	}
}

func TestRowToQuote_InvalidDate(t *testing.T) {
	r := row{Date: "not-a-date", Last: "44280", BestBid: "44272", BestAsk: "44284", Volume: "1"}

	if _, err := rowToQuote(quote.TX, r); err == nil {
		t.Fatal("expected an error for an invalid date")
	}
}

func TestGetQuote_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[
			{"Date":"20260806","Contract":"TX","ContractMonth(Week)":"202608","Last":"44280","Volume":"55986","BestBid":"44272","BestAsk":"44284","TradingSession":"一般"},
			{"Date":"20260806","Contract":"MTX","ContractMonth(Week)":"202608","Last":"44274","Volume":"129055","BestBid":"44271","BestAsk":"44275","TradingSession":"一般"}
		]`))
	}))
	defer server.Close()

	p := &Provider{httpClient: server.Client(), url: server.URL}

	got, err := p.GetQuote(quote.TX)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Price != 44280 || got.Ask1 != 44284 || got.Bid1 != 44272 || got.Volume != 55986 {
		t.Fatalf("GetQuote(TX) = %+v, unexpected values", got)
	}
}

func TestGetQuote_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("upstream error"))
	}))
	defer server.Close()

	p := &Provider{httpClient: server.Client(), url: server.URL}

	if _, err := p.GetQuote(quote.TX); err == nil {
		t.Fatal("expected an error for a non-200 response")
	}
}

func TestGetQuote_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not valid json`))
	}))
	defer server.Close()

	p := &Provider{httpClient: server.Client(), url: server.URL}

	if _, err := p.GetQuote(quote.TX); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestGetQuote_ContractNotPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"Date":"20260806","Contract":"MTX","ContractMonth(Week)":"202608","Last":"1","Volume":"1","BestBid":"1","BestAsk":"1","TradingSession":"一般"}]`))
	}))
	defer server.Close()

	p := &Provider{httpClient: server.Client(), url: server.URL}

	if _, err := p.GetQuote(quote.TX); err == nil {
		t.Fatal("expected an error when the requested contract isn't in the response")
	}
}
