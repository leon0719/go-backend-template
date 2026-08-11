# Production Deployment (single-host Docker Compose)

1. Copy `.env.prod.example` to `.env.prod`, `chmod 600 .env.prod`, and fill in `DATABASE_URL`, `REDIS_URL`, `JWT_SECRET`, `DOMAIN` (and optionally `ARTICLE_PUBLISHED_WEBHOOK_URL`, `GIT_COMMIT_SHA`).
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
