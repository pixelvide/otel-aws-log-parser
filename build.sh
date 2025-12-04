#!/bin/bash
# Build script for local development

set -e

# Create bin directory if it doesn't exist
mkdir -p bin

echo "Building binaries to bin/..."

# Build parse-demo
echo "Building parse-demo..."
go build -o bin/parse-demo ./cmd/parse-demo

# Build convert-otel
echo "Building convert-otel..."
go build -o bin/convert-otel ./cmd/convert-otel

# Build lambda (for local testing)
echo "Building lambda..."
go build -o bin/lambda ./cmd/lambda

echo ""
echo "✓ Build complete!"
echo "Binaries are in ./bin/"
echo ""
echo "Usage:"
echo "  ./bin/parse-demo <log-file>"
echo "  ./bin/convert-otel <log-file>"
