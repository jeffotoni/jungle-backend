.PHONY: deps tidy test race lint up down logs ps api consumer publisher

deps:
	go get github.com/jeffotoni/quick
	go get github.com/jeffotoni/log
	go get go.uber.org/fx
	go get github.com/jackc/pgx/v5/pgxpool

tidy:
	go mod tidy

test:
	go test ./...

race:
	go test -race ./...

lint:
	@formatted_files="$$(gofmt -l .)"; if [ -n "$$formatted_files" ]; then echo "The following files are not formatted:"; echo "$$formatted_files"; exit 1; fi
	go vet ./...
	golangci-lint run ./...
	gosec ./...
	govulncheck ./...
	go test ./...
	go test -race ./...

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

ps:
	docker compose ps

api:
	go run ./cmd/api

consumer:
	go run ./cmd/consumer

publisher:
	go run ./cmd/publisher
