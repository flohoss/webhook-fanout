# webhook-fanout

A small Go service that receives a webhook, debounces it, and fans it out to every Dockhand stack that has webhooks enabled. It is tailored to Dockhand's Git webhook API — the target list is looked up at fire time, so there is no static ID list to maintain.

## How it works

1. A webhook arrives at the fixed listen path `POST /api/git/stacks/webhook`.
2. The payload is verified against the `X-Hub-Signature-256` HMAC header using `FANOUT_SECRET`.
3. The payload is scheduled for delivery after a debounce window (default `5m`). New webhooks reset the timer.
4. When the timer fires, the service calls `GET {DOCKHAND_URL}/api/git/stacks` and keeps entries where `webhookEnabled=true`.
5. The payload is POSTed concurrently to `{DOCKHAND_URL}/api/git/stacks/<id>/webhook` for each enabled stack, signed with that stack's own `webhookSecret`. Failures are retried.

## Configuration

All configuration is via environment variables:

| Variable             | Required |    Default     | Description                                                                  |
| -------------------- | :------: | :------------: | ---------------------------------------------------------------------------- |
| `FANOUT_SECRET`      |   yes    |       —        | HMAC secret used to verify incoming webhooks                                 |
| `TRUSTED_PROXY`      |   yes    |       —        | Trusted proxy IP or CIDR whose X-Forwarded-For header is honored             |
| `DOCKHAND_URL`       |   yes    |       —        | Base URL of the Dockhand API, e.g. `http://100.99.100.64:3000`               |
| `DOCKHAND_API_TOKEN` |   yes    |       —        | Bearer token sent when calling `GET {DOCKHAND_URL}/api/git/stacks`           |
| `ACCEPT_REGEX`       |    no    |       —        | Only fan out when the payload matches this regex (empty = accept everything) |
| `LOG_LEVEL`          |    no    |     `info`     | `debug`, `info`, `warn`, or `error`                                          |
| `TZ`                 |    no    |     `UTC`      | Timezone for log timestamps                                                  |
| `LISTEN_ADDR`        |    no    | `0.0.0.0:8080` | Address the HTTP server listens on                                           |
| `DEBOUNCE`           |    no    |      `5m`      | Debounce window before fan-out                                               |
| `REQUEST_TIMEOUT`    |    no    |     `30s`      | Timeout per target delivery attempt                                          |
| `MAX_BODY_BYTES`     |    no    |   `1048576`    | Max webhook body size (1 MiB)                                                |
| `MAX_ATTEMPTS`       |    no    |      `4`       | Delivery attempts per target (incl. first)                                   |
| `RETRY_BACKOFF`      |    no    |    `500ms`     | Initial backoff between attempts (doubles)                                   |
| `SHUTDOWN_TIMEOUT`   |    no    |     `10s`      | Graceful shutdown timeout                                                    |
| `RATE_LIMIT`         |    no    |      `10`      | Requests per second per IP                                                   |
| `MAX_CONCURRENT`     |    no    |      `4`       | Max concurrent target deliveries                                             |

Durations use Go syntax (`500ms`, `15s`, `5m`).

## Filtering

`ACCEPT_REGEX` is an allowlist: when set, a webhook is only scheduled for fan-out if its raw body matches the regex. Non-matching payloads are acknowledged with `202` and `{"ignored": true}` but never scheduled. When unset or empty, every verified webhook is accepted.

This is useful when a provider fires multiple webhooks per change but only one should trigger a deploy. For example, GitHub sends a `push` webhook for the Renovate branch creation, the branch deletion after merge, and the merge to `main` — only the actual merge should deploy:

```bash
ACCEPT_REGEX='"head_commit":{"id":[^}]*"message":\s*"Merge pull request'
```

The regex is matched against the raw JSON body (Go RE2 syntax). Note that a squash merge whose PR title does not start with `Merge pull request` will not match; anchor on `"ref": "refs/heads/main"` instead if you use custom merge titles.

## Endpoints

### `POST /api/git/stacks/webhook`

Accepts the webhook payload. Requires a valid `X-Hub-Signature-256` header (GitHub-style HMAC-SHA256 of the body using `FANOUT_SECRET`).

Returns `202 Accepted` with the scheduled deploy time.

## Example

With `DOCKHAND_URL=http://100.99.100.64:3000`:

- Listens on `POST /api/git/stacks/webhook`
- At fire time, fetches stacks from `http://100.99.100.64:3000/api/git/stacks` and delivers to `http://100.99.100.64:3000/api/git/stacks/<id>/webhook` for every stack where `webhookEnabled=true` — e.g. `.../stacks/42/webhook` and `.../stacks/43/webhook`.
- Each delivery is signed with the target stack's own `webhookSecret`.

The listen path is fixed and logged at startup ("listen" attribute).

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
