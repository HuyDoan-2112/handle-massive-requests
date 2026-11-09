.PHONY: run test fmt tidy up down reset db-tables

run:
	go run ./cmd/api

test:
	go test ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

up:
	docker compose -f deploy/docker-compose.yml up -d

down:
	docker compose -f deploy/docker-compose.yml down

# Wipe the DB volume and start fresh so migrations/*.sql run again.
reset:
	docker compose -f deploy/docker-compose.yml down -v
	docker compose -f deploy/docker-compose.yml up -d

# List tables in the running database.
db-tables:
	docker exec -it postgres psql -U app -d train -c '\dt'
