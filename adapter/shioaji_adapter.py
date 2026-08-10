"""Shioaji quote adapter.

Bridges the Python-only Shioaji SDK to the Go signal engine: logs into
Shioaji, subscribes to TX/MTX (front-month TXF/MXF) tick + best-bid/ask
data, keeps the latest snapshot of each in memory, and serves it over a
local HTTP endpoint that internal/quote/shioaji.Provider (Go) polls.

Run standalone: python shioaji_adapter.py
Requires env vars SHIOAJI_API_KEY / SHIOAJI_SECRET_KEY. Optional:
SHIOAJI_ADAPTER_PORT (default 8787), SHIOAJI_SIMULATION (default "false").
"""

import json
import os
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import urlparse, parse_qs

import shioaji as sj

SYMBOL_TO_PRODUCT = {"TX": "TXF", "MTX": "MXF"}

# Guards the two dicts below; the HTTP handler thread(s) and Shioaji's own
# callback thread(s) both touch them.
_lock = threading.Lock()
_snapshots = {}  # "TX"/"MTX" -> dict ready to json.dumps
_ticks = {}      # "TX"/"MTX" -> latest TickFOPv1, to merge with bidask updates
_bidasks = {}    # "TX"/"MTX" -> latest BidAskFOPv1


def _env(name, default=None, required=False):
    value = os.environ.get(name, default)
    if required and not value:
        print(f"missing required environment variable {name}", file=sys.stderr)
        sys.exit(1)
    return value


def _front_month_contract(api, product_code):
    """api.contracts.futures(root) returns a flat list of FuturesInfo for
    that root (confirmed by inspection against the installed SDK build —
    there's no separate rollover-alias lookup here), so just pick the
    nearest upcoming delivery date ourselves."""
    contracts = api.contracts.futures(product_code)
    if not contracts:
        raise RuntimeError(f"no contracts found for {product_code}")
    return min(contracts, key=lambda c: c.delivery_date)


def _merge_snapshot(symbol):
    tick = _ticks.get(symbol)
    bidask = _bidasks.get(symbol)
    if tick is None or bidask is None:
        return
    _snapshots[symbol] = {
        "symbol": symbol,
        "ask1": float(bidask.ask_price[0]) if bidask.ask_price else 0.0,
        "bid1": float(bidask.bid_price[0]) if bidask.bid_price else 0.0,
        "price": float(tick.close),
        "volume": int(tick.total_volume),
        "time": max(tick.datetime, bidask.datetime).astimezone().isoformat(),
    }


def _symbol_for_code(code):
    for symbol, product in SYMBOL_TO_PRODUCT.items():
        if code.startswith(product):
            return symbol
    return None


def make_handler(get_port):
    class Handler(BaseHTTPRequestHandler):
        def log_message(self, fmt, *args):
            pass  # keep stdout limited to our own startup/status logging

        def _write_json(self, status, payload):
            body = json.dumps(payload).encode("utf-8")
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def do_GET(self):
            parsed = urlparse(self.path)
            if parsed.path == "/health":
                self._write_json(200, {"status": "ok"})
                return
            if parsed.path == "/quote":
                symbol = parse_qs(parsed.query).get("symbol", [""])[0]
                if symbol not in SYMBOL_TO_PRODUCT:
                    self._write_json(400, {"error": f"unknown symbol {symbol!r}"})
                    return
                with _lock:
                    snapshot = _snapshots.get(symbol)
                if snapshot is None:
                    self._write_json(503, {"error": f"no data yet for {symbol}"})
                    return
                self._write_json(200, snapshot)
                return
            self._write_json(404, {"error": "not found"})

    return Handler


def main():
    api_key = _env("SHIOAJI_API_KEY", required=True)
    secret_key = _env("SHIOAJI_SECRET_KEY", required=True)
    port = int(_env("SHIOAJI_ADAPTER_PORT", "8787"))
    simulation = _env("SHIOAJI_SIMULATION", "false").strip().lower() in ("1", "true", "yes")

    api = sj.Shioaji(simulation=simulation)
    print(f"logging in (simulation={simulation})...")
    api.login(api_key=api_key, secret_key=secret_key)

    # login() already kicks off a contract fetch in the background; calling
    # fetch_contracts() again here races it ("exclusive access lost"), and
    # the status property the SDK docs advertise isn't actually present at
    # runtime on this version. So instead of watching for a "done" signal,
    # just retry the lookup itself until the contract data shows up.
    print("waiting for contracts to finish fetching...")
    deadline = time.monotonic() + 30
    contracts = {}
    last_err = None
    while time.monotonic() < deadline:
        try:
            contracts = {
                symbol: _front_month_contract(api, product)
                for symbol, product in SYMBOL_TO_PRODUCT.items()
            }
            break
        except Exception as e:
            last_err = e
            time.sleep(0.5)
    else:
        print(f"timed out waiting for contracts to fetch: {last_err}", file=sys.stderr)
        sys.exit(1)

    for symbol, contract in contracts.items():
        print(f"{symbol} -> {contract.code} (delivery {contract.delivery_date})")

    @api.on_tick_fop_v1()
    def on_tick(tick):
        symbol = _symbol_for_code(tick.code)
        if symbol is None:
            return
        with _lock:
            _ticks[symbol] = tick
            _merge_snapshot(symbol)

    @api.on_bidask_fop_v1()
    def on_bidask(bidask):
        symbol = _symbol_for_code(bidask.code)
        if symbol is None:
            return
        with _lock:
            _bidasks[symbol] = bidask
            _merge_snapshot(symbol)

    for symbol, contract in contracts.items():
        api.subscribe(contract, quote_type="tick", version=sj.QuoteVersion.v1)
        api.subscribe(contract, quote_type="bidask", version=sj.QuoteVersion.v1)

    server = ThreadingHTTPServer(("127.0.0.1", port), make_handler(lambda: port))
    print(f"serving on http://127.0.0.1:{port} (GET /health, GET /quote?symbol=TX|MTX)")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass
    finally:
        server.server_close()
        api.logout()


if __name__ == "__main__":
    main()
