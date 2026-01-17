.PHONY: build run test clean lint fmt

build:
	go build -o bin/ci-dashboard ./cmd/ci-dashboard

run:
	go run ./cmd/ci-dashboard

test:
	go test -v ./...

test-coverage:
	go test -cover ./...

clean:
	rm -rf bin/

lint:
	go vet ./...

fmt:
	go fmt ./...

.DEFAULT_GOAL := build
