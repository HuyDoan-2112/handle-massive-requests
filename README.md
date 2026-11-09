# handle-massive-requests

## What is this

## Status

## Project layout

```
cmd/api/            entrypoint (main.go): starts the HTTP and gRPC servers
proto/ticket/v1/    gRPC API contract (ticket.proto)
gen/                generated Go code from proto/ - do not edit, run `make proto`
internal/grpcapi/   gRPC TicketService implementation (and its tests)
internal/httpapi/   HTTP routes (health check)
internal/ticket/    ticket domain model + Postgres repository
migrations/         SQL run on first start of the postgres container
deploy/             docker-compose for local infra
```

## How to run

```bash
make tools  # first time: installs buf, protoc plugins, grpcurl
make tidy   # first time: downloads dependencies, writes go.sum
make up     # start local infra (docker compose)
make run    # HTTP on :8080 (health), gRPC on :9090 (tickets)
make test
make proto  # after editing proto/: lint + regenerate gen/
```

`.env` (not committed):

```
POSTGRES_URL=postgres://app:app@localhost:5433/train
PORT=8080
GRPC_PORT=9090
```

Try the API:

```bash
grpcurl -plaintext localhost:9090 list
grpcurl -plaintext -d '{"name":"SE1","departure_location":"Hanoi","destination":"Saigon","price_cents":125000,"ticket_type":"TICKET_TYPE_ECONOMY","time_start":"2026-12-01T08:00:00Z","time_end":"2026-12-02T14:00:00Z"}' localhost:9090 ticket.v1.TicketService/CreateTicket
grpcurl -plaintext -d '{"departure_location":"Hanoi","destination":"Saigon"}' localhost:9090 ticket.v1.TicketService/ListTickets
```

## Team
