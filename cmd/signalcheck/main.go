// signalcheck fetches a fresh TX/MTX quote, prints every value the
// expression can see, then evaluates a boolean expression against them and
// prints true/false. Every invocation re-fetches — there's no background
// polling loop; run it again whenever you want an updated reading.
package main

import (
	"flag"
	"fmt"
	"os"

	"tx-signal-engine/internal/engine"
	"tx-signal-engine/internal/quote"
	"tx-signal-engine/internal/quote/shioaji"
	"tx-signal-engine/internal/quote/taifex"
)

func main() {
	expression := flag.String("expr", "TX.a1 > TX.b1 && TX.volume > 1000",
		"boolean expression to evaluate, e.g. \"TX.a1 > TX.b1 && TX.volume > 1000\"")
	providerName := flag.String("provider", "taifex", `quote source: "taifex" or "shioaji"`)
	shioajiAdapterURL := flag.String("shioaji-adapter-url", "",
		"base URL of the running shioaji_adapter.py (defaults to http://127.0.0.1:8787)")
	flag.Parse()

	rule, err := engine.Compile(*expression)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

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

	tx, err := provider.GetQuote(quote.TX)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TX: %v\n", err)
		os.Exit(1)
	}
	mtx, err := provider.GetQuote(quote.MTX)
	if err != nil {
		fmt.Fprintf(os.Stderr, "MTX: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(tx)
	fmt.Println(mtx)

	result, err := rule.Eval(engine.NewEnv(tx, mtx))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	fmt.Printf("expr: %s\n", *expression)
	fmt.Printf("signal = %v\n", result)
}
