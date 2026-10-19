# Self-hosting

ChasteDesk runs as two Go processes and one Postgres database. The repository ships a Compose stack that builds both binaries, applies migrations, and starts the web and worker services.

## Requirements

- Docker Engine 24+ with Compose v2
- 1 vCPU and 1 GB RAM for a small desk
- A domain name and TLS if the instance is public
- Optional: an API key for OpenAI, Anthropic, Google AI, or a reachable Ollama server

## First run

```bash
cp .env.example .env
openssl rand -base64 48
# paste the value into APP_SECRET

docker compose up -d --build --wait
```

The stack exposes the web app on `${PORT:-3000}`. Open it, complete `/setup`, then follow the onboarding steps.

```bash
docker compose ps
docker compose logs -f web worker
curl -fsS http://localhost:3000/api/health
curl -fsS http://localhost:3000/api/ready
```

`/api/health` reports that the process is alive. `/api/ready` also pings Postgres.

## Configuration

| Variable | Required | Notes |
| --- | --- | --- |
| `APP_SECRET` | yes | At least 32 bytes. Used for HKDF subkeys, AES-256-GCM, widget tokens, and session security. |
| `APP_URL` | yes | Public URL. Set the exact scheme and host. Widget snippets and origin checks depend on it. |
| `POSTGRES_PASSWORD` | yes in production | Compose builds the DSN from it. |
| `PORT` | no | Host port mapping only. The container always listens on 3000. |
| `APP_ENV` | compose sets `production` | Do not lower it in production. The SSRF test escape hatch is inert when the value is `production`. |
| `LOG_LEVEL` | no | `info` is a good default. Use `debug` briefly when diagnosing. |
| `WORKER_CONCURRENCY` | no | Workers per queue. Four is fine for a small desk. |
| `UPLOADS_DIR` | no | Defaults to `/data/uploads` in the worker container. |

Changing `APP_SECRET` invalidates stored provider keys and widget tokens. Re-enter provider keys and re-copy the widget snippet after a rotation.

## Reverse proxy and TLS

Set `APP_URL` to the public HTTPS URL before first start. That makes session cookies `Secure`, enables HSTS, and keeps CSRF origin checks aligned with the browser.

Caddy example:

```caddyfile
support.example.com {
  reverse_proxy 127.0.0.1:3000
}
```

Nginx example:

```nginx
server {
  listen 443 ssl http2;
  server_name support.example.com;

  location / {
    proxy_pass http://127.0.0.1:3000;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_buffering off;
  }
}
```

`proxy_buffering off` is important for SSE streams. Keep request body limits at or below 16 MiB so uploads reach the application intact.

## Data and backups

Two named volumes hold state:

- `pgdata`: Postgres data directory.
- `uploads`: uploaded knowledge files.

Back up Postgres logically:

```bash
docker compose exec -T postgres pg_dump -U chastedesk -Fc chastedesk > chastedesk.dump
```

Restore:

```bash
docker compose stop web worker
docker compose exec -T postgres pg_restore -U chastedesk -d chastedesk --clean --if-exists < chastedesk.dump
docker compose start web worker
```

Back up the uploads volume with `docker run --rm -v chastedesk_uploads:/data -v "$PWD":/backup alpine tar czf /backup/uploads.tgz -C /data .` and adjust the volume name to your Compose project.

## Upgrades

```bash
git pull
docker compose build
docker compose up -d --wait
```

Migrations run in the `migrate` service before web and worker start. The web process also applies migrations on boot. Read `CHANGELOG.md` before a major upgrade.

## Scaling

- Run one web replica unless you replace the SSE listener with a shared pub/sub layer.
- The worker is safe to scale horizontally because jobs use `FOR UPDATE SKIP LOCKED`.
- Postgres is the bottleneck for retrieval at M1 scale. Add memory and IOPS before adding replicas.
- Exact cosine search is used because `knowledge_chunks.embedding` has no fixed vector type. Add an ANN index when the corpus grows.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| `/api/ready` returns 503 | Postgres health, credentials, and network. |
| Setup redirects to login | An organization already exists. Use the existing owner account. |
| Widget says the site is not enabled | Add the exact origin under Settings, then re-copy the snippet. |
| Knowledge stays pending | Worker logs, crawler reachability, SSRF rejection, file size limits. |
| AI answers never arrive | Provider key, base URL, model id, worker logs, queue failures. |
| Login returns 429 | Rate limit window. Wait 15 minutes or inspect `rate_limit_windows`. |
| SSE disconnects every 10 minutes | Expected. EventSource reconnects automatically. |

## Security checklist

- [ ] `APP_SECRET` is random and stored in a secret manager or protected `.env`
- [ ] `APP_URL` is HTTPS
- [ ] `APP_ENV=production`
- [ ] `CHASTESK_INGEST_ALLOW_PRIVATE` is unset
- [ ] Postgres is not exposed to the public network
- [ ] Backups are encrypted and tested
- [ ] Reverse proxy disables buffering for SSE
- [ ] Operator accounts use unique passwords
- [ ] Audit export is reviewed after role changes
