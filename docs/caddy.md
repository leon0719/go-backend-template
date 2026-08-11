# Cloudflare + Caddy Setup

1. Point a proxied (orange-cloud) DNS A record at your host.
2. Set Cloudflare SSL/TLS mode to **Full** (not Full Strict — the origin cert is self-signed).
3. Open inbound port 443 on the host firewall.
4. Set `DOMAIN` in `.env.prod`.
5. `docker/Caddyfile` uses `tls internal` to generate a self-signed origin certificate; Cloudflare terminates visitor-facing TLS and encrypts the Cloudflare→origin leg against this cert.

Not using Caddy? Remove the `caddy` service from `docker/docker-compose.prod.yml` and front the `api` service with your own reverse proxy. If that proxy runs on a different host than `api`, firewall port 8000 from the public internet — the `api` service itself does not expose a `ports:` mapping in `docker-compose.prod.yml`, so it's only reachable from other containers on the Compose network unless you add one.
