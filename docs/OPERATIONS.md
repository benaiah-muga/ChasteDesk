# Operations runbook

This is the practical guide for running ChasteDesk. It assumes the Compose deployment in `docker-compose.yml`.

## Daily checks

```bash
docker compose ps
curl -fsS http://localhost:3000/api/health
curl -fsS http://localhost:3000/api/ready
docker compose logs --since 24h web worker | grep -E '"level":"(ERROR|WARN)"' || true
```

Open **Analytics** and **Approvals** once a day. The overview page lists failed ingests, pending approvals, and conversations waiting for a human.

## Start and stop

```bash
docker compose up -d --wait
docker compose stop
docker compose down                 # keeps volumes
docker compose down -v              # deletes database and uploads, use carefully
```

## Migrations

The `migrate` service runs before web and worker. It uses the same image as web and exits after applying `internal/db/migrations`.

```bash
docker compose run --rm migrate
```

The web binary also calls `db.Migrate` on boot. Migrations are recorded in `schema_migrations`. Never edit an applied migration. Add a new one.

## Backups

```bash
mkdir -p backups
docker compose exec -T postgres pg_dump -U chastedesk -Fc chastedesk > backups/chastedesk-$(date +%F).dump
docker run --rm -v chastedesk_uploads:/data -v "$PWD/backups":/backup alpine \
  tar czf /backup/uploads-$(date +%F).tgz -C /data .
```

Restore:

```bash
docker compose stop web worker
docker compose exec -T postgres pg_restore -U chastedesk -d chastedesk --clean --if-exists < backups/chastedesk.dump
docker run --rm -v chastedesk_uploads:/data -v "$PWD/backups":/backup alpine \
  tar xzf /backup/uploads.tgz -C /data
docker compose start web worker
```

## Queue stuck or backing up

Inspect jobs:

```sql
select name, status, count(*)
from jobs
group by name, status
order by name, status;
```

Find recent failures:

```sql
select id, name, status, retry_count, retry_limit, last_error, run_at
from jobs
where status = 'failed'
order by completed_at desc
limit 20;
```

Retry a specific job:

```sql
update jobs set status = 'created', run_at = now(), retry_count = 0 where id = 'JOB_ID';
```

If a queue is not moving, check that the worker is running and can reach Postgres and the provider endpoint.

## Provider failure

Symptoms: AI runs fail, conversations remain `ai_active`, logs contain provider HTTP errors.

1. Open **Settings → AI providers** and press **Test connection**.
2. Check the base URL and model id in the support agent policy.
3. For Ollama, confirm the worker can reach the Ollama host. `localhost` inside a container is the container, not the host.
4. If the provider is down, remove or disable it. The desk then escalates every new conversation to a human.

## Knowledge ingest failure

1. Open **Knowledge** and read the error under the failed source.
2. Common causes: DNS failure, 404, a private IP, a content type outside the allowlist, or a redirect loop.
3. Fix the URL or file, then press the reindex icon.
4. If a page requires a login, download it and upload the file instead.

## Approval queue behavior

Approvals are safe to leave waiting. A pending action means the AI stopped before execution. Approving runs the tool once, then verifies the result. Rejecting marks the action `rejected_before_exec` and the run remains escalated.

If an approval is approved but the verification fails, the conversation is escalated with a tool failure reason. Do not send a success message until the verification issue is resolved.

## Rate limit lockouts

Login is limited to 10 attempts per 15 minutes per IP and email. Inspect and clear a window only when you are sure it is a false positive:

```sql
select * from rate_limit_windows where key like 'auth:%';
delete from rate_limit_windows where key = 'auth:IP:email@example.com';
```

Do not disable rate limits globally.

## Secrets and key rotation

- Rotating `APP_SECRET` invalidates provider keys and widget tokens.
- Re-enter every provider key after rotation.
- Re-copy the widget snippet.
- Existing sessions remain valid because session tokens are hashed, not derived from the secret.

Procedure:

1. Set the new `APP_SECRET`.
2. Restart web and worker.
3. Re-enter provider keys.
4. Re-copy the widget snippet.
5. Confirm analytics, ingestion, and AI replies.

## Audit access

Use **Audit log** filters or export CSV. The audit table rejects updates and deletes at the database level. If an audit row must be removed, that is a deliberate database-owner operation, not an application feature.

## Incident checklist

- [ ] Identify the affected surface: dashboard, widget, REST, ingestion, or provider.
- [ ] Check `/api/ready`, web logs, worker logs, and Postgres health.
- [ ] Rotate `APP_SECRET` if you suspect key exposure, then re-enter provider keys.
- [ ] Disable the widget or remove the allowed origin if abuse is coming from a site.
- [ ] Revoke API keys that may be leaked.
- [ ] Export the audit log before changing roles or deleting accounts.
- [ ] Restore from backup only after preserving the current database for forensics.
- [ ] Document the timeline and the decisions.
