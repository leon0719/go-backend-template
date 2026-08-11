# go-backend-template 設計

## 背景與目標

以既有的 `django-ninja-backend-template` 為對照範本，打造一個規模完整對等的 Go 後端模板：分層架構、範例 app（auth、CRUD、realtime）、背景任務、health check、Docker dev/prod、部署文件等全部對映過來，作為未來 Go 後端專案的起點。

module path：`go-backend-template`。

## Stack 對照表

| Layer | Django Ninja 模板 | Go 模板 |
|---|---|---|
| Web/路由 | Django Ninja (ASGI) | `net/http`（1.22+ pattern routing）+ `chi` |
| DB | PostgreSQL + psycopg | PostgreSQL + `pgx` |
| DB 存取 | Django ORM | `sqlc`（純 SQL → 型別安全 Go code） |
| Migration | Django migrations | `goose` |
| 背景任務 | Celery + Redis | `asynq` + Redis |
| Config | pydantic Settings 讀 `.env` | 自訂 `Config` struct + `caarlos0/env` |
| 日誌 | loguru | `log/slog`（JSON handler in prod，text in dev） |
| Auth | Opaque bearer token | JWT access + DB-backed refresh token（可撤銷） |
| API 文件 | Django Ninja 自動 Swagger | `swaggo/swag`（annotation → openapi spec + Swagger UI） |
| Realtime | WebSocket + SSE | SSE only |
| 管理後台 | Django admin | 無（不做，需要時再評估） |
| 部署 | Docker Compose + Caddy + Cloudflare | 同樣模式對映 |

## 專案結構

```
cmd/
  api/main.go            # HTTP server 進入點
  worker/main.go         # asynq worker + scheduler 進入點
internal/
  config/                # Config struct，讀 env
  httpserver/
    router.go            # chi router 組裝
    middleware/           # request-id, slog logger, recover, cors, rate-limit, jwtauth
    respond/              # 統一 JSON 回應/錯誤格式 helper
  db/
    queries/*.sql         # sqlc 輸入
    sqlc/                 # sqlc 產生的 Go code（commit 進 repo，CI 檢查 diff 防漂移）
    migrations/*.sql      # goose migration（-- +goose Up / Down）
  accounts/               # 範例 app：handler/service/repository/schema（JWT auth）
  articles/                # 範例 app：CRUD + ownership + pagination + throttle + task
  health/                  # /health/live, /health/ready
  realtime/                # SSE 範例
  tasks/                    # asynq task type 常數 + payload struct
  logging/                  # slog 初始化 + request-id 綁定
pkg/                        # 目前空，未來對外重用才放
docker/
  Dockerfile.dev（air 熱重載）/ Dockerfile.prod（multi-stage build）
  docker-compose.dev.yml / docker-compose.prod.yml
  Caddyfile
docs/                      # 對映原模板的規範文件
Makefile
go.mod（module: go-backend-template）
.air.toml
```

### 分層慣例（對映 App Layering）

- `handler.go` — HTTP handler，只做 request/response 轉換，不含業務邏輯
- `schema.go` — request/response struct + validation（`go-playground/validator`）
- `service.go` — 業務邏輯，stateless，回傳 domain error
- `repository.go` — 包裝 sqlc 產生的 `Queries`，該 app 專屬的資料存取，把 sqlc error 轉成 domain error
- `external.go` — 第三方 HTTP 呼叫，統一 retry/log wrapper（對映 `external_services.py`）
- `tasks.go` — asynq task handler，參數用 primitive/JSON，task 必須 idempotent

## Web 層與中介層

chi router，middleware 鏈：`RequestID → Recoverer → SlogLogger → CORS → (route-specific: RateLimit / JWTAuth)`。

統一錯誤回應格式：`{"error": {"code": "...", "message": "..."}}`，對映 `exceptions.py` 的 422/401/404/500 handler。

## API 文件（swaggo/swag）

Handler 上以 comment annotation 標註 route/params/response，`swag init` 產生 openapi spec，`http-swagger` 掛在 `/api/docs`。Makefile 提供 `make docs`；CI 檢查 `swag init` 後 `git diff` 為空，避免 spec 與程式碼漂移。

## 資料層（sqlc + pgx + goose）

- Migration：`internal/db/migrations/*.sql`，goose 格式，`make migrate` / `make migrate-down`
- Query：`internal/db/queries/*.sql`，sqlc 註解標記型別（如 `-- name: GetUserByID :one`）
- `sqlc generate` 產物 commit 進 repo，CI 檢查 diff 防手動漂移
- 連線池：`pgxpool`，連線字串來自 `Config.DatabaseURL`
- 每個 app 的 `repository.go` 包一層 `*sqlc.Queries`，handler/service 不直接碰 sqlc

## Auth（JWT access + DB refresh token）

- `POST /api/v1/auth/register` — bcrypt hash 密碼，回 access + refresh
- `POST /api/v1/auth/login` — 驗證密碼，簽發 access token（15 分、HS256，claims: `sub`/`exp`/`iat`）+ refresh token（random 32B，SHA-256 digest 存 DB，30 天）
- `POST /api/v1/auth/refresh` — 用 refresh 換新 access，並 rotate refresh（舊的作廢，防重放）
- `POST /api/v1/auth/logout` — 撤銷該使用者所有 refresh token（`revoked_at` 標記）
- `GET /api/v1/auth/me` — 需 access token
- `middleware/jwtauth.go` 解析 `Authorization: Bearer <token>`，驗簽/驗期，userID 放入 `context.Context`
- `register` / `login` 用 Redis token bucket 做 IP rate limit（對映 `AnonRateThrottle`）

## 範例 CRUD app：articles

- Endpoints：`GET/POST /api/v1/articles`、`GET/PATCH/DELETE /api/v1/articles/{id}`
- Ownership：repository 查詢加 `WHERE user_id = $owner`，查不到即 404（IDOR → 404 慣例）
- Pagination：`?page=&page_size=`，回傳 `{"items":[], "total":, "page":, "page_size":}`
- Filter：`?status=&q=`
- 寫入操作 per-user Redis rate limit
- draft→published：`UPDATE ... WHERE status='draft' RETURNING id`（rows-affected 判斷避免 race），成功後 enqueue asynq task 呼叫 webhook

## 背景任務（asynq）

- `internal/tasks/` 定義 task type 常數 + JSON payload struct
- `cmd/worker/main.go` 啟動 asynq server + 註冊 handler
- `asynq.Scheduler` 對映 Celery Beat（例如心跳 ping 任務）
- Task 必須 idempotent，payload 只放 primitive/ID，handler 內重查 DB

## Realtime（SSE only）

`GET /api/v1/realtime/sse`，JWT auth，示範 token-by-token 假 LLM 回應後送 `[DONE]`。用 `http.Flusher` + channel 推送。跨程序（多 worker）廣播需 Redis Pub/Sub，本版標記為未來可選項，不在初版範圍內。

## Logging（slog）

- `internal/logging` 初始化：dev = `slog.NewTextHandler`，prod = `slog.NewJSONHandler`
- Request-ID middleware 產生/延續 `X-Request-ID`，綁進 `slog` context 並回填 response header
- 4xx → `slog.Warn`，5xx → `slog.Error`

## Config

`internal/config.Config` struct，`caarlos0/env` 讀取，`.env.{ENV}` 用 `godotenv` load（`ENV` 預設 `local`）。所有環境變數集中在此，app code 不直接呼叫 `os.Getenv`。

## Docker / 部署

- `docker/Dockerfile.dev`：`air` 熱重載（監看 `.go` 檔案變動自動重新 build + 重啟）
- `docker/Dockerfile.prod`：multi-stage build，最終 image 用 distroless/alpine
- `docker-compose.dev.yml`：api、postgres、redis、worker、scheduler
- `docker-compose.prod.yml`：外部 DB/Redis、`GIT_COMMIT_SHA` build arg、一次性 `migrate` job 擋在 api/worker 前、caddy service（Cloudflare 橘雲 + Full SSL + `tls internal`）
- `/health/live`、`/health/ready`（DB + Redis + asynq 佇列可達性）供 Docker healthcheck 使用

## 測試

- 單元測試：`testing` + `testify/assert`，service 層對 repository 用 interface + mock
- 整合測試：`testcontainers-go` 起 Postgres/Redis，跑真實 repository 查詢；用 build tag 或 `-short` flag 區分 unit/integration
- Handler 測試：`net/http/httptest`

## CI

`.github/workflows/ci.yml`：`gofmt -l` / `golangci-lint run` / `go vet` / `go build ./...` / `go test ./...`（unit）/ 帶 service container 的 integration test / prod Docker build / `swag init` diff 檢查

## Makefile 對映

`make up/down/rebuild/logs-api`（docker）· `make format/lint/vet`（Go 沒有獨立 type-check，用 `go vet` + `golangci-lint` 取代）· `make test` / `make test-integration` · `make migrate` / `make migrate-down` · `make docs`（`swag init`）

## 文件（`docs/`）

對映原模板 5 份：`local-development.md`、`deployment.md`、`caddy.md`、`api-standards.md`、`backend-standards.md`。`alerting.md` 對映項目（Sentry、Celery beat heartbeat 等）在 Go 生態沒有預設等價整合，文件中明確標記「待補」而非造假對齊。

## 範圍邊界（本版不做）

- Django admin 等價的管理後台
- WebSocket（先做 SSE，WS 之後可再評估）
- 跨 worker 程序的 SSE 廣播（需 Redis Pub/Sub，標記為未來項）
