package quote

import (
	"context"
	"log"
	"sync"
	"time"
)

// Poller periodically refreshes quotes for a fixed set of symbols from a
// Provider and caches the latest successful result per symbol, so a slow or
// failing upstream request never blocks a caller asking for "the current
// quote" — it just gets the last known good one plus a way to check if that
// data is stale.
type Poller struct {
	provider Provider
	symbols  []Symbol
	interval time.Duration

	mu     sync.RWMutex
	latest map[Symbol]Quote
	errs   map[Symbol]error
}

func NewPoller(provider Provider, symbols []Symbol, interval time.Duration) *Poller {
	return &Poller{
		provider: provider,
		symbols:  symbols,
		interval: interval,
		latest:   make(map[Symbol]Quote),
		errs:     make(map[Symbol]error),
	}
}

// Run blocks, polling every interval until ctx is cancelled. It polls once
// immediately so callers have data right away instead of waiting a full
// interval for the first tick.
func (p *Poller) Run(ctx context.Context) {
	p.pollOnce()

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollOnce()
		}
	}
}

func (p *Poller) pollOnce() {
	for _, sym := range p.symbols {
		q, err := p.provider.GetQuote(sym)

		p.mu.Lock()
		if err != nil {
			p.errs[sym] = err
		} else {
			p.latest[sym] = q
			delete(p.errs, sym)
		}
		p.mu.Unlock()

		if err != nil {
			log.Printf("quote poll failed for %s: %v", sym, err)
		}
	}
}

// Latest returns the most recently fetched quote for symbol, and whether one
// has been successfully fetched at least once. A prior fetch failure does not
// clear it — the caller keeps trading on stale-but-known data rather than
// nothing, and can check LastError to decide if it's too stale to trust.
func (p *Poller) Latest(symbol Symbol) (Quote, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	q, ok := p.latest[symbol]
	return q, ok
}

// LastError returns the error from the most recent poll for symbol, if that
// poll failed. A subsequent successful poll clears it.
func (p *Poller) LastError(symbol Symbol) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.errs[symbol]
}
