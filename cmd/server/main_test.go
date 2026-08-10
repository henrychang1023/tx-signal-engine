package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"tx-signal-engine/internal/quote"
	"tx-signal-engine/internal/shioajiproc"
)

type stubProvider struct {
	quotes map[quote.Symbol]quote.Quote
	errs   map[quote.Symbol]error
}

func (p *stubProvider) GetQuote(symbol quote.Symbol) (quote.Quote, error) {
	if err, ok := p.errs[symbol]; ok {
		return quote.Quote{}, err
	}
	return p.quotes[symbol], nil
}

func healthyProvider() *stubProvider {
	return &stubProvider{quotes: map[quote.Symbol]quote.Quote{
		quote.TX:  {Symbol: quote.TX, Ask1: 44284, Bid1: 44272, Price: 44280, Volume: 55986},
		quote.MTX: {Symbol: quote.MTX, Ask1: 44275, Bid1: 44271, Price: 44274, Volume: 129055},
	}}
}

func TestHandleSignal_Success(t *testing.T) {
	s := &server{provider: healthyProvider()}
	req := httptest.NewRequest(http.MethodGet, "/api/signal?expr="+`TX.a1+%3E+TX.b1`, nil)
	rec := httptest.NewRecorder()

	newMux(s).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp signalResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Result {
		t.Errorf("Result = false, want true")
	}
	if resp.TX.Ask1 != 44284 || resp.MTX.Price != 44274 {
		t.Errorf("quotes not mapped correctly: %+v", resp)
	}
}

func TestHandleSignal_DefaultExpr(t *testing.T) {
	s := &server{provider: healthyProvider()}
	req := httptest.NewRequest(http.MethodGet, "/api/signal", nil)
	rec := httptest.NewRecorder()

	newMux(s).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp signalResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Expr != defaultExpr {
		t.Errorf("Expr = %q, want default %q", resp.Expr, defaultExpr)
	}
}

func TestHandleSignal_CompileError(t *testing.T) {
	s := &server{provider: healthyProvider()}
	req := httptest.NewRequest(http.MethodGet, "/api/signal?expr=TX.ask1", nil)
	rec := httptest.NewRecorder()

	newMux(s).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	json.Unmarshal(rec.Body.Bytes(), &body)
	if body["error"] == "" {
		t.Error("expected an error message in the response body")
	}
}

func TestHandleSignal_UpstreamFailure(t *testing.T) {
	s := &server{provider: &stubProvider{errs: map[quote.Symbol]error{quote.TX: errors.New("upstream 500")}}}
	req := httptest.NewRequest(http.MethodGet, "/api/signal", nil)
	rec := httptest.NewRecorder()

	newMux(s).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}
	var body map[string]string
	json.Unmarshal(rec.Body.Bytes(), &body)
	if !strings.Contains(body["error"], "TX") {
		t.Errorf("expected error to mention TX, got %q", body["error"])
	}
}

func TestHandleIndex(t *testing.T) {
	s := &server{provider: healthyProvider()}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	newMux(s).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	if !strings.Contains(rec.Body.String(), "台指訊號判斷引擎") {
		t.Error("expected the index page to contain the page title")
	}
}

func TestHandleShioajiStatus_Unconfigured(t *testing.T) {
	s := &server{
		provider:   healthyProvider(),
		configPath: filepath.Join(t.TempDir(), "does-not-exist.json"),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/shioaji/status", nil)
	rec := httptest.NewRecorder()

	newMux(s).ServeHTTP(rec, req)

	var resp shioajiStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Configured || resp.Active {
		t.Errorf("resp = %+v, want Configured=false Active=false", resp)
	}
}

func TestHandleShioajiStatus_ReflectsSavedConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := shioajiproc.Save(configPath, shioajiproc.Config{APIKey: "k", SecretKey: "s"}); err != nil {
		t.Fatal(err)
	}
	s := &server{provider: healthyProvider(), configPath: configPath}
	req := httptest.NewRequest(http.MethodGet, "/api/shioaji/status", nil)
	rec := httptest.NewRecorder()

	newMux(s).ServeHTTP(rec, req)

	var resp shioajiStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Configured {
		t.Errorf("resp = %+v, want Configured=true", resp)
	}
	// A saved config file doesn't imply an active connection until connectShioaji succeeds.
	if resp.Active {
		t.Errorf("resp = %+v, want Active=false", resp)
	}
}

func TestHandleShioajiConfig_MissingFields(t *testing.T) {
	s := &server{
		provider:   healthyProvider(),
		configPath: filepath.Join(t.TempDir(), "config.json"),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/shioaji/config", strings.NewReader(`{"api_key":"","secret_key":""}`))
	rec := httptest.NewRecorder()

	newMux(s).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleShioajiConfig_SavesAndReportsConnectFailure(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	s := &server{
		provider:       healthyProvider(),
		configPath:     configPath,
		shioajiManager: shioajiproc.NewManager(t.TempDir(), 19999), // no adapter script here -> Start() fails fast
	}
	req := httptest.NewRequest(http.MethodPost, "/api/shioaji/config", strings.NewReader(`{"api_key":"k","secret_key":"s"}`))
	rec := httptest.NewRecorder()

	newMux(s).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502, body = %s", rec.Code, rec.Body.String())
	}

	// The credentials should still be persisted even though connecting failed,
	// so a retry (or a fixed adapter install) doesn't require re-entering them.
	saved, err := shioajiproc.Load(configPath)
	if err != nil {
		t.Fatalf("expected config to be saved despite connect failure: %v", err)
	}
	if saved.APIKey != "k" || saved.SecretKey != "s" {
		t.Fatalf("saved config = %+v, want APIKey=k SecretKey=s", saved)
	}

	if s.currentProvider() == nil {
		t.Fatal("provider should remain set (falling back to taifex) after a failed connect")
	}
}
