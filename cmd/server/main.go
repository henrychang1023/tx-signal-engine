// server exposes signalcheck's behavior over HTTP: every request to
// /api/signal re-fetches TX/MTX from the provider and evaluates the given
// expression fresh, same as the CLI. No background polling/caching — once
// the data source is real-time, caching would just add staleness.
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	"tx-signal-engine/internal/engine"
	"tx-signal-engine/internal/quote"
	"tx-signal-engine/internal/quote/taifex"
)

//go:embed index.html
var indexHTML []byte

const defaultExpr = "TX.a1 > TX.b1 && TX.volume > 1000"

type quoteView struct {
	Date   string  `json:"date"`
	Price  float64 `json:"price"`
	Volume int64   `json:"volume"`
	Bid1   float64 `json:"b1"`
	Ask1   float64 `json:"a1"`
}

func toQuoteView(q quote.Quote) quoteView {
	return quoteView{
		Date:   q.Time.Format("2006-01-02"),
		Price:  q.Price,
		Volume: q.Volume,
		Bid1:   q.Bid1,
		Ask1:   q.Ask1,
	}
}

type signalResponse struct {
	Expr   string    `json:"expr"`
	TX     quoteView `json:"tx"`
	MTX    quoteView `json:"mtx"`
	Result bool      `json:"result"`
}

type server struct {
	provider quote.Provider
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func (s *server) handleSignal(w http.ResponseWriter, r *http.Request) {
	expression := r.URL.Query().Get("expr")
	if expression == "" {
		expression = defaultExpr
	}

	rule, err := engine.Compile(expression)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	tx, err := s.provider.GetQuote(quote.TX)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("TX: %w", err))
		return
	}
	mtx, err := s.provider.GetQuote(quote.MTX)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("MTX: %w", err))
		return
	}

	result, err := rule.Eval(engine.NewEnv(tx, mtx))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, signalResponse{
		Expr:   expression,
		TX:     toQuoteView(tx),
		MTX:    toQuoteView(mtx),
		Result: result,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

func newMux(s *server) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/signal", s.handleSignal)
	return mux
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	openBrowser := flag.Bool("open", true, "open the default browser automatically on startup")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}

	url := browserURL(ln.Addr().String())
	log.Printf("listening on %s", ln.Addr())

	if *openBrowser {
		if err := openInBrowser(url); err != nil {
			log.Printf("couldn't open browser automatically (%v) — open %s manually", err, url)
		}
	} else {
		log.Printf("open %s in your browser", url)
	}

	s := &server{provider: taifex.NewProvider()}
	log.Fatal(http.Serve(ln, newMux(s)))
}

// browserURL turns a listen address like "[::]:8080" or "127.0.0.1:8080"
// into a URL a browser on the same machine can actually reach.
func browserURL(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = addr
	}
	return "http://localhost:" + port + "/"
}

// openInBrowser launches the OS's default browser. Best-effort: a failure
// here just means the user opens the URL themselves, so callers should log
// and continue rather than treat it as fatal.
func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
