// Package quote defines the data-layer contract for pulling TX/MTX quotes,
// independent of which upstream source (TAIFEX, FinMind, a broker API, ...) backs it.
package quote

import (
	"fmt"
	"time"
)

// Symbol identifies a futures product.
type Symbol string

const (
	TX  Symbol = "TX"  // 大台
	MTX Symbol = "MTX" // 小台
)

// Quote holds the fields the signal engine's expressions can reference.
type Quote struct {
	Symbol Symbol
	Ask1   float64 // a1 委賣一價
	Bid1   float64 // b1 委買一價
	Price  float64 // 最新成交價
	Volume int64   // 成交量
	Time   time.Time
}

// String renders a Quote's fields for CLI output.
func (q Quote) String() string {
	return fmt.Sprintf("%-4s date=%s price=%.0f volume=%d b1(bid1)=%.0f a1(ask1)=%.0f",
		q.Symbol, q.Time.Format("2006-01-02"), q.Price, q.Volume, q.Bid1, q.Ask1)
}

// Provider fetches the latest Quote for a symbol. Implementations may source
// this from a REST snapshot, a broker's tick stream, or (initially) an EOD report.
type Provider interface {
	GetQuote(symbol Symbol) (Quote, error)
}
