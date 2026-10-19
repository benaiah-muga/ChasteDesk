# ChasteDesk

AI-native, self-hostable customer support. AGPL-3.0-only.

## Quickstart

1. Copy `.env.example` to `.env` and set `APP_SECRET` (`openssl rand -base64 48`).
2. `docker compose up -d --build --wait` — starts Postgres (pgvector), runs migrations, launches web + worker.
3. Open http://localhost:3000 — the first-run wizard creates your organization, owner account, and AI provider.
4. Add a knowledge source (your help docs URL), then embed the chat widget:

```html
<script src="http://localhost:3000/widget.js" data-org="YOUR_ORG_ID" data-token="YOUR_WIDGET_TOKEN" defer></script>
```

Your widget token and embed snippet are in **Dashboard → Settings → Chat widget**.

## AI providers

Works with OpenAI, Anthropic, Google AI, local Ollama, or any OpenAI-compatible endpoint. Without any provider the AI routes every conversation to a human (the desk still works).

With Ollama locally: start Ollama, then in Settings add provider "Ollama (local)" — no key needed.

## Development

See AGENTS.md for commands and conventions.
