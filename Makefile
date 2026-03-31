.PHONY: build test clean run

VERSION ?= dev

build:
	CGO_ENABLED=0 go build -ldflags="-s -w -X main.version=$(VERSION)" -o dist/corral ./cmd/corral
	@echo "Built dist/corral ($(VERSION))"

test:
	go test ./... -count=1 -timeout 60s

run:
	go run ./cmd/corral

clean:
	rm -rf dist/ data/

check:
	CGO_ENABLED=0 go build -o /dev/null ./cmd/corral && echo "corral: ok"
	go vet ./...
