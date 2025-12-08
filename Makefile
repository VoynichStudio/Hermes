.PHONY: all build run test clean proto lint fmt help

# Default target
all: proto build

# Build the server binary
build:
	@echo "Building server..."
	go build -o bin/server ./cmd/server

# Run the server
run: build
	@echo "Running server..."
	./bin/server

# Run the server with hot reload (requires air: go install github.com/air-verse/air@latest)
dev:
	@echo "Running server with hot reload..."
	air

# Run tests
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

# Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	buf generate

# Lint protobuf files
proto-lint:
	@echo "Linting protobuf files..."
	buf lint

# Run Go linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	golangci-lint run ./...

# Format Go code
fmt:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	go mod tidy

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -rf bin/
	rm -rf gen/
	rm -f coverage.out coverage.html

# Install development tools
tools:
	@echo "Installing development tools..."
	go install github.com/air-verse/air@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/bufbuild/buf/cmd/buf@latest

# Show help
help:
	@echo "Available targets:"
	@echo "  all           - Generate proto and build (default)"
	@echo "  build         - Build the server binary"
	@echo "  run           - Build and run the server"
	@echo "  dev           - Run with hot reload (requires air)"
	@echo "  test          - Run tests"
	@echo "  test-coverage - Run tests with coverage report"
	@echo "  proto         - Generate protobuf code"
	@echo "  proto-lint    - Lint protobuf files"
	@echo "  lint          - Run Go linter"
	@echo "  fmt           - Format Go code"
	@echo "  tidy          - Tidy Go dependencies"
	@echo "  clean         - Remove build artifacts"
	@echo "  tools         - Install development tools"
	@echo "  help          - Show this help"
