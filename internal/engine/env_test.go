package engine

import (
	"testing"

	"tx-signal-engine/internal/quote"
)

func TestNewEnv(t *testing.T) {
	tx := quote.Quote{Symbol: quote.TX, Ask1: 44284, Bid1: 44272, Price: 44280, Volume: 55986}
	mtx := quote.Quote{Symbol: quote.MTX, Ask1: 44275, Bid1: 44271, Price: 44274, Volume: 129055}

	env := NewEnv(tx, mtx)

	if env.TX.A1 != 44284 || env.TX.B1 != 44272 || env.TX.Price != 44280 || env.TX.Volume != 55986 {
		t.Fatalf("TX not mapped correctly: %+v", env.TX)
	}
	if env.MTX.A1 != 44275 || env.MTX.B1 != 44271 || env.MTX.Price != 44274 || env.MTX.Volume != 129055 {
		t.Fatalf("MTX not mapped correctly: %+v", env.MTX)
	}
	if env.Now.IsZero() {
		t.Fatal("expected Now to be set")
	}
}
