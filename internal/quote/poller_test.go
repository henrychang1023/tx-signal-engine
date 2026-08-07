package quote_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"tx-signal-engine/internal/quote"
)

// fakeProvider lets tests script GetQuote's return value per symbol and swap
// that script between calls, to simulate an upstream flipping from healthy to failing.
type fakeProvider struct {
	quotes map[quote.Symbol]quote.Quote
	errs   map[quote.Symbol]error
	calls  int
}

func (f *fakeProvider) GetQuote(symbol quote.Symbol) (quote.Quote, error) {
	f.calls++
	if err, ok := f.errs[symbol]; ok {
		return quote.Quote{}, err
	}
	return f.quotes[symbol], nil
}

// runOnce drives exactly one poll cycle: Poller.Run always polls once before
// checking ctx.Done(), so a pre-cancelled context makes Run perform a single
// synchronous pollOnce() and return, with no ticker/goroutine timing involved.
func runOnce(p *quote.Poller) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.Run(ctx)
}

func TestPoller_NoDataBeforeFirstPoll(t *testing.T) {
	p := quote.NewPoller(&fakeProvider{}, []quote.Symbol{quote.TX}, time.Hour)

	if _, ok := p.Latest(quote.TX); ok {
		t.Fatal("expected no data before any poll")
	}
	if err := p.LastError(quote.TX); err != nil {
		t.Fatalf("expected no error before any poll, got %v", err)
	}
}

func TestPoller_PopulatesLatestOnSuccess(t *testing.T) {
	want := quote.Quote{Symbol: quote.TX, Ask1: 44284, Bid1: 44272, Price: 44280, Volume: 55986}
	fp := &fakeProvider{quotes: map[quote.Symbol]quote.Quote{quote.TX: want}}
	p := quote.NewPoller(fp, []quote.Symbol{quote.TX}, time.Hour)

	runOnce(p)

	got, ok := p.Latest(quote.TX)
	if !ok {
		t.Fatal("expected data after a successful poll")
	}
	if got != want {
		t.Fatalf("Latest = %+v, want %+v", got, want)
	}
	if err := p.LastError(quote.TX); err != nil {
		t.Fatalf("expected no error after a successful poll, got %v", err)
	}
}

func TestPoller_KeepsStaleDataWhenPollFails(t *testing.T) {
	good := quote.Quote{Symbol: quote.TX, Price: 44280}
	fp := &fakeProvider{quotes: map[quote.Symbol]quote.Quote{quote.TX: good}}
	p := quote.NewPoller(fp, []quote.Symbol{quote.TX}, time.Hour)

	runOnce(p) // succeeds, caches `good`

	fp.errs = map[quote.Symbol]error{quote.TX: errors.New("upstream 500")}
	runOnce(p) // fails

	got, ok := p.Latest(quote.TX)
	if !ok || got != good {
		t.Fatalf("Latest = %+v, %v, want stale %+v, true", got, ok, good)
	}
	if err := p.LastError(quote.TX); err == nil {
		t.Fatal("expected LastError to report the failed poll")
	}
}

func TestPoller_ErrorClearsOnNextSuccess(t *testing.T) {
	fp := &fakeProvider{errs: map[quote.Symbol]error{quote.TX: errors.New("boom")}}
	p := quote.NewPoller(fp, []quote.Symbol{quote.TX}, time.Hour)

	runOnce(p)
	if err := p.LastError(quote.TX); err == nil {
		t.Fatal("expected an error after a failing poll")
	}

	fp.errs = nil
	fp.quotes = map[quote.Symbol]quote.Quote{quote.TX: {Symbol: quote.TX, Price: 100}}
	runOnce(p)

	if err := p.LastError(quote.TX); err != nil {
		t.Fatalf("expected LastError cleared after a successful poll, got %v", err)
	}
	if _, ok := p.Latest(quote.TX); !ok {
		t.Fatal("expected data after the recovering poll")
	}
}

func TestPoller_SymbolsAreIndependent(t *testing.T) {
	fp := &fakeProvider{
		quotes: map[quote.Symbol]quote.Quote{quote.TX: {Symbol: quote.TX, Price: 44280}},
		errs:   map[quote.Symbol]error{quote.MTX: errors.New("MTX feed down")},
	}
	p := quote.NewPoller(fp, []quote.Symbol{quote.TX, quote.MTX}, time.Hour)

	runOnce(p)

	if _, ok := p.Latest(quote.TX); !ok {
		t.Fatal("expected TX to have data")
	}
	if _, ok := p.Latest(quote.MTX); ok {
		t.Fatal("expected MTX to have no data, its poll failed")
	}
	if err := p.LastError(quote.MTX); err == nil {
		t.Fatal("expected MTX LastError to be set")
	}
	if err := p.LastError(quote.TX); err != nil {
		t.Fatalf("expected TX LastError to be nil, got %v", err)
	}
}
