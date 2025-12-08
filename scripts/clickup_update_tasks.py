#!/usr/bin/env python3
"""
ClickUp Task Updater for Hermes Chat Engine
Updates existing tasks with detailed descriptions and acceptance criteria.

Usage:
    python clickup_update_tasks.py --api-key YOUR_API_KEY --team-id YOUR_TEAM_ID
    python clickup_update_tasks.py --api-key YOUR_API_KEY --team-id YOUR_TEAM_ID --dry-run

Requirements:
    pip install requests
"""

import argparse
import requests
import time
import sys
import re
from typing import Optional
from dataclasses import dataclass, field

# ============================================================================
# Configuration
# ============================================================================

SPACE_NAME = "Core"
FOLDER_NAME = "Hermes"
LIST_NAME = "Product Backlog"

BASE_URL = "https://api.clickup.com/api/v2"
DEBUG = False

# ============================================================================
# Detailed Task Descriptions
# ============================================================================

def get_detailed_descriptions() -> dict:
    """
    Returns detailed descriptions for each task.
    Key format: "Story Name|Task Name" -> detailed description
    """

    descriptions = {}

    # =========================================================================
    # Epic 0: Foundation & Critical Fixes
    # =========================================================================

    # Story 0.1: Fix Critical Bugs
    descriptions["Story 0.1: Fix Critical Bugs|Fix double RUnlock() in SendMessage()"] = """
## Description
The `SendMessage()` function in the chat server has a critical bug where `RUnlock()` is called twice on the same mutex, which will cause a panic at runtime.

## Current Issue
```go
func (s *Server) SendMessage(...) {
    s.mu.RLock()
    // ... code ...
    s.mu.RUnlock()  // First unlock
    // ... more code ...
    s.mu.RUnlock()  // BUG: Second unlock causes panic!
}
```

## Steps to Fix
1. Open `internal/chat/handlers.go` (or wherever SendMessage is defined)
2. Trace the mutex lock/unlock flow
3. Remove the duplicate `RUnlock()` call
4. Ensure every `RLock()` has exactly one corresponding `RUnlock()`

## Acceptance Criteria
- [ ] No panic occurs when running concurrent message sends
- [ ] Unit test added that sends 100+ concurrent messages without crash
- [ ] Code review confirms single unlock per lock
"""

    descriptions["Story 0.1: Fix Critical Bugs|Fix race condition in channel user access"] = """
## Description
When iterating over channel users to broadcast messages, the read lock must be held for the entire iteration, not just for accessing the channel.

## Current Issue
The code releases the lock before iterating, allowing concurrent modifications to `ch.Users` slice.

## Steps to Fix
1. Identify where `ch.Users` is accessed
2. Copy user IDs to a local slice while holding the lock
3. Release lock after copying
4. Iterate over the local copy (not the shared slice)

## Example Fix
```go
s.mu.RLock()
userIDs := make([]string, len(ch.Users))
for i, u := range ch.Users {
    userIDs[i] = u.Id
}
s.mu.RUnlock()

// Now safe to iterate without lock
for _, userID := range userIDs {
    // send message
}
```

## Acceptance Criteria
- [ ] Race detector passes: `go test -race ./...`
- [ ] Concurrent join/leave while broadcasting doesn't panic
- [ ] Test with 50+ concurrent channel operations
"""

    descriptions["Story 0.1: Fix Critical Bugs|Add proper error handling in main()"] = """
## Description
The server's main() function should handle errors gracefully and provide clear error messages when startup fails.

## Steps to Fix
1. Wrap `http.ListenAndServe()` in error handling
2. Log fatal errors with context
3. Return appropriate exit codes
4. Add timeout for graceful shutdown

## Example
```go
func main() {
    // ... setup ...

    server := &http.Server{
        Addr:    cfg.Server.Address(),
        Handler: mux,
    }

    log.Printf("Starting server on %s", cfg.Server.Address())
    if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
        log.Fatalf("Server failed to start: %v", err)
    }
}
```

## Acceptance Criteria
- [ ] Clear error message when port is already in use
- [ ] Clear error message when address is invalid
- [ ] Exit code 1 on startup failure
- [ ] Log message confirms successful startup with address
"""

    descriptions["Story 0.1: Fix Critical Bugs|Add graceful shutdown with signal handling"] = """
## Description
The server should handle SIGTERM and SIGINT signals to gracefully drain connections before shutting down.

## Steps to Fix
1. Create a signal channel listening for SIGTERM/SIGINT
2. On signal, call `server.Shutdown(ctx)` with timeout context
3. Wait for in-flight requests to complete
4. Clean up resources (close DB connections, etc.)

## Example
```go
// Signal handling
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

go func() {
    <-quit
    log.Println("Shutting down server...")

    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()

    if err := server.Shutdown(ctx); err != nil {
        log.Printf("Server forced to shutdown: %v", err)
    }
}()
```

## Acceptance Criteria
- [ ] `kill -TERM <pid>` triggers graceful shutdown
- [ ] Ctrl+C in terminal triggers graceful shutdown
- [ ] In-flight streaming connections complete within timeout
- [ ] Log message indicates shutdown initiated and completed
"""

    # Story 0.2: Configuration Management
    descriptions["Story 0.2: Configuration Management|Create config struct with all settings"] = """
## Description
Create a centralized configuration struct that holds all application settings in one place.

## Steps to Implement
1. Create `internal/config/config.go`
2. Define nested structs for logical groupings:
   - `ServerConfig`: host, port, timeouts
   - `CognitoConfig`: region, user_pool_id, client_id
   - `LogConfig`: level, format, output
3. Add getter methods where needed (e.g., `Address() string`)

## Example Structure
```go
type Config struct {
    Server  ServerConfig
    Cognito CognitoConfig
    Log     LogConfig
}

type ServerConfig struct {
    Host         string
    Port         int
    ReadTimeout  time.Duration
    WriteTimeout time.Duration
}

func (s ServerConfig) Address() string {
    return fmt.Sprintf("%s:%d", s.Host, s.Port)
}
```

## Acceptance Criteria
- [ ] Single Config struct contains all settings
- [ ] Logical grouping with nested structs
- [ ] No global variables for configuration
- [ ] Config can be passed via dependency injection
"""

    descriptions["Story 0.2: Configuration Management|Add environment variable support"] = """
## Description
All configuration values should be overridable via environment variables for 12-factor app compliance.

## Steps to Implement
1. Create a `Load()` function that reads from env vars
2. Use consistent naming: `HERMES_SERVER_PORT`, `HERMES_COGNITO_REGION`
3. Provide sensible defaults for development
4. Use `os.Getenv()` with fallback to defaults

## Example
```go
func Load() (*Config, error) {
    return &Config{
        Server: ServerConfig{
            Host: getEnv("HERMES_SERVER_HOST", "localhost"),
            Port: getEnvInt("HERMES_SERVER_PORT", 8080),
        },
        Cognito: CognitoConfig{
            Region:     getEnv("HERMES_COGNITO_REGION", "us-east-1"),
            UserPoolID: getEnv("HERMES_COGNITO_USER_POOL_ID", ""),
        },
    }, nil
}
```

## Acceptance Criteria
- [ ] All settings configurable via env vars
- [ ] Naming convention: `HERMES_<SECTION>_<KEY>`
- [ ] Defaults work for local development
- [ ] Sensitive values (API keys) have no defaults
"""

    descriptions["Story 0.2: Configuration Management|Add config file support (YAML/TOML)"] = """
## Description
Support optional configuration files that can be overridden by environment variables.

## Steps to Implement
1. Add `gopkg.in/yaml.v3` dependency
2. Create config file parsing in `Load()`
3. Priority: ENV > config file > defaults
4. Support multiple file locations: `./config.yaml`, `/etc/hermes/config.yaml`

## Example Config File (config.yaml)
```yaml
server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 30s
  write_timeout: 30s

cognito:
  region: us-east-1
  user_pool_id: us-east-1_XXXXX

log:
  level: info
  format: json
```

## Acceptance Criteria
- [ ] YAML config file parsed correctly
- [ ] Environment variables override file values
- [ ] Missing config file doesn't cause error (uses defaults)
- [ ] Config file path configurable via `HERMES_CONFIG_PATH`
"""

    descriptions["Story 0.2: Configuration Management|Validate configuration on startup"] = """
## Description
Validate all configuration values on startup and fail fast with clear error messages.

## Steps to Implement
1. Add `Validate()` method to Config struct
2. Check required fields are not empty
3. Check numeric values are in valid ranges
4. Return descriptive error messages

## Example
```go
func (c *Config) Validate() error {
    if c.Cognito.UserPoolID == "" {
        return fmt.Errorf("HERMES_COGNITO_USER_POOL_ID is required")
    }
    if c.Server.Port < 1 || c.Server.Port > 65535 {
        return fmt.Errorf("HERMES_SERVER_PORT must be between 1-65535, got %d", c.Server.Port)
    }
    return nil
}
```

## Acceptance Criteria
- [ ] Server exits with code 1 if config invalid
- [ ] Error message states exactly which field is wrong
- [ ] Error message includes the invalid value
- [ ] All required fields validated
"""

    descriptions["Story 0.2: Configuration Management|Add configuration for Cognito (region, pool ID)"] = """
## Description
Remove hardcoded Cognito values and make them configurable.

## Current Issue
```go
const (
    cognitoRegion     = "us-east-1"  // HARDCODED!
    cognitoUserPoolID = "us-east-1_example"  // HARDCODED!
)
```

## Steps to Fix
1. Remove hardcoded constants from `pkg/auth/auth.go`
2. Accept Cognito config via function parameter or package init
3. Build JWKS URL from configured values
4. Update all callers to pass config

## Example
```go
type AuthConfig struct {
    Region     string
    UserPoolID string
}

func NewAuthenticator(cfg AuthConfig) (*Authenticator, error) {
    issuer := fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s",
        cfg.Region, cfg.UserPoolID)
    // ...
}
```

## Acceptance Criteria
- [ ] No hardcoded AWS values in code
- [ ] Auth works with any Cognito region
- [ ] Auth works with any User Pool ID
- [ ] Tests can use mock values
"""

    # Story 0.3: Project Structure Refactor
    descriptions["Story 0.3: Project Structure Refactor|Create internal/ directory for private packages"] = """
## Description
Create the `internal/` directory for packages that should not be imported by external projects.

## Go Convention
- `internal/` packages can only be imported by code in the parent of `internal/`
- This is enforced by the Go compiler

## Directory Structure
```
Hermes/
├── internal/
│   ├── chat/       # Chat server logic
│   ├── config/     # Configuration management
│   └── middleware/ # HTTP/gRPC middleware
```

## Steps to Implement
1. Create `internal/` directory
2. Move private packages under `internal/`
3. Update all import paths
4. Verify with `go build ./...`

## Acceptance Criteria
- [ ] `internal/` directory created
- [ ] Private packages moved under internal/
- [ ] All imports updated
- [ ] `go build ./...` succeeds
"""

    descriptions["Story 0.3: Project Structure Refactor|Create pkg/ directory for shared libraries"] = """
## Description
Create the `pkg/` directory for packages that can be imported by external projects.

## Go Convention
- `pkg/` packages are considered public API
- External projects can import `github.com/you/Hermes/pkg/auth`

## Directory Structure
```
Hermes/
├── pkg/
│   └── auth/       # Authentication (reusable across services)
```

## Steps to Implement
1. Create `pkg/` directory
2. Move auth package to `pkg/auth/`
3. Update import paths across codebase
4. Document public API in comments

## Acceptance Criteria
- [ ] `pkg/` directory created
- [ ] Auth package moved to `pkg/auth/`
- [ ] Import paths updated
- [ ] Public functions have doc comments
"""

    descriptions["Story 0.3: Project Structure Refactor|Move chat logic to internal/chat/"] = """
## Description
Extract chat server logic from main.go into a dedicated internal package.

## Steps to Implement
1. Create `internal/chat/` directory
2. Create `server.go` with Server struct
3. Create `handlers.go` with RPC handlers
4. Create `errors.go` with error definitions
5. Update main.go to use the new package

## File Structure
```
internal/chat/
├── server.go      # Server struct, NewServer(), connection management
├── handlers.go    # JoinChannel(), SendMessage() RPCs
└── errors.go      # ErrChannelNotFound, etc.
```

## Acceptance Criteria
- [ ] Chat logic in dedicated package
- [ ] main.go only handles startup/shutdown
- [ ] Clear separation of concerns
- [ ] Unit tests can import internal/chat
"""

    descriptions["Story 0.3: Project Structure Refactor|Create internal/config/ package"] = """
## Description
Create a dedicated configuration package under internal/.

## Steps to Implement
1. Create `internal/config/config.go`
2. Move Config struct and related types
3. Implement Load() function
4. Implement Validate() function
5. Update main.go to use config.Load()

## Package Contents
```go
// internal/config/config.go
package config

type Config struct { ... }
type ServerConfig struct { ... }
type CognitoConfig struct { ... }

func Load() (*Config, error)
func (c *Config) Validate() error
```

## Acceptance Criteria
- [ ] Config package created at internal/config/
- [ ] Load() reads from env vars and files
- [ ] Validate() checks all required fields
- [ ] main.go uses config.Load()
"""

    descriptions["Story 0.3: Project Structure Refactor|Add Makefile with common commands"] = """
## Description
Create a Makefile with standard development commands.

## Commands to Include
```makefile
.PHONY: build run test clean proto lint

build:           # Build the server binary
run:             # Build and run the server
dev:             # Run with hot reload (air)
test:            # Run all tests
test-coverage:   # Run tests with coverage report
test-race:       # Run tests with race detector
proto:           # Generate protobuf code
lint:            # Run golangci-lint
fmt:             # Format code
tidy:            # go mod tidy
clean:           # Remove build artifacts
```

## Acceptance Criteria
- [ ] `make build` creates binary at `bin/server`
- [ ] `make test` runs all tests
- [ ] `make lint` runs golangci-lint
- [ ] `make proto` generates protobuf code
- [ ] `make help` shows available targets
"""

    descriptions["Story 0.3: Project Structure Refactor|Add .env.example file"] = """
## Description
Create a .env.example file documenting all environment variables.

## Contents
```bash
# Server Configuration
HERMES_SERVER_HOST=localhost
HERMES_SERVER_PORT=8080

# AWS Cognito Configuration
HERMES_COGNITO_REGION=us-east-1
HERMES_COGNITO_USER_POOL_ID=us-east-1_XXXXX

# Logging
HERMES_LOG_LEVEL=info
HERMES_LOG_FORMAT=text  # text or json

# Database (future)
# HERMES_POSTGRES_URL=postgres://user:pass@localhost:5432/hermes
# HERMES_SCYLLA_HOSTS=localhost:9042
# HERMES_REDIS_URL=redis://localhost:6379
```

## Acceptance Criteria
- [ ] .env.example file created at project root
- [ ] All environment variables documented
- [ ] Example values provided (not real secrets)
- [ ] Comments explain each variable
- [ ] .env added to .gitignore
"""

    # Story 0.4: Testing Foundation
    descriptions["Story 0.4: Testing Foundation|Add unit test framework setup"] = """
## Description
Set up the testing infrastructure so `go test ./...` works correctly.

## Steps to Implement
1. Create test files alongside source files (*_test.go)
2. Set up test helpers for common operations
3. Add GO_TEST_MODE environment variable handling
4. Configure test timeouts

## Test Organization
```
internal/chat/
├── server.go
├── server_test.go      # Tests for server.go
├── handlers.go
└── handlers_test.go    # Tests for handlers.go
```

## Acceptance Criteria
- [ ] `go test ./...` passes
- [ ] Tests don't require external services
- [ ] Test coverage > 60%
- [ ] Tests complete in < 30 seconds
"""

    descriptions["Story 0.4: Testing Foundation|Write tests for auth validation"] = """
## Description
Write comprehensive unit tests for the authentication package.

## Test Cases
1. Valid JWT token with correct claims
2. Expired JWT token
3. Invalid signature
4. Missing required claims (sub, iss)
5. Wrong issuer
6. Missing Authorization header
7. Malformed token

## Example Test
```go
func TestAuthenticate_ValidToken(t *testing.T) {
    setupTestAuth()  // Configure test key

    token := createTestToken(jwt.MapClaims{
        "iss": GetCognitoIssuer(),
        "sub": "user-123",
        "exp": time.Now().Add(time.Hour).Unix(),
    })

    header := http.Header{}
    header.Set("Authorization", "Bearer " + token)

    user, err := Authenticate(header)
    require.NoError(t, err)
    assert.Equal(t, "user-123", user.Id)
}
```

## Acceptance Criteria
- [ ] Tests cover all error cases
- [ ] Tests use mock JWT signing key
- [ ] Coverage > 90% for auth package
- [ ] Tests don't call real Cognito
"""

    descriptions["Story 0.4: Testing Foundation|Write tests for chat server logic"] = """
## Description
Write unit tests for the chat server's core functionality.

## Test Cases
1. NewServer() creates server with default channel
2. GetChannel() returns existing channel
3. GetChannel() returns false for non-existent
4. GetOrCreateChannel() creates new channel
5. GetOrCreateChannel() returns existing channel
6. AddUserToChannel() adds user correctly
7. BroadcastToChannel() sends to all users
8. BroadcastToChannel() handles non-existent channel
9. Concurrent operations don't race

## Example Test
```go
func TestServer_BroadcastToChannel(t *testing.T) {
    server := NewServer()

    // Add users
    user1 := &chatv1.User{Id: "user-1"}
    user2 := &chatv1.User{Id: "user-2"}
    server.AddUserToChannel("general", user1)
    server.AddUserToChannel("general", user2)

    // Register streams
    stream1 := server.RegisterUserStream("user-1")
    stream2 := server.RegisterUserStream("user-2")

    // Broadcast
    msg := &chatv1.Message{Content: "Hello"}
    err := server.BroadcastToChannel("general", msg)
    require.NoError(t, err)

    // Verify both received
    assert.Equal(t, "Hello", (<-stream1).Content)
    assert.Equal(t, "Hello", (<-stream2).Content)
}
```

## Acceptance Criteria
- [ ] All public methods have tests
- [ ] Concurrent access tested with -race
- [ ] Edge cases covered (empty channel, full buffer)
- [ ] Coverage > 80% for chat package
"""

    descriptions["Story 0.4: Testing Foundation|Add integration test setup with testcontainers"] = """
## Description
Set up testcontainers for integration tests that need real databases.

## Steps to Implement
1. Add testcontainers-go dependency
2. Create test helpers to start/stop containers
3. Configure containers for ScyllaDB, PostgreSQL, Redis
4. Add build tag for integration tests

## Example
```go
//go:build integration

func TestWithPostgres(t *testing.T) {
    ctx := context.Background()

    container, err := postgres.RunContainer(ctx,
        testcontainers.WithImage("postgres:15"),
        postgres.WithDatabase("test"),
    )
    require.NoError(t, err)
    defer container.Terminate(ctx)

    connStr, _ := container.ConnectionString(ctx)
    // Run tests with real postgres...
}
```

## Acceptance Criteria
- [ ] Integration tests separated with build tag
- [ ] Containers start/stop automatically
- [ ] Tests run in CI with Docker
- [ ] Each test gets fresh container
"""

    descriptions["Story 0.4: Testing Foundation|Add test coverage reporting"] = """
## Description
Set up code coverage reporting for CI visibility.

## Steps to Implement
1. Add coverage commands to Makefile
2. Generate coverage.out file
3. Configure Codecov or similar service
4. Add coverage badge to README

## Makefile Targets
```makefile
test-coverage:
    go test -coverprofile=coverage.out ./...
    go tool cover -html=coverage.out -o coverage.html
    go tool cover -func=coverage.out
```

## Acceptance Criteria
- [ ] Coverage report generated on every test run
- [ ] HTML report available locally
- [ ] Coverage uploaded to Codecov in CI
- [ ] Coverage badge in README
"""

    descriptions["Story 0.4: Testing Foundation|Create mock generators for interfaces"] = """
## Description
Set up mock generation for interfaces to enable easy unit testing.

## Options
1. **mockgen** (Go official): `go install github.com/golang/mock/mockgen`
2. **mockery** (popular): `go install github.com/vektra/mockery/v2`

## Steps to Implement
1. Choose mock generator tool
2. Add go:generate directives to interfaces
3. Generate mocks in internal/mocks/
4. Document mock usage in tests

## Example
```go
//go:generate mockgen -destination=../mocks/mock_repository.go -package=mocks . ChannelRepository

type ChannelRepository interface {
    GetChannel(id string) (*Channel, error)
    CreateChannel(ch *Channel) error
}
```

## Acceptance Criteria
- [ ] Mock generator tool installed
- [ ] go:generate directives added
- [ ] `go generate ./...` creates mocks
- [ ] Example test using mocks
"""

    # Story 0.5: CI/CD Pipeline Setup
    descriptions["Story 0.5: CI/CD Pipeline Setup|Create GitHub Actions workflow for tests"] = """
## Description
Create a GitHub Actions workflow that runs tests on every PR.

## Workflow File: .github/workflows/ci.yml
```yaml
name: CI

on:
  push:
    branches: [main, dev]
  pull_request:
    branches: [main, dev]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go test -race -v ./...
```

## Acceptance Criteria
- [ ] Tests run on every PR
- [ ] Tests run on push to main/dev
- [ ] Race detector enabled
- [ ] Workflow fails if tests fail
"""

    descriptions["Story 0.5: CI/CD Pipeline Setup|Add linting (golangci-lint)"] = """
## Description
Add golangci-lint to CI to enforce code quality.

## Steps to Implement
1. Create .golangci.yml configuration
2. Add lint job to CI workflow
3. Enable useful linters (errcheck, govet, staticcheck)
4. Configure exclusions for generated code

## .golangci.yml
```yaml
linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - unused
    - gofmt
    - misspell

issues:
  exclude-rules:
    - path: gen/
      linters: [all]
```

## Acceptance Criteria
- [ ] golangci-lint runs in CI
- [ ] Config file defines enabled linters
- [ ] Generated code excluded
- [ ] PRs fail if lint issues found
"""

    descriptions["Story 0.5: CI/CD Pipeline Setup|Add security scanning (gosec)"] = """
## Description
Add security vulnerability scanning to catch issues early.

## Steps to Implement
1. Add gosec to CI workflow
2. Configure severity thresholds
3. Exclude false positives if needed

## Workflow Addition
```yaml
- name: Run gosec
  uses: securego/gosec@master
  with:
    args: ./...
```

## Acceptance Criteria
- [ ] gosec runs in CI
- [ ] High severity issues fail the build
- [ ] Results visible in PR checks
"""

    descriptions["Story 0.5: CI/CD Pipeline Setup|Add proto generation verification"] = """
## Description
Verify that generated protobuf code is up to date in CI.

## Steps to Implement
1. Add proto generation step to CI
2. Compare generated files with committed files
3. Fail if they differ

## Workflow
```yaml
- name: Generate proto
  run: make proto

- name: Check for changes
  run: |
    if [ -n "$(git status --porcelain gen/)" ]; then
      echo "Generated code is out of date!"
      git diff gen/
      exit 1
    fi
```

## Acceptance Criteria
- [ ] CI generates proto code
- [ ] CI fails if generated code differs
- [ ] Clear error message explaining issue
"""

    descriptions["Story 0.5: CI/CD Pipeline Setup|Add build workflow"] = """
## Description
Add a workflow that builds release binaries on main branch.

## Steps to Implement
1. Create build job that depends on test/lint
2. Build binary with version info
3. Upload as artifact

## Workflow
```yaml
build:
  runs-on: ubuntu-latest
  needs: [test, lint]
  steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v5
    - run: go build -o bin/server ./cmd/server
    - uses: actions/upload-artifact@v4
      with:
        name: server-binary
        path: bin/server
```

## Acceptance Criteria
- [ ] Build runs after tests pass
- [ ] Binary uploaded as artifact
- [ ] Build includes version info
"""

    # =========================================================================
    # Epic 1: Core Chat Service Hardening
    # =========================================================================

    descriptions["Story 1.1: Interface-Based Architecture|Define ChannelRepository interface"] = """
## Description
Define an interface for channel storage operations to enable different implementations.

## Interface Definition
```go
type ChannelRepository interface {
    // Get retrieves a channel by ID
    Get(ctx context.Context, id string) (*Channel, error)

    // Create creates a new channel
    Create(ctx context.Context, ch *Channel) error

    // Update updates channel metadata
    Update(ctx context.Context, ch *Channel) error

    // Delete removes a channel
    Delete(ctx context.Context, id string) error

    // List returns channels matching criteria
    List(ctx context.Context, opts ListOptions) ([]*Channel, error)

    // AddMember adds a user to a channel
    AddMember(ctx context.Context, channelID, userID string) error

    // RemoveMember removes a user from a channel
    RemoveMember(ctx context.Context, channelID, userID string) error

    // GetMembers returns all members of a channel
    GetMembers(ctx context.Context, channelID string) ([]string, error)
}
```

## Acceptance Criteria
- [ ] Interface defined in internal/chat/repository.go
- [ ] All operations have context parameter
- [ ] Error types defined for common cases
- [ ] Interface is mockable for testing
"""

    descriptions["Story 1.1: Interface-Based Architecture|Define MessageRepository interface"] = """
## Description
Define an interface for message storage operations.

## Interface Definition
```go
type MessageRepository interface {
    // Save persists a message
    Save(ctx context.Context, msg *Message) error

    // GetByID retrieves a single message
    GetByID(ctx context.Context, id string) (*Message, error)

    // GetHistory retrieves messages for a channel
    GetHistory(ctx context.Context, channelID string, opts HistoryOptions) ([]*Message, error)

    // Delete removes a message
    Delete(ctx context.Context, id string) error

    // Search finds messages matching criteria
    Search(ctx context.Context, query SearchQuery) ([]*Message, error)
}

type HistoryOptions struct {
    Before    time.Time
    After     time.Time
    Limit     int
    Cursor    string
}
```

## Acceptance Criteria
- [ ] Interface supports pagination
- [ ] Time-range queries supported
- [ ] Cursor-based pagination for efficiency
- [ ] Interface mockable for tests
"""

    descriptions["Story 1.1: Interface-Based Architecture|Define UserSessionManager interface"] = """
## Description
Define an interface for managing user sessions and connections.

## Interface Definition
```go
type UserSessionManager interface {
    // Register creates a new session for a user
    Register(ctx context.Context, userID string, serverID string) (*Session, error)

    // Unregister removes a user's session
    Unregister(ctx context.Context, userID string) error

    // GetSession retrieves session info
    GetSession(ctx context.Context, userID string) (*Session, error)

    // GetServerForUser returns which server instance a user is connected to
    GetServerForUser(ctx context.Context, userID string) (string, error)

    // RefreshSession extends session TTL
    RefreshSession(ctx context.Context, userID string) error

    // GetActiveSessions returns all active sessions (for metrics)
    GetActiveSessions(ctx context.Context) (int, error)
}
```

## Acceptance Criteria
- [ ] Session tracks server instance
- [ ] Sessions have TTL
- [ ] Interface supports distributed setup
"""

    descriptions["Story 1.1: Interface-Based Architecture|Define MessageBroadcaster interface"] = """
## Description
Define an interface for broadcasting messages to channel subscribers.

## Interface Definition
```go
type MessageBroadcaster interface {
    // Subscribe adds a listener for a channel
    Subscribe(ctx context.Context, channelID string, handler MessageHandler) error

    // Unsubscribe removes a listener
    Unsubscribe(ctx context.Context, channelID string, handler MessageHandler) error

    // Broadcast sends a message to all channel subscribers
    Broadcast(ctx context.Context, channelID string, msg *Message) error

    // BroadcastToUser sends a message directly to a user
    BroadcastToUser(ctx context.Context, userID string, msg *Message) error
}

type MessageHandler func(msg *Message) error
```

## Acceptance Criteria
- [ ] Supports pub/sub pattern
- [ ] Can broadcast to channel or user
- [ ] Handler errors don't block other deliveries
"""

    descriptions["Story 1.1: Interface-Based Architecture|Create in-memory implementations"] = """
## Description
Create in-memory implementations of all interfaces for development and testing.

## Files to Create
```
internal/chat/
├── repository_memory.go      # InMemoryChannelRepository
├── message_store_memory.go   # InMemoryMessageRepository
├── session_manager_memory.go # InMemorySessionManager
└── broadcaster_memory.go     # InMemoryBroadcaster
```

## Example
```go
type InMemoryChannelRepository struct {
    channels map[string]*Channel
    mu       sync.RWMutex
}

func NewInMemoryChannelRepository() *InMemoryChannelRepository {
    return &InMemoryChannelRepository{
        channels: make(map[string]*Channel),
    }
}

func (r *InMemoryChannelRepository) Get(ctx context.Context, id string) (*Channel, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    ch, ok := r.channels[id]
    if !ok {
        return nil, ErrChannelNotFound
    }
    return ch, nil
}
```

## Acceptance Criteria
- [ ] All interfaces have in-memory implementation
- [ ] Thread-safe with proper locking
- [ ] Tests pass with in-memory implementations
- [ ] Current behavior preserved
"""

    descriptions["Story 1.1: Interface-Based Architecture|Use dependency injection in server"] = """
## Description
Refactor the server to accept dependencies via constructor injection.

## Before
```go
type Server struct {
    channels map[string]*Channel  // hardcoded in-memory
}
```

## After
```go
type Server struct {
    channels    ChannelRepository
    messages    MessageRepository
    sessions    UserSessionManager
    broadcaster MessageBroadcaster
}

func NewServer(opts ...ServerOption) *Server {
    s := &Server{}
    for _, opt := range opts {
        opt(s)
    }
    return s
}

// Functional options
func WithChannelRepository(r ChannelRepository) ServerOption {
    return func(s *Server) { s.channels = r }
}
```

## Acceptance Criteria
- [ ] Dependencies injected via constructor
- [ ] Functional options pattern used
- [ ] Easy to swap implementations
- [ ] Tests can inject mocks
"""

    # Add more detailed descriptions for remaining stories...
    # (This is a subset - the full script would include all stories)

    return descriptions


# ============================================================================
# API Client
# ============================================================================

class ClickUpClient:
    def __init__(self, api_key: str):
        self.api_key = api_key
        self.headers = {
            "Authorization": api_key,
            "Content-Type": "application/json"
        }
        self.request_count = 0

    def _request(self, method: str, endpoint: str, data: dict = None) -> dict:
        """Make an API request with rate limiting"""
        self.request_count += 1

        if self.request_count % 10 == 0:
            time.sleep(0.5)

        url = f"{BASE_URL}/{endpoint}"

        if DEBUG:
            print(f"  DEBUG: {method} {url}")

        try:
            if method == "GET":
                response = requests.get(url, headers=self.headers)
            elif method == "POST":
                response = requests.post(url, headers=self.headers, json=data)
            elif method == "PUT":
                response = requests.put(url, headers=self.headers, json=data)
            else:
                raise ValueError(f"Unknown method: {method}")

            if response.status_code == 429:
                print("  Rate limited, waiting 60 seconds...")
                time.sleep(60)
                return self._request(method, endpoint, data)

            response.raise_for_status()
            return response.json() if response.text else {}

        except requests.exceptions.RequestException as e:
            print(f"  Error: {e}")
            raise

    def get_spaces(self, team_id: str) -> list:
        return self._request("GET", f"team/{team_id}/space")["spaces"]

    def get_folders(self, space_id: str) -> list:
        return self._request("GET", f"space/{space_id}/folder")["folders"]

    def get_lists(self, folder_id: str) -> list:
        return self._request("GET", f"folder/{folder_id}/list")["lists"]

    def get_tasks(self, list_id: str, subtasks: bool = True) -> list:
        """Get all tasks in a list"""
        params = f"?subtasks={'true' if subtasks else 'false'}&include_closed=true"
        return self._request("GET", f"list/{list_id}/task{params}")["tasks"]

    def get_task(self, task_id: str) -> dict:
        """Get a single task by ID"""
        return self._request("GET", f"task/{task_id}")

    def update_task(self, task_id: str, data: dict) -> dict:
        """Update a task"""
        return self._request("PUT", f"task/{task_id}", data)

    def get_checklists(self, task_id: str) -> list:
        """Get checklists for a task"""
        task = self.get_task(task_id)
        return task.get("checklists", [])

    def update_checklist_item(self, checklist_id: str, item_id: str, data: dict) -> dict:
        """Update a checklist item"""
        return self._request("PUT", f"checklist/{checklist_id}/checklist_item/{item_id}", data)


# ============================================================================
# Main Update Logic
# ============================================================================

def update_tasks(api_key: str, team_id: str, dry_run: bool = False):
    """Update all tasks with detailed descriptions"""

    print("=" * 60)
    print("ClickUp Task Updater for Hermes Chat Engine")
    print("=" * 60)

    client = ClickUpClient(api_key)
    descriptions = get_detailed_descriptions()

    # Find the list
    print("\n[1/4] Finding workspace structure...")

    spaces = client.get_spaces(team_id)
    space = next((s for s in spaces if s["name"].lower() == SPACE_NAME.lower()), None)
    if not space:
        print(f"  ERROR: Space '{SPACE_NAME}' not found!")
        sys.exit(1)
    print(f"  Found space: {space['name']}")

    folders = client.get_folders(space["id"])
    folder = next((f for f in folders if f["name"].lower() == FOLDER_NAME.lower()), None)
    if not folder:
        print(f"  ERROR: Folder '{FOLDER_NAME}' not found!")
        sys.exit(1)
    print(f"  Found folder: {folder['name']}")

    lists = client.get_lists(folder["id"])
    task_list = next((l for l in lists if l["name"].lower() == LIST_NAME.lower()), None)
    if not task_list:
        print(f"  ERROR: List '{LIST_NAME}' not found!")
        sys.exit(1)
    print(f"  Found list: {task_list['name']}")

    # Get all tasks
    print("\n[2/4] Fetching existing tasks...")
    tasks = client.get_tasks(task_list["id"])
    print(f"  Found {len(tasks)} tasks")

    # Build task hierarchy
    print("\n[3/4] Analyzing task structure...")

    epics = {}  # id -> task
    stories = {}  # id -> (epic_id, task)

    for task in tasks:
        if task["name"].startswith("Epic"):
            epics[task["id"]] = task
        elif task["name"].startswith("Story"):
            parent_id = task.get("parent")
            stories[task["id"]] = (parent_id, task)

    print(f"  Found {len(epics)} Epics and {len(stories)} Stories")

    # Update tasks with descriptions
    print("\n[4/4] Updating task descriptions...")

    updates_made = 0
    updates_skipped = 0

    for story_id, (epic_id, story) in stories.items():
        story_name = story["name"]

        # Get checklists (tasks) for this story
        checklists = story.get("checklists", [])

        for checklist in checklists:
            checklist_id = checklist["id"]

            for item in checklist.get("items", []):
                item_id = item["id"]
                item_name = item["name"]

                # Clean up task name (remove description suffix if present)
                task_name = item_name.split(" - ")[0] if " - " in item_name else item_name

                # Build lookup key
                lookup_key = f"{story_name}|{task_name}"

                if lookup_key in descriptions:
                    detailed_desc = descriptions[lookup_key]

                    if dry_run:
                        print(f"\n  Would update: {story_name} > {task_name}")
                        print(f"    Description length: {len(detailed_desc)} chars")
                    else:
                        # Update the checklist item with full name including description preview
                        # Note: ClickUp checklist items don't have full description field,
                        # so we need to update the parent story description instead
                        pass

                    updates_made += 1
                else:
                    updates_skipped += 1
                    if DEBUG:
                        print(f"  No description for: {lookup_key}")

        # Update the story description with all task details
        story_desc_parts = [story.get("description", "")]

        for checklist in checklists:
            for item in checklist.get("items", []):
                task_name = item["name"].split(" - ")[0]
                lookup_key = f"{story_name}|{task_name}"

                if lookup_key in descriptions:
                    story_desc_parts.append(f"\n\n---\n\n## Task: {task_name}\n\n{descriptions[lookup_key]}")

        if len(story_desc_parts) > 1:
            full_description = "\n".join(story_desc_parts)

            if dry_run:
                print(f"\n  Would update story description: {story_name}")
                print(f"    New description length: {len(full_description)} chars")
            else:
                try:
                    client.update_task(story_id, {"description": full_description})
                    print(f"  Updated: {story_name}")
                    time.sleep(0.3)  # Rate limiting
                except Exception as e:
                    print(f"  ERROR updating {story_name}: {e}")

    print("\n" + "=" * 60)
    print("Update Complete!")
    print("=" * 60)
    print(f"\n  Tasks with descriptions: {updates_made}")
    print(f"  Tasks without descriptions: {updates_skipped}")

    if dry_run:
        print("\n  DRY RUN - No changes were made")
        print("  Run without --dry-run to apply changes")


# ============================================================================
# CLI Entry Point
# ============================================================================

def main():
    global DEBUG

    parser = argparse.ArgumentParser(
        description="Update ClickUp tasks with detailed descriptions",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )

    parser.add_argument(
        "--api-key",
        required=True,
        help="ClickUp API key"
    )

    parser.add_argument(
        "--team-id",
        required=True,
        help="ClickUp Team/Workspace ID"
    )

    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Preview changes without applying"
    )

    parser.add_argument(
        "--debug",
        action="store_true",
        help="Enable debug output"
    )

    args = parser.parse_args()

    if args.debug:
        DEBUG = True

    try:
        update_tasks(args.api_key, args.team_id, args.dry_run)
    except KeyboardInterrupt:
        print("\n\nCancelled by user")
        sys.exit(1)
    except Exception as e:
        print(f"\nError: {e}")
        if DEBUG:
            import traceback
            traceback.print_exc()
        sys.exit(1)


if __name__ == "__main__":
    main()
