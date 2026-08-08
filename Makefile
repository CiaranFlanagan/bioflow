.PHONY: build test race lint cover clean

build:
	go build -o bin/bioflow ./cmd/bioflow

test:
	go test ./...

# Concurrency bugs in the executor are the ones most likely to be subtle, so
# the race detector runs as part of the normal loop, not just in CI.
race:
	go test -race ./...

lint:
	go vet ./...
	gofmt -l .

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

clean:
	rm -rf bin coverage.out .bioflow
