.PHONY: build clean test run-parse run-convert docker-build

# Build all binaries to bin/
build:
	@echo "Building binaries to bin/..."
	@mkdir -p bin
	@go build -o bin/parse-demo ./cmd/parse-demo
	@go build -o bin/convert-otel ./cmd/convert-otel
	@go build -o bin/lambda ./cmd/lambda
	@echo "✓ Build complete! Binaries in ./bin/"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@rm -f bootstrap lambda.zip
	@echo "✓ Clean complete!"

# Run tests
test:
	@go test -v ./...

# Run tests with coverage
test-coverage:
	@go test -cover ./...

# Run parse-demo
run-parse:
	@./bin/parse-demo $(FILE)

# Run convert-otel
run-convert:
	@./bin/convert-otel $(FILE)

# Build Docker image
docker-build:
	@docker build --no-cache -t alb-processor:latest .

# Build Lambda deployment package
lambda-package: build
	@echo "Creating Lambda deployment package..."
	@cd bin && zip -r ../lambda.zip lambda
	@echo "✓ Lambda package created: lambda.zip"

# Install development dependencies
dev-setup:
	@go mod download
	@go install golang.org/x/tools/gopls@latest

# Format code
fmt:
	@go fmt ./...

# Lint code (requires golangci-lint)
lint:
	@golangci-lint run

help:
	@echo "Available targets:"
	@echo "  make build          - Build all binaries to bin/"
	@echo "  make clean          - Remove build artifacts"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage"
	@echo "  make run-parse      - Run parse-demo (use FILE=path)"
	@echo "  make run-convert    - Run convert-otel (use FILE=path)"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make lambda-package - Create Lambda deployment package"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Lint code"
