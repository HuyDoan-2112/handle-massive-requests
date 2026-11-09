# cache-platform

## What is this

## Status

## Project layout

```
cmd/api/            entrypoint (main.go): starts the HTTP server
internal/httpapi/   routes + handlers (and their tests)
internal/ticket/    ticket domain model
deploy/             docker-compose for local infra
```

## How to run

```bash
make tidy   # first time: downloads dependencies, writes go.sum
make up     # start local infra (docker compose)
make run    # API on :8080  →  curl localhost:8080/health
make test
```

## Team
