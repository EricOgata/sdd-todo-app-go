GO       ?= go
BINARY   := todo
BIN_DIR  := bin
VERSION  ?= $(shell git describe --tags --abbrev=0 2>/dev/null || echo dev)
LDFLAGS  := -X todo/internal/buildinfo.Version=$(VERSION)

SOURCES := go.mod go.sum \
	$(shell find cmd internal test -type f \( -name '*.go' -o -name '*.sql' \) 2>/dev/null)

.PHONY: all build test test-race cover lint fmt install clean

all: build

build: $(BIN_DIR)/$(BINARY)

$(BIN_DIR)/$(BINARY): $(SOURCES)
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $@ ./cmd/todo

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

cover:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html

lint:
	$(GO) vet ./...

fmt:
	gofmt -l -w cmd internal test

install:
	$(GO) install -trimpath -ldflags "$(LDFLAGS)" ./cmd/todo

clean:
	rm -rf $(BIN_DIR) coverage.out coverage.html
