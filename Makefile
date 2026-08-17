.PHONY: all build run dev docker clean fmt hooks test test-race test-integration test-docker lint db openapi openapi-check

all: build

build:
	CGO_ENABLED=0 go build -o boreas ./cmd/boreas
	@echo "Boreas built successfully"

run: build
	./boreas

dev:
	go run ./cmd/boreas

db:
	docker compose up -d boreas-db

docker:
	docker build -t boreas:latest .

clean:
	rm -f boreas

fmt:
	golangci-lint fmt

hooks:
	lefthook install

openapi:
	go run ./cmd/openapi > api/openapi.yaml
	@echo "api/openapi.yaml updated"

openapi-check:
	@go run ./cmd/openapi | diff -u api/openapi.yaml - \
		|| (echo "api/openapi.yaml is stale; run 'make openapi'" && exit 1)

test:
	go test ./...

test-docker:
	BOREAS_TEST_DOCKER=1 go test ./internal/infra/docker/... -count=1

test-integration:
	BOREAS_TEST_DATABASE_URL=$${BOREAS_TEST_DATABASE_URL:-postgres://postgres:postgres@localhost:5432/boreas?sslmode=disable} \
		go test ./internal/infra/postgres/... -count=1

test-race:
	CGO_ENABLED=1 go test -race ./...

lint:
	golangci-lint run ./...
