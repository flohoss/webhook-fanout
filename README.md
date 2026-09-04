# webhook-fanout

A small Go service that receives a webhook, debounces it, and fans it out to a list of target webhook URLs.

## How it works

1. A webhook arrives at the listen path derived from `TARGET_TEMPLATE` — the template with its `/{id}` segment removed.
2. The payload is verified against the `X-Hub-Signature-256` HMAC header.
3. The payload is scheduled for delivery after a debounce window (default `5m`). New webhooks reset the timer.
4. When the timer fires, the payload is POSTed to every target URL concurrently, with retries on failure.

## Configuration

All configuration is via environment variables:

| Variable           | Required |    Default     | Description                                                                        |
| ------------------ | :------: | :------------: | ---------------------------------------------------------------------------------- |
| `WEBHOOK_SECRET`   |   yes    |       —        | HMAC secret for webhook signatures and health                                      |
| `IDS`              |   yes    |       —        | Comma-separated stack IDs to fan out to (sorted)                                   |
| `TARGET_TEMPLATE`  |   yes    |       —        | Absolute URL template; `{id}` segment is stripped for listening, filled per target |
| `LOG_LEVEL`        |    no    |     `info`     | `debug`, `info`, `warn`, or `error`                                                |
| `TZ`               |    no    |     `UTC`      | Timezone for log timestamps                                                        |
| `LISTEN_ADDR`      |    no    | `0.0.0.0:8080` | Address the HTTP server listens on                                                 |
| `DEBOUNCE`         |    no    |      `5m`      | Debounce window before fan-out                                                     |
| `REQUEST_TIMEOUT`  |    no    |     `15s`      | Timeout per target delivery attempt                                                |
| `MAX_BODY_BYTES`   |    no    |   `1048576`    | Max webhook body size (1 MiB)                                                      |
| `MAX_ATTEMPTS`     |    no    |      `4`       | Delivery attempts per target (incl. first)                                         |
| `RETRY_BACKOFF`    |    no    |    `500ms`     | Initial backoff between attempts (doubles)                                         |
| `SHUTDOWN_TIMEOUT` |    no    |     `10s`      | Graceful shutdown timeout                                                          |
| `RATE_LIMIT`       |    no    |      `10`      | Requests per second per IP                                                         |
| `MAX_CONCURRENT`   |    no    |      `8`       | Max concurrent target deliveries                                                   |

Durations use Go syntax (`500ms`, `15s`, `5m`).

## Endpoints

### `POST <listen path>`

The listen path is `TARGET_TEMPLATE`'s URL path with the `/{id}` segment removed:

| `TARGET_TEMPLATE`                                       | Listens on                     |
| ------------------------------------------------------- | ------------------------------ |
| `https://hooks.example.com/api/git/stacks/{id}/webhook` | `POST /api/git/stacks/webhook` |
| `https://hooks.example.com/webhook/{id}`                | `POST /webhook`                |

Accepts the webhook payload. Requires a valid `X-Hub-Signature-256` header (GitHub-style HMAC-SHA256 of the body using `WEBHOOK_SECRET`).

Returns `202 Accepted` with the scheduled deploy time.

### `GET /health`

Health and status endpoint. Requires the secret as a bearer token:

```bash
curl -H "Authorization: Bearer $WEBHOOK_SECRET" http://localhost:8080/health
```

Returns the service status, the remote IP, the list of target URLs, and the next deploy time (`null` when no deploy is pending).

## Examples

With `TARGET_TEMPLATE=https://dockhand.example.com/api/git/stacks/{id}/webhook` and `IDS=42,43`:

- Listens on `POST /api/git/stacks/webhook`
- Delivers to `https://dockhand.example.com/api/git/stacks/42/webhook` and `https://dockhand.example.com/api/git/stacks/43/webhook`

With `TARGET_TEMPLATE=https://dockhand.example.com/webhook/{id}` and `IDS=a,b`:

- Listens on `POST /webhook`
- Delivers to `https://dockhand.example.com/webhook/a` and `https://dockhand.example.com/webhook/b`

The derived listen path is logged at startup ("listen" attribute).

## Development

```bash
cp .env.example .env
docker compose up --build
```

The dev service uses [air](https://github.com/air-verse/air) for hot reload.

## Release

```bash
docker compose --profile build build release
```

The release image is a scratch-based, non-root binary. The GitHub Actions workflow builds and pushes the image to GHCR as `:latest` on every push to the default branch.
