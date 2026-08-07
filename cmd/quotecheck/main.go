// quotecheck is the Phase 1 connectivity check: fetch TX/MTX from the
// configured quote.Provider and print the latest values.
package main

import (
	"fmt"
	"os"

	"tx-signal-engine/internal/quote"
	"tx-signal-engine/internal/quote/taifex"
)

func main() {
	provider := taifex.NewProvider()

	for _, sym := range []quote.Symbol{quote.TX, quote.MTX} {
		q, err := provider.GetQuote(sym)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", sym, err)
			continue
		}
		fmt.Println(q)
	}
}
