.PHONY: build run test clean lint fmt dev install-air

build:
	go build -o bin/ci-dashboard ./cmd/ci-dashboard

run:
	go run ./cmd/ci-dashboard

dev:
	@AIR_BIN=$$(go env GOPATH)/bin/air; \
	if [ ! -f "$$AIR_BIN" ]; then \
		echo "Air is not installed. Installing..."; \
		go install github.com/air-verse/air@latest; \
	fi; \
	$$AIR_BIN

install-air:
	go install github.com/air-verse/air@latest

test:
	go test -v ./...

test-coverage:
	go test -cover ./...

clean:
	rm -rf bin/ tmp/

lint:
	go vet ./...

fmt:
	go fmt ./...

.DEFAULT_GOAL := build
