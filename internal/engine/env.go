// Package engine parses and evaluates user-supplied boolean expressions
// (e.g. "TX.a1 > TX.b1 && TX.volume > 1000") against the latest quotes from
// the data layer.
package engine

import (
	"time"

	"tx-signal-engine/internal/quote"
)

// Quote is the per-symbol view exposed to expressions. Field names are the
// variables listed in the project plan: a1（委賣一價）, b1（委買一價）, 最新成交價, 成交量.
type Quote struct {
	A1     float64   `expr:"a1"`
	B1     float64   `expr:"b1"`
	Price  float64   `expr:"price"`
	Volume int64     `expr:"volume"`
	Time   time.Time `expr:"time"`
}

// Env is the variable scope expressions are evaluated against.
type Env struct {
	TX  Quote     `expr:"TX"`
	MTX Quote     `expr:"MTX"`
	Now time.Time `expr:"now"`
}

func toEngineQuote(q quote.Quote) Quote {
	return Quote{
		A1:     q.Ask1,
		B1:     q.Bid1,
		Price:  q.Price,
		Volume: q.Volume,
		Time:   q.Time,
	}
}

// NewEnv builds the expression scope from already-fetched TX/MTX quotes.
func NewEnv(tx, mtx quote.Quote) Env {
	return Env{
		TX:  toEngineQuote(tx),
		MTX: toEngineQuote(mtx),
		Now: time.Now(),
	}
}
