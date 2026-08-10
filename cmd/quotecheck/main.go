// quotecheck is the Phase 1 connectivity check: fetch TX/MTX from the
// configured quote.Provider and print the latest values.
package main

import (
	"flag"
	"fmt"
	"os"

	"tx-signal-engine/internal/quote"
	"tx-signal-engine/internal/quote/shioaji"
	"tx-signal-engine/internal/quote/taifex"
)

func main() {
	providerName := flag.String("provider", "taifex", `quote source: "taifex" or "shioaji"`)
	shioajiAdapterURL := flag.String("shioaji-adapter-url", "",
		"base URL of the running shioaji_adapter.py (defaults to http://127.0.0.1:8787)")
	flag.Parse()

	var provider quote.Provider
	switch *providerName {
	case "taifex":
		provider = taifex.NewProvider()
	case "shioaji":
		provider = shioaji.NewProvider(*shioajiAdapterURL)
	default:
		fmt.Fprintf(os.Stderr, "unknown -provider %q (want \"taifex\" or \"shioaji\")\n", *providerName)
		os.Exit(1)
	}

	for _, sym := range []quote.Symbol{quote.TX, quote.MTX} {
		q, err := provider.GetQuote(sym)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", sym, err)
			continue
		}
		fmt.Println(q)
	}
}
