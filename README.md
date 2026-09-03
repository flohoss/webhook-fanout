# webhook-fanout

A small Go service that receives a webhook, debounces it, and fans it out to a list of target webhook URLs.

## How it works

1. A webhook arrives at `POST /api/git/stacks/webhook`.
2. The payload is verified against the `X-Hub-Signature-256` HMAC header.
3. The payload is scheduled for delivery after a debounce window (default `5m`). New webhooks reset the timer.
4. When the timer fires, the payload is POSTed to every target URL concurrently, with retries on failure.

## Configuration

All configuration is via environment variables:

| Variable         | Required |  Default  | Description                                   |
| ---------------- | :------: | :-------: | --------------------------------------------- |
| `WEBHOOK_SECRET` |   yes    |     —     | HMAC secret for webhook signatures and health |
| `STACK_IDS`      |   yes    |     —     | Comma-separated stack IDs to fan out to       |
| `TARGET_BASE`    |   yes    |     —     | Base URL, targets are `<base>/<id>/webhook`   |
| `LOG_LEVEL`      |    no    |  `info`   | `debug`, `info`, `warn`, or `error`           |
| `TZ`             |    no    |   `UTC`   | Timezone for log timestamps                   |
| `APP_VERSION`    |    no    |   `dev`   | Version string reported by `/health`          |
| `BUILD_TIME`     |    no    | `unknown` | Build timestamp reported by `/health`         |
| `REPO_URL`       |    no    |     —     | Repository URL reported by `/health`          |

See `.env.example` for a template.

## Endpoints

### `POST /api/git/stacks/webhook`

Accepts the webhook payload. Requires a valid `X-Hub-Signature-256` header (GitHub-style HMAC-SHA256 of the body using `WEBHOOK_SECRET`).

Returns `202 Accepted` with the scheduled deploy time.

### `GET /health`

Health and status endpoint. Requires the secret as a bearer token:

```bash
curl -H "Authorization: Bearer $WEBHOOK_SECRET" http://localhost:8080/health
```

Returns the fan-out target URLs, whether a deploy is pending, and the next deploy time.

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

The release image is a scratch-based, non-root binary. The GitHub Actions workflow builds and pushes the image to GHCR as `:latest` on every push to the default branch. The build args `APP_VERSION`, `BUILD_TIME`, and `REPO_URL` are baked into the image as environment variables and reported by the `/health` endpoint.
