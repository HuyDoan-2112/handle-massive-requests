.PHONY: run test fmt tidy up down reset db-tables tools proto benchmark-v1

run:
	go run ./cmd/api

test:
	go test ./...

benchmark-v1:
	python3 benchmarks/v1/run.py

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

# One-time: install the proto toolchain into $(go env GOPATH)/bin.
tools:
	go install github.com/bufbuild/buf/cmd/buf@v1.47.2
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.10
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
	go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# Regenerate gen/ from proto/. Run after every .proto change.
proto:
	buf lint
	buf generate
