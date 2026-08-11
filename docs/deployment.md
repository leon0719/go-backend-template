# Production Deployment (single-host Docker Compose)

1. Copy `.env.prod.example` to `.env.prod`, `chmod 600 .env.prod`, and fill in `DATABASE_URL`, `REDIS_URL`, `JWT_SECRET`, `DOMAIN`, `TRUSTED_PROXIES` (see below) (and optionally `CORS_ALLOWED_ORIGINS`, `ARTICLE_PUBLISHED_WEBHOOK_URL`, `GIT_COMMIT_SHA`).
2. Build and start the stack:
   ```bash
   docker compose -f docker/docker-compose.prod.yml up -d
   ```
   `migrate` is the only service with a `build:` block; it builds a single image (`go-backend-template:latest`) containing all three binaries (`/api`, `/worker`, `/migrate`) and runs `/migrate up` once. `api` and `worker` reference that same image (via `entrypoint: ["/api"]` / `["/worker"]`) and wait for `migrate` to exit 0 (`depends_on: condition: service_completed_successfully`) before starting.
3. Expose via Cloudflare (orange-cloud) + the bundled `caddy` service — see [docs/caddy.md](caddy.md).
4. Health checks: `/health/live` (process only) and `/health/ready` (DB + Redis) are served by `api`. The `api` service's Docker healthcheck invokes the binary itself in probe mode (`/api -healthcheck`, which GETs its own `/health/live`) since the distroless prod image has no shell/curl/wget.

## Notes

- `GIT_COMMIT_SHA` build arg is optional; pass it via `GIT_COMMIT_SHA=$(git rev-parse HEAD) docker compose -f docker/docker-compose.prod.yml build` if you want it baked into the `/api` binary (surfaced in its startup log line).
- Secrets injected via `env_file` are visible through `docker inspect`/`docker exec ... env` to anyone with Docker daemon access — fine on a single host you control; use Docker secrets or an external secrets manager on a shared daemon.
- `worker` runs `cmd/worker`, the asynq task consumer; it shares the same env vars as `api` (including `DATABASE_URL` and `REDIS_URL`) and also waits on `migrate`.
- The prod image is built from `docker/Dockerfile.prod`: a `golang:1.26-alpine` build stage producing three static (`CGO_ENABLED=0`) binaries, copied into a `gcr.io/distroless/static-debian12` final stage.

## Client IP resolution behind a proxy (`TRUSTED_PROXIES`) — read this

The bundled `caddy` service reverse-proxies to `api`, so from the API's point
of view `r.RemoteAddr` is **always Caddy's container IP**. IP-based rate
limiting on `/auth/register` and `/auth/login` would therefore key every
internet user into a single shared bucket (a global 10 req/min — effectively a
self-inflicted DoS with zero per-attacker protection).

`middleware.RealIP` fixes this, with a strict trust contract:

- `X-Forwarded-For` is **attacker-controlled** and is consulted **only** when
  the direct peer's address falls inside a `TRUSTED_PROXIES` CIDR.
- From an untrusted peer the header is **ignored entirely** and `RemoteAddr`
  is used — a client cannot spoof its way past the throttle.
- When the peer is trusted, the client IP is the **rightmost** entry in
  `X-Forwarded-For` that is not itself a trusted proxy. Entries further left
  may have been forged by the client before your edge ever saw the request.
- Empty `TRUSTED_PROXIES` (the default) means "trust nothing" — correct for a
  directly internet-facing server, wrong behind any proxy.

Getting this wrong fails in one of two ways, both bad:

| Misconfiguration | Consequence |
|---|---|
| Unset while behind a proxy | All clients share one rate-limit bucket; one attacker locks out everyone |
| Too broad (e.g. `0.0.0.0/0`) | Any client spoofs `X-Forwarded-For` and bypasses rate limiting entirely |

`.env.prod.example` ships `TRUSTED_PROXIES=172.16.0.0/12`, which matches the
Docker bridge range Compose allocates for the bundled `caddy`. If you replace
Caddy with a proxy on another host, narrow this to that proxy's exact address
(e.g. `10.0.1.5/32`). Verify with the `remote_ip` field in the request log
line: it should show real client addresses, not `172.x.x.x`.

## Server timeouts and graceful shutdown

`cmd/api` runs an explicit `http.Server` with `ReadHeaderTimeout` (10s),
`ReadTimeout` (30s) and `IdleTimeout` (120s) set — `ReadHeaderTimeout` in
particular closes the Slowloris hole that bare `http.ListenAndServe` leaves
open. `WriteTimeout` is intentionally `0` (unbounded) because it would sever
long-lived SSE streams mid-response; bound streaming inside the handler
instead.

Both `api` and `worker` handle `SIGINT`/`SIGTERM`: the API calls
`srv.Shutdown` with a 20s grace period so in-flight requests drain (and the
deferred pool/asynq-client `Close` calls actually run), and the worker calls
`asynq.Server.Shutdown()` so in-flight tasks finish rather than being killed
mid-deploy.
