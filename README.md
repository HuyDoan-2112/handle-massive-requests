# handle-massive-requests

## What is this

## Status

## Project layout

```
cmd/api/            entrypoint (main.go): starts the gRPC server
proto/ticket/v1/    gRPC API contract (ticket.proto)
gen/                generated Go code from proto/ - do not edit, run `make proto`
internal/grpcapi/   gRPC TicketService implementation (and its tests)
internal/ticket/    ticket domain model + Postgres repository
migrations/         SQL run on first start of the postgres container
deploy/             docker-compose for local infra
```

Ticket requests follow `gRPC handler → ticket.Service → ticket.Repository → PostgreSQL`.

- `internal/ticket/ticket.go`: ticket types, validation tags, domain errors, and allowed status transitions.
- `internal/ticket/service.go`: application operations and the `Store` interface they require. The service validates input using the existing `go-playground/validator` dependency.
- `internal/ticket/repository.go`: parameterized SQL and database error handling. Status changes check and update the current status in one SQL statement.
- `internal/ticket/service_test.go`: service validation and storage error tests, without PostgreSQL.
- `internal/ticket/repository_test.go`: PostgreSQL integration tests, including competing reservations. Each test creates and removes its own schema.
- `internal/db/db_test.go`: pool error handling tests and a PostgreSQL integration test for connecting, querying, and closing the pool.

The gRPC handlers keep protobuf conversion, UUID parsing, page tokens, and gRPC error mapping. `cmd/api/main.go` connects the repository to the service and the service to the handlers.

## How to run

```bash
make tools  # first time: installs buf, protoc plugins, grpcurl
make tidy   # first time: downloads dependencies, writes go.sum
make up     # start local infra (docker compose)
make run    # gRPC on :9090 (tickets and health checks)
make test
make proto  # after editing proto/: lint + regenerate gen/
```

Database integration tests are skipped unless `TEST_DATABASE_URL` is set. With local PostgreSQL running, run all tests using:

```bash
TEST_DATABASE_URL='postgres://app:app@localhost:5433/train?sslmode=disable' go test ./...
```

The test database user needs permission to create schemas. The tests apply the existing migration inside temporary schemas and leave application tables untouched.

`.env` (not committed):

```
POSTGRES_URL=postgres://app:app@localhost:5433/train
GRPC_PORT=9090
```

Try the API:

```bash
grpcurl -plaintext localhost:9090 list
grpcurl -plaintext -d '{}' localhost:9090 grpc.health.v1.Health/Check
grpcurl -plaintext -d '{"name":"SE1","departure_location":"Hanoi","destination":"Saigon","price_cents":125000,"ticket_type":"TICKET_TYPE_ECONOMY","time_start":"2026-12-01T08:00:00Z","time_end":"2026-12-02T14:00:00Z"}' localhost:9090 ticket.v1.TicketService/CreateTicket
grpcurl -plaintext -d '{"departure_location":"Hanoi","destination":"Saigon"}' localhost:9090 ticket.v1.TicketService/ListTickets
```

## Team
