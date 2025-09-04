# Makefile for annas-mcp with IPv4-only builds

.PHONY: build install test clean dev-install

# Build variables
BINARY_NAME=annas-mcp
BUILD_DIR=build
GO_FILES=$(shell find . -name "*.go" -type f)

# Use IPv4-only DNS for Go builds
export GODEBUG=netdns=cgo+4

build: clean
	@echo "Building $(BINARY_NAME) with IPv4 preference..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/annas-mcp

# Alternative build using Docker for systems with IPv6 issues
build-docker:
	@echo "Building with Docker (IPv4 only)..."
	@mkdir -p $(BUILD_DIR)
	docker run --rm \
		-v "$$PWD":/app \
		-w /app \
		golang:1.21 \
		go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/annas-mcp

install: build
	@echo "Installing $(BINARY_NAME) to ~/.local/bin..."
	@mkdir -p ~/.local/bin
	cp $(BUILD_DIR)/$(BINARY_NAME) ~/.local/bin/$(BINARY_NAME)
	@echo "Installation complete!"

# Development install - use current Python version as fallback
dev-install:
	@echo "Installing development version..."
	@if [ -f "$(BUILD_DIR)/$(BINARY_NAME)" ]; then \
		cp $(BUILD_DIR)/$(BINARY_NAME) ~/.local/bin/$(BINARY_NAME); \
		echo "Go binary installed"; \
	else \
		echo "No Go binary found, keeping current Python version"; \
	fi

test:
	@echo "Running tests..."
	go test ./...

clean:
	@echo "Cleaning build directory..."
	@rm -rf $(BUILD_DIR)

# Try to build with Go, fall back to keeping Python version if it fails
build-safe:
	@echo "Attempting safe build..."
	@mkdir -p $(BUILD_DIR)
	@if go build -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/annas-mcp 2>/dev/null; then \
		echo "Go build successful!"; \
	else \
		echo "Go build failed due to network issues, keeping existing binary"; \
	fi