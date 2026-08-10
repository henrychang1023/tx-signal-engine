# 台指訊號判斷引擎

持續運行的程式：定時拉取 TX（大台）/ MTX（小台）期貨報價，讓使用者用布林表達式（例如
`TX.a1 > TX.b1 && TX.volume > 1000`）描述判斷條件，程式即時算出結果是 `true` 還是
`false`。目前範圍只到「輸出 true/false」，不涉及下單。

完整規劃、階段劃分、未來方向見 [台指訊號判斷引擎規劃.md](./台指訊號判斷引擎規劃.md)。

## Quickstart（新電腦第一次使用）

### 事先要裝好的東西

| 東西 | 一定要嗎？ | 用途 |
|---|---|---|
| [Git](https://git-scm.com/) | 要 | 從 GitHub clone 這個 repo（也可以直接在 GitHub 網頁上下載 ZIP，不裝 Git 也行） |
| [Go 1.23+](https://go.dev/dl/)（開發用 1.26.4） | 要 | 這個專案沒有另外提供打包好的執行檔下載，要自己 `go build`／`go run` |
| [Python 3.12+](https://www.python.org/downloads/) | 選用 | 只有要用 **Shioaji 即時報價**才需要；不裝也能用，預設會走 TAIFEX 每日行情，不用任何密鑰 |
| 永豐金證券帳戶 + Shioaji API Key/Secret | 選用 | 同上，只有要即時報價才需要；帳戶開設、API 金鑰申請請洽永豐金 Shioaji 官方文件 |

### 步驟

1. Clone 這個 repo，進到資料夾：

   ```bash
   git clone https://github.com/henrychang1023/tx-signal-engine.git
   cd tx-signal-engine
   ```

2. 建執行檔並執行（或跳過建檔，直接用 `go run` 跑起來測試）：

   ```bash
   go build -o tx-signal-engine-server.exe ./cmd/server
   ./tx-signal-engine-server.exe
   ```

   會自動打開瀏覽器到 `http://localhost:8080`。不需要任何設定就能查詢——預設資料源是
   免費、不用密鑰的 TAIFEX 每日行情。

3.（選用）要接**即時報價**的話：

   ```bash
   python -m venv adapter\.venv
   adapter\.venv\Scripts\pip install -r adapter\requirements.txt
   ```

   裝好後回到網頁，展開「設定 Shioaji 即時報價」，貼上 API Key/Secret Key，按「啟用」——
   Go 會自動幫你拉起 Python adapter 並切換到即時報價，之後每次重開程式都會自動用存好的
   設定重新連線，不用每次都重輸入。

   完整細節（手動模式、`-provider` flag、已知限制）見下方「即時報價（Shioaji）」章節。

## 目前進度

Phase 1～5 都已完成，可跑可測。

| Phase | 內容 | 狀態 |
|---|---|---|
| 1 | 資料連線驗證 | ✅ |
| 2 | 資料層抽象（`QuoteProvider` 介面 + 定時輪詢 + 錯誤處理） | ✅ |
| 3 | 判斷引擎（`expr-lang/expr` 表達式求值） | ✅ |
| 4 | 整合測試（單元測試涵蓋多種市場情境） | ✅ 邏輯測試／⏸ 延遲量測待真即時資料源 |
| 5 | 換成券商 API（Shioaji） | ✅ 2026-08-10 已用正式環境帳號實測，盤中連線成功、拿到即時報價 |

### 資料源：TAIFEX（預設）與 Shioaji（即時，選用）

**預設資料源**仍是 **台灣期交所（TAIFEX）官方公開 API**（`openapi.taifex.com.tw`）：

- 完全免費、不用註冊、不用 token
- 有 `a1`（委賣一）/`b1`（委買一）/成交價/成交量，欄位對得上規劃書需求
- **但一天只更新一次**（收盤後才有當天資料），不是逐筆即時報價，`time` 欄位是「該筆
  行情所屬的交易日」而非即時 tick 時間

要拿真正即時的報價，現在可以改用 **Shioaji**（永豐金證券）——見下面「即時報價
（Shioaji）」一節。兩種資料源都實作同一個 `quote.Provider` 介面，`internal/engine`
完全不用改，用 `-provider` flag 切換。

### 即時報價（Shioaji）

Shioaji 官方 SDK 只有 Python，所以用一支獨立的 Python adapter 進程
（`adapter/shioaji_adapter.py`）負責登入 Shioaji、訂閱 TX/MTX（大台/小台近月合約）
的 tick + 最佳一檔委買委賣，並把最新快照用本機 HTTP 服務（預設
`http://127.0.0.1:8787`）暴露出來；Go 這邊的 `internal/quote/shioaji.Provider`
就是打這個本機服務，跟 `taifex.Provider` 打遠端 REST API 的寫法幾乎一樣。

不管哪種啟用方式，**Python 環境都要先手動裝好一次**（Go 不會自動安裝 Python）：

```powershell
python -m venv adapter\.venv
adapter\.venv\Scripts\pip install -r adapter\requirements.txt
```

只讀報價不下單，所以不需要 CA 憑證。合約選擇是抓 Shioaji 回傳的該商品合約清單、
自己依到期日挑最近月——這個 SDK 版本（`shioaji==1.7.2`）沒有 `TXFR1`/`MXFR1`
這種近月別名可以直接查，細節見 `adapter/shioaji_adapter.py` 內的註解。

#### 透過網頁設定（推薦）

`cmd/server` 網頁版有一個「設定 Shioaji 即時報價」的收合區塊，展開後直接貼上
API Key/Secret Key、按「啟用」，Go 會自動：

1. 存進執行檔同目錄下的 `config.json`（明碼 JSON，不進 git；之後才會考慮換成
   OS 憑證保管箱）
2. 自動拉起 `adapter/shioaji_adapter.py` 子行程（不用自己開第二個終端機、不用手動
   設環境變數），輪詢它的健康檢查最多 30 秒
3. 成功後把網頁查詢用的資料源從 TAIFEX 切換成 Shioaji

下次啟動 `cmd/server` 時，只要 `config.json` 還在，會自動用裡面存的憑證重新連線，
不用每次都重新輸入。網頁上任何時候都看得到目前是用 TAIFEX 還是 Shioaji、連線失敗的
話也會顯示錯誤訊息；`GET /api/shioaji/status` 這支 API 只回布林狀態，不會把
key/secret 傳回瀏覽器。

```bash
go run ./cmd/server   # 不用加任何 -provider flag，開瀏覽器後在網頁上設定即可
```

#### 手動兩終端機（進階／除錯用）

`quotecheck`/`pollcheck`/`signalcheck` 這三支 CLI，以及 `cmd/server` 的
`-provider`/`-shioaji-adapter-url` flag，維持原本手動啟動 adapter 的模式，適合
開發除錯、或不想讓 Go 幫你管理 Python 子行程的情境：

1. 設定環境變數（自己的 shell 設，不要寫進任何檔案）：

   ```powershell
   $env:SHIOAJI_API_KEY = "..."
   $env:SHIOAJI_SECRET_KEY = "..."
   ```

2. 啟動 adapter：

   ```powershell
   adapter\.venv\Scripts\python adapter\shioaji_adapter.py
   ```

   看到 `serving on http://127.0.0.1:8787 ...` 代表登入、訂閱都成功了。

3. 另開一個終端機，加上 `-provider=shioaji` 跑 Go 端：

   ```bash
   go run ./cmd/quotecheck -provider=shioaji
   go run ./cmd/pollcheck -provider=shioaji
   go run ./cmd/signalcheck -provider=shioaji -expr "TX.a1 > TX.b1 && TX.volume > 1000"
   go run ./cmd/server -provider=shioaji
   ```

   adapter 沒有跑在預設 port 的話，加 `-shioaji-adapter-url=http://127.0.0.1:xxxx`。

   這個模式跟網頁設定模式是各自獨立的路徑，不要同時用——手動啟動的 adapter 跟
   Go 自動拉起的 adapter 子行程會搶同一個預設 port（`8787`）。

`SHIOAJI_SIMULATION=true` 可以切成模擬環境（預設為正式環境 `false`），兩種啟用
方式都吃這個環境變數。

## 需求

- Go 1.23 以上（開發時用 1.26.4）
- 網路連線（呼叫 `openapi.taifex.com.tw`，或 Shioaji 的伺服器）
- 要用 Shioaji 即時報價才需要：Python 3.12+、永豐金證券帳戶及 Shioaji API key/secret

## 專案結構

```
internal/quote/            QuoteProvider 介面、Quote 型別、Poller（定時輪詢 + 快取）
internal/quote/taifex/     TAIFEX 每日行情的 Provider 實作
internal/quote/shioaji/    打本機 Shioaji adapter 的 Provider 實作
internal/shioajiproc/      cmd/server 用來管理 Shioaji adapter 子行程 + config.json 的套件
internal/engine/           表達式編譯／求值（expr-lang/expr）、Env/Quote 對應
adapter/shioaji_adapter.py Python：登入 Shioaji、訂閱 TX/MTX、本機 HTTP 服務暴露最新報價
cmd/quotecheck/            Phase 1：單次拉一筆 TX/MTX 資料並印出
cmd/pollcheck/             Phase 2：驗證定時輪詢機制持續運作
cmd/signalcheck/           Phase 3：單次呼叫——抓最新 TX/MTX 資料、印出所有參數、求值輸出 true/false
cmd/server/                網頁介面：signalcheck 的邏輯包成 HTTP API + 內嵌前端頁面
scripts/build-release.sh   一次打包 Windows／macOS／Linux 的獨立執行檔到 dist/
```

以上四支 `cmd/*` 都支援 `-provider=taifex|shioaji`（預設 `taifex`，不需要密鑰就能跑）。

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
go run ./cmd/server                    # 預設監聽 127.0.0.1:8080（只有這台機器連得到）
go run ./cmd/server -addr=:9000        # 換 port，同時開放給同網段其他裝置連
```

預設只監聽 `127.0.0.1`，不是 `:8080`（所有介面）——因為這支網頁現在會經手 Shioaji
API secret，預設不對外網段開放比較安全；真的要讓同網段其他裝置連得到，自己用
`-addr` 指定成 `:8080` 或 `0.0.0.0:8080`。

啟動後開瀏覽器到 `http://localhost:8080`，畫面上有一個表達式輸入框（預設值跟
`signalcheck` 一樣）跟一個查詢按鈕，按下去會顯示 TX/MTX 目前所有參數跟 TRUE/FALSE
結果；上方還有一個「設定 Shioaji 即時報價」收合區塊，見上面「透過網頁設定（推薦）」
一節。

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

六個套件都有測試：

- `internal/quote`：`Poller` 的快取行為——成功寫入、輪詢失敗時保留舊資料、錯誤在下次
  成功後自動清除、多商品互不干擾（用假的 `Provider` 驅動，不打真的網路）
- `internal/quote/taifex`：用 `httptest.Server` 模擬 TAIFEX 回應，涵蓋正常回應、HTTP
  錯誤、JSON 格式錯誤、查詢商品不存在；另外對近月合約篩選邏輯與數值解析
  （`-`/`NULL`/空字串等 TAIFEX 的佔位符）做了 table-driven 測試
- `internal/quote/shioaji`：用 `httptest.Server` 模擬本機 adapter 回應，涵蓋正常回應、
  尚無資料（503）、HTTP 錯誤、JSON/時間格式錯誤
- `internal/engine`：表達式編譯期錯誤（未知欄位、非布林回傳、語法錯誤）+ 多組市場
  情境的求值案例（多空條件、量能門檻、跨商品比較、邏輯運算、`now` 時間函式）
- `cmd/server`：HTTP handler 測試（`httptest`）——正常請求、預設表達式、編譯錯誤回
  400、資料源失敗回 502、首頁 HTML 正確回傳，以及 `/api/shioaji/config`、
  `/api/shioaji/status` 的成功/失敗案例
- `internal/shioajiproc`：`Config` 的 `Load`/`Save` round-trip、找不到檔案時的錯誤、
  `LocateAdapterDir` 在「CWD 找得到」跟「要 fallback 到 exe 目錄」兩種情境下的判斷；
  實際拉起 Python 子行程這塊沒有自動化測試（跟 `adapter/shioaji_adapter.py` 一樣，
  只能手動驗證），但純路徑邏輯（`pythonPath`）有覆蓋到

`adapter/shioaji_adapter.py` 需要真的登入 Shioaji，沒有自動化測試，只能手動連線驗證
（見上面「即時報價（Shioaji）」）。

> 這台環境沒有 C 編譯器，`go test -race` 因為 cgo 關閉跑不起來；`Poller` 目前只用單一
> mutex 保護內部 map，邏輯單純，有 cgo 環境時可以再補跑一次確認。

## 已知限制 / 下一步

- `taifex` 資料是**每日**行情，不是即時報價；`shioaji` 已經在正式環境盤中實測過，
  確認能拿到即時報價（2026-08-10），但還沒有系統性量測延遲數字
- `a1`/`b1` 是最佳一檔委買委賣，不是完整五檔深度（規劃書實際需求也只到一檔）
- `config.json`（網頁設定 Shioaji 存的憑證）是明碼 JSON，只做到「不進 git、檔案權限
  設成 0600」——之後如果要更進一步，會換成 OS 憑證保管箱（Windows Credential
  Manager／macOS Keychain）
- 網頁關閉 `cmd/server` 這支程式時，Go 目前不會主動去 kill 掉它拉起的 Python adapter
  子行程，可能會變成孤兒行程留在背景——沒有做行程群組/Job Object 管理；真的要清乾淨
  可以用工作管理員手動關掉 `python.exe`
- 手動兩終端機模式（`-provider=shioaji`）跟網頁自動管理模式各自獨立，不要同時用，
  兩者預設都搶 `8787` port
- Shioaji SDK 的合約近月別名、tick/bidask 欄位名稱是照目前安裝的版本
  （`shioaji==1.7.2`）寫的，SDK 升級後如果欄位有變動要對照調整；實測發現這個版本的
  型別定義檔（`.pyi`）跟實際執行時的行為有多處對不上（`api.contracts.futures` 要傳
  `root` 參數、沒有 `TXFR1`/`MXFR1` 近月別名可查等），細節見程式內的註解
