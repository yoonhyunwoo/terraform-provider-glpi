.PHONY: build test testacc generate lint testenv-down

build:
	go build ./...

test:
	go test ./internal/glpi/...

generate:
	go generate ./...

lint:
	golangci-lint run

testenv-down:
	docker compose -f testenv/docker-compose.yml down -v

# Usage: eval "$$(testenv/bootstrap.sh)" && make testacc
testacc:
	go test ./internal/provider/... -v -timeout 15m
