.PHONY: run test fmt tidy up down

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
