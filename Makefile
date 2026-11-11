.PHONY: build test testacc testenv-up testenv-down

build:
	go build ./...

test:
	go test ./internal/glpi/...

testenv-up:
	testenv/bootstrap.sh

testenv-down:
	docker compose -f testenv/docker-compose.yml down -v

# Usage: eval "$$(testenv/bootstrap.sh)" && make testacc
testacc:
	go test ./internal/provider/... -v -timeout 15m
