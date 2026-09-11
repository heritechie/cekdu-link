# cekdu-link

A standalone, developer-friendly programmable short URL service.

`cekdu-link` provides short URLs that redirect to destination URLs with basic click tracking:

```text
Short Link → Redirect → Click Count
```

The service is **domain-agnostic**: its model contains no CekDulu-specific concepts such as campaigns, partners, users, or organizations. `cekdulu.co.id` is one consumer, but the same service can be used by any application.

## Current capabilities

```text
GET    /health

POST   /api/links
GET    /api/links
GET    /api/links/{id}
PATCH  /api/links/{id}

GET    /{code}
```

## Prerequisites

* Go 1.24 or newer
* Docker with Docker Compose

## Start development environment (Docker Compose)

```bash
cp .env.example .env
docker compose up --build -d
```

Runs `app` (Go HTTP server) and `postgres` as Docker services. Host ports are configurable via `.env` — defaults are `18080` for the app and `15432` for PostgreSQL.

Health check:

```bash
curl http://localhost:18080/health
```

Expected response:

```json
{"status":"ok"}
```

## Run the server (local, without Docker)

```bash
HTTP_ADDR=:8080 go run ./cmd/server
```

`HTTP_ADDR` defaults to `:8080` when not set.

## Database migrations

Migrations are SQL files under `migrations/`, applied with `goose` via `cmd/migrate`. **Migrations are not executed automatically when the application starts** — run them explicitly.

### Locally (from the project root)

```bash
go run ./cmd/migrate up
go run ./cmd/migrate down
go run ./cmd/migrate status
```

or using `make` (`migrate-up`, `migrate-down`, `migrate-status`).

### In Docker

```bash
docker compose exec app /app/migrate up
docker compose exec app /app/migrate down
docker compose exec app /app/migrate status
```

`DB_HOST`/`DB_PORT`/`DB_NAME`/`DB_USER`/`DB_PASSWORD` are read from the environment (`postgres:5432` inside the compose network).

## Create a short link

```http
POST /api/links
Content-Type: application/json
```

Request:

```json
{
  "destination_url": "https://example.com/some-page"
}
```

The `code` field is optional. When omitted, a random 8-character code is generated.

```json
{
  "destination_url": "https://example.com/some-page",
  "code": "promo-motor"
}
```

Successful response (`201 Created`):

```json
{
  "id": "3f2a9b8c-1d4e-4a5b-9c6d-7e8f9a0b1c2d",
  "code": "promo-motor",
  "short_url": "http://localhost:18080/promo-motor",
  "destination_url": "https://example.com/some-page",
  "status": "active",
  "click_count": 0,
  "created_at": "2026-09-12T00:00:00Z",
  "updated_at": "2026-09-12T00:00:00Z"
}
```

Custom codes must match `^[A-Za-z0-9_-]{3,64}$` and cannot be reserved (`api`, `health`, `favicon.ico`). Codes are case-sensitive — `abc123` and `ABC123` are distinct.

### `PUBLIC_BASE_URL`

Controls the `short_url` returned by the API (the public URL of the service, not `HTTP_ADDR`):

```bash
PUBLIC_BASE_URL=http://localhost:18080   # local development
PUBLIC_BASE_URL=https://cekdu.lu          # production
```

Trailing slashes are handled automatically.

## List links

```http
GET /api/links
```

Returns all links, newest first:

```json
[
  {
    "id": "765563fa-0ceb-4148-a615-0e10a29ec8e4",
    "code": "example-test",
    "short_url": "http://localhost:18080/example-test",
    "destination_url": "https://example.com",
    "status": "active",
    "click_count": 12,
    "created_at": "2026-09-11T18:01:10.507423Z",
    "updated_at": "2026-09-11T18:01:10.507423Z"
  }
]
```

No pagination, filtering, or sorting parameters in the MVP.

## Get one link

```http
GET /api/links/{id}
```

* `200` with the link body above
* `400` when `{id}` is not a valid UUID (no database query is made)
* `404` for an unknown id

## Update a link

```http
PATCH /api/links/{id}
Content-Type: application/json
```

Both fields are optional, but at least one recognized field must be supplied:

```json
{
  "destination_url": "https://example.org/new"
}
```

```json
{
  "status": "inactive"
}
```

* `destination_url` follows the same rules as Create Link (`http`/`https`, valid host).
* `status` accepts only `active` or `inactive`.
* `200` returns the updated link; `400`/`404` match the other endpoints.
* Immutable fields (`id`, `code`, `click_count`, `created_at`) are rejected as unknown fields.
* `updated_at` is refreshed only when an actual change is made.
* `DELETE` is not implemented.

## Redirect

```http
GET /{code}
```

Looks up the link, atomically increments its `click_count`, and issues a `302 Found` redirect to the destination URL.

```bash
curl -i http://localhost:18080/example-test
```

* Active link: `302` with a `Location` header pointing to the destination URL.
* Unknown or inactive link: `404` with `{"error":"link not found"}`.
* Non-`GET` methods: `405` with `Allow: GET`.
* Redirects are always `302`, never `301`, so destinations can change freely.

## Link lifecycle

```text
active            → redirect works
inactive          → redirect returns 404
inactive → active → redirect works again
```

`click_count` is never reset when a link is deactivated or reactivated.

## API behavior

* Custom code rules: `^[A-Za-z0-9_-]{3,64}$`, reserved codes rejected.
* Generated code: 8 random characters (URL-safe alphabet, no ambiguous `0 O 1 l I`).
* Destination URL validation: only `http`/`https`, must contain a valid host. No network/DNS checks.
* Status values: `active`, `inactive`.
* Duplicate code on create → `409`.
* Invalid input (bad URL, bad status, unknown field, malformed JSON, invalid UUID) → `400`.
* Unknown link → `404`.
* Unsupported method → `405`.

## Stop

```bash
docker compose down
```

Remove local database data too:

```bash
docker compose down -v
```

This removes the named PostgreSQL volume `postgres_data` and permanently deletes all local database data.