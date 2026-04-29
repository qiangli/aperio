.PHONY: build test clean install

BINARY := aperio
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build:
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/aperio

test:
	go test ./... -v

test-short:
	go test ./... -short

clean:
	rm -rf bin/

install:
	go install $(LDFLAGS) ./cmd/aperio

lint:
	go vet ./...
