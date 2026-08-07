// pollcheck is the Phase 2 check: run the polling loop in the background and
// print each symbol's latest cached quote (or last error) on a fixed cadence,
// to confirm the poller keeps serving data across repeated ticks instead of
// just proving a single request works.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tx-signal-engine/internal/quote"
	"tx-signal-engine/internal/quote/taifex"
)

func main() {
	pollInterval := flag.Duration("interval", 5*time.Minute, "how often to poll the upstream provider")
	printInterval := flag.Duration("print-interval", 10*time.Second, "how often to print the cached state")
	flag.Parse()

	provider := taifex.NewProvider()
	symbols := []quote.Symbol{quote.TX, quote.MTX}
	poller := quote.NewPoller(provider, symbols, *pollInterval)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go poller.Run(ctx)

	fmt.Printf("polling every %s, printing every %s (Ctrl+C to stop)\n", *pollInterval, *printInterval)

	ticker := time.NewTicker(*printInterval)
	defer ticker.Stop()

	printLatest(poller, symbols)
	for {
		select {
		case <-ctx.Done():
			fmt.Println("shutting down")
			return
		case <-ticker.C:
			printLatest(poller, symbols)
		}
	}
}

func printLatest(poller *quote.Poller, symbols []quote.Symbol) {
	now := time.Now().Format("15:04:05")
	for _, sym := range symbols {
		if err := poller.LastError(sym); err != nil {
			fmt.Printf("[%s] %-4s ERROR: %v\n", now, sym, err)
			continue
		}
		q, ok := poller.Latest(sym)
		if !ok {
			fmt.Printf("[%s] %-4s no data yet\n", now, sym)
			continue
		}
		fmt.Printf("[%s] %-4s date=%s price=%.0f volume=%d b1=%.0f a1=%.0f\n",
			now, sym, q.Time.Format("2006-01-02"), q.Price, q.Volume, q.Bid1, q.Ask1)
	}
}
