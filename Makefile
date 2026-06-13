.PHONY: help build clean test install run lint fmt vet all

# ─────────────────────────────────────────────────────────────────────────────
# Variables
# ─────────────────────────────────────────────────────────────────────────────

BINARY_NAME := cfr
BUILD_DIR := build
GO := go
GOFLAGS := -v
LDFLAGS := 
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS += -X main.Version=$(VERSION)

# ─────────────────────────────────────────────────────────────────────────────
# Targets
# ─────────────────────────────────────────────────────────────────────────────

## help: Show this help message
help:
	@echo "Makefile targets:"
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## all: Build, test, and lint
all: clean fmt vet test build

## build: Build the binary and store in build/ directory
build:
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) .

## test: Run tests
test:
	$(GO) test $(GOFLAGS) -race -coverprofile=coverage.out ./...

## coverage: Show test coverage report
coverage: test
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## lint: Run linter (requires golangci-lint)
lint:
	@command -v golangci-lint >/dev/null 2>&1 || (echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest" && exit 1)
	golangci-lint run ./...

## fmt: Format code with gofmt
fmt:
	$(GO) fmt ./...
	@echo "Code formatted"

## vet: Run go vet
vet:
	$(GO) vet ./...
	@echo "Vet check passed"

## clean: Remove build directory and generated files
clean:
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "Cleaned build artifacts"

## install: Build and install binary to GOPATH/bin
install: build
	@mkdir -p $(GOPATH)/bin
	@cp $(BUILD_DIR)/$(BINARY_NAME) $(GOPATH)/bin/
	@echo "Installed $(BINARY_NAME) to $(GOPATH)/bin"

## run: Build and run the binary
run: build
	@./$(BUILD_DIR)/$(BINARY_NAME)

## deps: Download and verify dependencies
deps:
	$(GO) mod download
	$(GO) mod verify
	@echo "Dependencies up to date"

## tidy: Tidy go.mod and go.sum
tidy:
	$(GO) mod tidy
	@echo "Dependencies tidied"

.DEFAULT_GOAL := help
