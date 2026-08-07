# 台指訊號判斷引擎

持續運行的程式：定時拉取 TX（大台）/ MTX（小台）期貨報價，讓使用者用布林表達式（例如
`TX.a1 > TX.b1 && TX.volume > 1000`）描述判斷條件，程式即時算出結果是 `true` 還是
`false`。目前範圍只到「輸出 true/false」，不涉及下單。

完整規劃、階段劃分、未來方向見 [台指訊號判斷引擎規劃.md](./台指訊號判斷引擎規劃.md)。

## 目前進度

Phase 1～4 已完成，可跑可測；Phase 5（換券商 API）待辦。

| Phase | 內容 | 狀態 |
|---|---|---|
| 1 | 資料連線驗證 | ✅ |
| 2 | 資料層抽象（`QuoteProvider` 介面 + 定時輪詢 + 錯誤處理） | ✅ |
| 3 | 判斷引擎（`expr-lang/expr` 表達式求值） | ✅ |
| 4 | 整合測試（單元測試涵蓋多種市場情境） | ✅ 邏輯測試／⏸ 延遲量測待真即時資料源 |
| 5 | 換成券商 API（Fugle / Shioaji） | ⏸ 暫緩 |

### 重要限制：目前資料源不是即時的

FinMind 免費方案測過，即時期貨報價（`taiwan_futures_snapshot`）需要付費 Sponsor
會員才能用，免費版讀不到。目前改用 **台灣期交所（TAIFEX）官方公開 API**
（`openapi.taifex.com.tw`）當折衷資料源：

- 完全免費、不用註冊、不用 token
- 有 `a1`（委賣一）/`b1`（委買一）/成交價/成交量，欄位對得上規劃書需求
- **但一天只更新一次**（收盤後才有當天資料），不是逐筆即時報價，`time` 欄位是「該筆
  行情所屬的交易日」而非即時 tick 時間

要換成真即時資料，之後只需要新增一個實作 `quote.Provider` 介面的 struct（付費版
FinMind 或 Fugle），`internal/engine` 完全不用改。

## 需求

- Go 1.23 以上（開發時用 1.26.4）
- 網路連線（呼叫 `openapi.taifex.com.tw`）

## 專案結構

```
internal/quote/            QuoteProvider 介面、Quote 型別、Poller（定時輪詢 + 快取）
internal/quote/taifex/     TAIFEX 每日行情的 Provider 實作
internal/engine/           表達式編譯／求值（expr-lang/expr）、Env/Quote 對應
cmd/quotecheck/            Phase 1：單次拉一筆 TX/MTX 資料並印出
cmd/pollcheck/             Phase 2：驗證定時輪詢機制持續運作
cmd/signalcheck/           Phase 3：單次呼叫——抓最新 TX/MTX 資料、印出所有參數、求值輸出 true/false
cmd/server/                網頁介面：signalcheck 的邏輯包成 HTTP API + 內嵌前端頁面
scripts/build-release.sh   一次打包 Windows／macOS／Linux 的獨立執行檔到 dist/
```

## 使用方法

### 1. 單次連線測試（`quotecheck`）

```bash
go run ./cmd/quotecheck
```

輸出範例：

```
TX   date=2026-08-06 price=44280 volume=55986 b1(bid1)=44272 a1(ask1)=44284
MTX  date=2026-08-06 price=44274 volume=129055 b1(bid1)=44271 a1(ask1)=44275
```

### 2. 驗證定時輪詢（`pollcheck`）

```bash
go run ./cmd/pollcheck -interval=5m -print-interval=10s
```

- `-interval`：向 TAIFEX 拉資料的頻率（預設 5 分鐘；資料本身一天只變一次，不用調太快）
- `-print-interval`：印出目前快取內容的頻率（預設 10 秒）

背景輪詢，前景每隔 `print-interval` 印一次目前快取的 TX/MTX 資料；請求失敗時會印
`ERROR`，但保留上一次成功的資料，不會中斷。`Ctrl+C` 結束。

### 3. 判斷式引擎（`signalcheck`）

單次執行：呼叫當下重新抓一次 TX/MTX 最新資料、印出所有參數（含 a1/b1），再求值印出
`true`/`false`，然後結束——沒有背景輪詢迴圈，想要新的一筆結果就再執行一次。

```bash
go run ./cmd/signalcheck -expr "TX.a1 > TX.b1 && TX.volume > 1000"
```

輸出範例：

```
TX   date=2026-08-06 price=44280 volume=55986 b1(bid1)=44272 a1(ask1)=44284
MTX  date=2026-08-06 price=44274 volume=129055 b1(bid1)=44271 a1(ask1)=44275
expr: TX.a1 > TX.b1 && TX.volume > 1000
signal = true
```

- `-expr`：要求值的布林表達式，變數為 `TX.a1`/`TX.b1`/`TX.price`/`TX.volume`/`TX.time`、
  對應的 `MTX.*`、以及 `now`（目前系統時間，可呼叫 `now.Hour()` 等方法）；不帶這個參數時
  用預設值 `TX.a1 > TX.b1 && TX.volume > 1000`

表達式打錯欄位名稱或結果不是布林值，程式啟動時就會報編譯錯誤並結束，不會等到求值才炸：

```bash
go run ./cmd/signalcheck -expr "TX.ask1 > TX.b1"
# compile expression "TX.ask1 > TX.b1": type engine.Quote has no field ask1 (1:4)
```

TX 或 MTX 任一筆資料抓取失敗時，會印出錯誤訊息並以非 0 狀態碼結束，不會印出錯誤或不完整
的判斷結果。

### 4. 網頁介面（`server`）

不想每次都開終端機打指令的話，可以跑一個網頁版：`signalcheck` 的邏輯原封不動包成
HTTP API，前端是內嵌在執行檔裡的單一 HTML 頁面（無需另外安裝任何前端工具鏈），瀏覽器
打開一個網址、輸入表達式、按按鈕就能看結果。

```bash
go run ./cmd/server            # 預設監聽 :8080
go run ./cmd/server -addr=:9000  # 換 port
```

啟動後開瀏覽器到 `http://localhost:8080`，畫面上有一個表達式輸入框（預設值跟
`signalcheck` 一樣）跟一個查詢按鈕，按下去會顯示 TX/MTX 目前所有參數跟 TRUE/FALSE
結果。

也可以直接打 API，不透過網頁：

```bash
curl "http://localhost:8080/api/signal?expr=TX.a1%20%3E%20TX.b1"
# {"expr":"TX.a1 > TX.b1","tx":{...},"mtx":{...},"result":true}
```

跟 `signalcheck` 一樣，**每次請求都會即時重新呼叫資料源**，沒有背景輪詢或快取——這支
API 預期使用者頂多個位數，所以沒有處理多人併發打上游 API 的節流問題；之後如果使用人數
變多、或換成有配額限制的付費資料源，要再補上快取或 rate limit。

表達式錯誤回 `400`、資料源請求失敗回 `502`，都是 JSON `{"error": "..."}`，網頁端會把
錯誤訊息顯示出來而不是空白或當掉。

#### 打包成單一執行檔

前端 HTML 是用 `//go:embed` 直接包進執行檔的，所以 `go build` 出來就是一個不用裝任何
東西、雙擊就能跑的獨立檔案——不用另外裝 Go、Node 或任何 runtime。啟動時預設會自動打開
瀏覽器（`-open=false` 可以關掉這個行為）。

```bash
go build -o tx-signal-engine-server.exe ./cmd/server   # 建目前平台的執行檔
```

要一次建 Windows／macOS／Linux 的版本，跑：

```bash
scripts/build-release.sh
```

會在 `dist/` 產生 4 個獨立執行檔（`windows-amd64.exe`、`darwin-amd64`、`darwin-arm64`、
`linux-amd64`），每個都是約 13MB 的單一檔案，複製到別的電腦上不用裝任何東西直接雙擊
（macOS/Linux 要先 `chmod +x`）就能開網頁介面。

## 測試

```bash
go test ./...        # 跑全部單元測試
go test ./... -v      # 顯示每個測試案例的名稱與結果
```

四個套件都有測試：

- `internal/quote`：`Poller` 的快取行為——成功寫入、輪詢失敗時保留舊資料、錯誤在下次
  成功後自動清除、多商品互不干擾（用假的 `Provider` 驅動，不打真的網路）
- `internal/quote/taifex`：用 `httptest.Server` 模擬 TAIFEX 回應，涵蓋正常回應、HTTP
  錯誤、JSON 格式錯誤、查詢商品不存在；另外對近月合約篩選邏輯與數值解析
  （`-`/`NULL`/空字串等 TAIFEX 的佔位符）做了 table-driven 測試
- `internal/engine`：表達式編譯期錯誤（未知欄位、非布林回傳、語法錯誤）+ 多組市場
  情境的求值案例（多空條件、量能門檻、跨商品比較、邏輯運算、`now` 時間函式）
- `cmd/server`：HTTP handler 測試（`httptest`）——正常請求、預設表達式、編譯錯誤回
  400、資料源失敗回 502、首頁 HTML 正確回傳

> 這台環境沒有 C 編譯器，`go test -race` 因為 cgo 關閉跑不起來；`Poller` 目前只用單一
> mutex 保護內部 map，邏輯單純，有 cgo 環境時可以再補跑一次確認。

## 已知限制 / 下一步

- 資料是**每日**行情，不是即時報價，無法驗證真正的市場情境或量測延遲
- `a1`/`b1` 是最佳一檔委買委賣，不是完整五檔深度（規劃書實際需求也只到一檔）
- 要換即時資料源時：付費升級 FinMind Sponsor，或改接 [Fugle](https://www.fugle.tw/)
  （原生 REST + WebSocket）；兩者都只需新增一個 `quote.Provider` 實作
