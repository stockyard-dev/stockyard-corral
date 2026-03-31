# Stockyard Corral

Webhook relay and debugger. Receive, log, replay, and forward webhooks. Self-hosted alternative to RequestBin, Hookdeck, and Webhook.site.

## What it does

Corral gives you a URL that receives webhooks. Every payload is logged with headers, body, timing, and source IP. You can inspect them in real time, replay them to your application, and set up forwarding rules.

## Features

- **Receive and log** — unique endpoint URLs, full payload capture (headers + body + timing)
- **Live stream** — SSE real-time feed of incoming webhooks
- **Replay** — re-send any captured webhook to a target URL with one click
- **Forward** — route incoming webhooks to multiple destinations with retry
- **Filter** — match webhooks by header, body content, or source
- **Inspect** — full request details: headers, body, IP, timing, response
- **Single binary** — Go + embedded SQLite, no external dependencies
- **Self-hosted** — webhook data never leaves your infrastructure

## Quick start

```bash
curl -fsSL https://stockyard.dev/corral/install.sh | sh
corral serve
```

## API

```bash
# Create an endpoint
curl -X POST localhost:8760/api/endpoints \
  -d '{"name":"stripe-test"}'
# Returns: https://your-server:8760/hook/ep_a8f21c4e

# List captured webhooks
curl localhost:8760/api/endpoints/ep_a8f21c4e/events

# Replay a webhook to your app
curl -X POST localhost:8760/api/events/evt_123/replay \
  -d '{"target":"http://localhost:3000/webhooks/stripe"}'
```

## Pricing

- **Free:** 1,000 events/month, 1 endpoint, 24h retention
- **Pro ($9/mo):** Unlimited events, unlimited endpoints, 30d retention, forwarding rules, replay

## Part of Stockyard

Corral reuses the webhook infrastructure from [Stockyard](https://stockyard.dev), the self-hosted LLM infrastructure platform.

## License

Apache 2.0
