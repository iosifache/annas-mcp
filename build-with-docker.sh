#!/bin/bash

# Build annas-mcp using Docker to avoid IPv6 issues
echo "Building annas-mcp with Docker (IPv4-only)..."

# Build using golang Docker image
docker run --rm \
  -v "$PWD":/app \
  -w /app \
  golang:1.21 \
  bash -c "go mod download && go build -o annas-mcp-fixed cmd/annas-mcp/main.go"

if [ -f "annas-mcp-fixed" ]; then
  echo "Build successful! Binary created: annas-mcp-fixed"
  echo "Making it executable..."
  chmod +x annas-mcp-fixed
  
  echo ""
  echo "To install the fixed version:"
  echo "  cp annas-mcp-fixed ~/.local/bin/annas-mcp"
else
  echo "Build failed!"
  exit 1
fi