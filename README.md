# on-context

A tiny sticky-note board over HTTP. It keeps a small JSON array on disk, and clients push or pull named notes. It's built for things like a phone automation pushing state that another service polls.

Uses only the Go standard library.

## Note shape

```json
{
  "noteName": "weather",
  "noteValue": "{\"WeatherCondition\":\"Windy\",\"FeelsLike\":\"15°C\"}",
  "updated": "2027-01-01T12:00:00Z",
  "persistent": false
}
```

- `noteValue` is an opaque string. The server doesn't parse it.
- `updated` is set by the server, from the system clock (UTC), on every write.
- Before each request, any note with `persistent: false` and `now - updated > ttl` is deleted.

## API

Every `/notes` route needs `Authorization: Bearer <token>` or `X-Api-Key: <token>`.

| Method | Path | Behaviour |
|---|---|---|
| `GET` | `/notes` | List all notes |
| `GET` | `/notes/{name}` | Get one note (404 if missing) |
| `POST` | `/notes` | Upsert. Returns `201` if created, `200` if updated |
| `DELETE` | `/notes/{name}` | Delete (`204`, or 404 if missing) |
| `GET` | `/healthz` | Health check, no auth |

Upsert body: `{"noteName": "...", "noteValue": "...", "persistent": true}`

- `noteValue` is required when creating a note. If you leave it out on an update, the old value is kept.
- `persistent` is optional. A new note defaults to `false`. If you leave it out on an update, the old value is kept.

```sh
curl -X POST localhost:8080/notes -H "X-Api-Key: $TOKEN" \
  -d '{"noteName":"weather","noteValue":"{\"FeelsLike\":\"15°C\"}"}'
```

## Configuration

Settings are applied in this order, each one overriding the one before: defaults, then `config.json`, then the `.env` file, then process environment variables.

| config.json | env | default |
|---|---|---|
| `listen` | `ONCONTEXT_LISTEN` | `:8080` |
| `dataFile` | `ONCONTEXT_DATA` | `notes.json` |
| `token` | `ONCONTEXT_TOKEN` | — (required) |
| `ttl` | `ONCONTEXT_TTL` | `2h` |

Use `-config path` or `ONCONTEXT_CONFIG` to point to a config file somewhere else.

## Run

```sh
go run .                       # local
cp .env.example .env           # set ONCONTEXT_TOKEN
mkdir -p data && sudo chown 65532:65532 data
docker compose up -d --build
```

The image runs as distroless `nonroot` (uid 65532), so the `./data` folder you mount must be writable by that user.

## License

MIT
