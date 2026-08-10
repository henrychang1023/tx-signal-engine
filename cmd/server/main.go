// server exposes signalcheck's behavior over HTTP: every request to
// /api/signal re-fetches TX/MTX from the active provider and evaluates the
// given expression fresh, same as the CLI. No background polling/caching —
// once the data source is real-time, caching would just add staleness.
//
// It also lets the web UI configure Shioaji at runtime: submitting API
// credentials to /api/shioaji/config saves them to config.json, launches
// adapter/shioaji_adapter.py as a managed subprocess (see
// internal/shioajiproc), and — once it reports healthy — switches the
// active provider from TAIFEX to Shioaji. This is additive to the existing
// -provider/-shioaji-adapter-url flags, which remain for the manual
// two-terminal workflow (running the adapter yourself, unmanaged by this
// process).
package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"tx-signal-engine/internal/engine"
	"tx-signal-engine/internal/quote"
	"tx-signal-engine/internal/quote/shioaji"
	"tx-signal-engine/internal/quote/taifex"
	"tx-signal-engine/internal/shioajiproc"
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
	mu       sync.RWMutex
	provider quote.Provider

	shioajiManager *shioajiproc.Manager
	configPath     string
	shioajiActive  bool
	shioajiErr     error
}

func (s *server) currentProvider() quote.Provider {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.provider
}

// connectShioaji starts (or restarts) the managed adapter subprocess with
// the given credentials and, on success, switches the active provider to
// it. Called both from the web UI's config submission and, at startup,
// from a saved config.json — same logic either way.
func (s *server) connectShioaji(apiKey, secretKey string) error {
	err := s.shioajiManager.Start(apiKey, secretKey)

	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.shioajiActive = false
		s.shioajiErr = err
		return err
	}
	s.provider = shioaji.NewProvider(s.shioajiManager.BaseURL())
	s.shioajiActive = true
	s.shioajiErr = nil
	return nil
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

	provider := s.currentProvider()

	tx, err := provider.GetQuote(quote.TX)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("TX: %w", err))
		return
	}
	mtx, err := provider.GetQuote(quote.MTX)
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

type shioajiConfigRequest struct {
	APIKey    string `json:"api_key"`
	SecretKey string `json:"secret_key"`
}

// handleShioajiConfig saves the submitted credentials and attempts to
// connect immediately, blocking (up to the adapter's health-check timeout)
// so the response tells the caller whether it actually worked.
func (s *server) handleShioajiConfig(w http.ResponseWriter, r *http.Request) {
	var req shioajiConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid request body: %w", err))
		return
	}
	req.APIKey = strings.TrimSpace(req.APIKey)
	req.SecretKey = strings.TrimSpace(req.SecretKey)
	if req.APIKey == "" || req.SecretKey == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("api_key and secret_key are required"))
		return
	}

	if err := shioajiproc.Save(s.configPath, shioajiproc.Config{APIKey: req.APIKey, SecretKey: req.SecretKey}); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("save config: %w", err))
		return
	}

	if err := s.connectShioaji(req.APIKey, req.SecretKey); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"connected": true})
}

type shioajiStatusResponse struct {
	Configured bool   `json:"configured"`
	Active     bool   `json:"active"`
	Error      string `json:"error,omitempty"`
}

// handleShioajiStatus never echoes back the stored credentials — only
// booleans and, on failure, the error text — so the web UI can render
// current state without the key/secret round-tripping back to the browser.
func (s *server) handleShioajiStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	active, connectErr := s.shioajiActive, s.shioajiErr
	s.mu.RUnlock()

	resp := shioajiStatusResponse{Active: active}
	if _, err := os.Stat(s.configPath); err == nil {
		resp.Configured = true
	}
	if connectErr != nil {
		resp.Error = connectErr.Error()
	}
	writeJSON(w, http.StatusOK, resp)
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
	mux.HandleFunc("POST /api/shioaji/config", s.handleShioajiConfig)
	mux.HandleFunc("GET /api/shioaji/status", s.handleShioajiStatus)
	return mux
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080",
		"listen address (defaults to localhost-only now that this server can accept Shioaji API secrets)")
	openBrowser := flag.Bool("open", true, "open the default browser automatically on startup")
	providerName := flag.String("provider", "taifex", `initial quote source: "taifex" or "shioaji"`)
	shioajiAdapterURL := flag.String("shioaji-adapter-url", "",
		"base URL of a manually-run shioaji_adapter.py (defaults to http://127.0.0.1:8787); only used with -provider=shioaji")
	shioajiPort := flag.Int("shioaji-port", 8787,
		"local port the Go-managed shioaji adapter subprocess listens on, used by the web UI's setup form")
	flag.Parse()

	var provider quote.Provider
	switch *providerName {
	case "taifex":
		provider = taifex.NewProvider()
	case "shioaji":
		provider = shioaji.NewProvider(*shioajiAdapterURL)
	default:
		log.Fatalf("unknown -provider %q (want \"taifex\" or \"shioaji\")", *providerName)
	}

	exeDir := "."
	if exePath, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exePath)
	}
	adapterDir := shioajiproc.LocateAdapterDir(exeDir)
	configPath := filepath.Join(filepath.Dir(adapterDir), "config.json")

	s := &server{
		provider:       provider,
		shioajiManager: shioajiproc.NewManager(adapterDir, *shioajiPort),
		configPath:     configPath,
	}

	if cfg, err := shioajiproc.Load(configPath); err == nil && cfg.APIKey != "" {
		go func() {
			if err := s.connectShioaji(cfg.APIKey, cfg.SecretKey); err != nil {
				log.Printf("auto-reconnect to shioaji using %s failed: %v", configPath, err)
			} else {
				log.Printf("auto-reconnected to shioaji using saved %s", configPath)
			}
		}()
	}

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
