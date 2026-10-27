# Go URL Shortener

[![ci](https://github.com/Lekopien/go-url-shortener/actions/workflows/ci.yml/badge.svg)](https://github.com/Lekopien/go-url-shortener/actions/workflows/ci.yml)

**Project page: https://short.lekopien.com**

A small, fast URL-shortening microservice written in **Go using only the standard library**. It uses Go 1.22's method-aware router, cryptographically random base62 codes, a mutex-guarded store with atomic JSON persistence, click counting, link expiry, custom aliases, structured request logging and graceful shutdown. It ships as a ~10 MB distroless Docker image.

## API

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/api/links` | Body `{"url": "...", "alias": "optional", "ttlSeconds": 0}` → `201` with the short link |
| `GET` | `/{code}` | `302` redirect (counts a click), `404` unknown, `410` expired |
| `GET` | `/api/links/{code}` | Stats: URL, clicks, created and expiry time |
| `DELETE` | `/api/links/{code}` | Remove a link |
| `GET` | `/healthz` | Liveness probe |
| `GET` | `/` | Minimal web form |

## Run it

```bash
go run ./cmd/server                     # http://localhost:8080
go test -race ./...                     # unit + HTTP tests, race detector on
go test -bench . ./internal/shortener   # benchmark code generation
```

```bash
curl -s localhost:8080/api/links -d '{"url":"https://lekopien.com","alias":"home"}'
# {"code":"home","url":"https://lekopien.com","clicks":0,...,"shortUrl":"http://localhost:8080/home"}

curl -i localhost:8080/home    # HTTP/1.1 302 Found, Location: https://lekopien.com
```

Flags (or environment variables): `-addr` (`ADDR`), `-db` (`DB_FILE`, empty for memory only), `-base` (`BASE_URL`).

## Docker

```bash
docker build -t go-url-shortener .
docker run -p 8080:8080 -v $PWD/data:/data go-url-shortener
```

## Design notes

- **Safety:** only absolute `http(s)` URLs are accepted, so `javascript:` links can't be stored. Request bodies are capped at 8 KB. Aliases are validated and route names are reserved.
- **Concurrency:** a `sync.RWMutex` guards the map; the test suite fires 200 concurrent redirects and checks every click is counted under `-race`.
- **Persistence:** writes go to a temp file then `rename`, so a crash never leaves a half-written database.
- **Testability:** the store's clock is injectable, so expiry is tested without sleeping.

```
cmd/server/            entry point, flags, graceful shutdown
internal/shortener/    store.go (data), handler.go (HTTP), shortener_test.go
site/                  project page served at short.lekopien.com
```

## License

MIT
