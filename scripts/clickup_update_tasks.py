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

    # Story 1.2: Proper gRPC Middleware
    descriptions["Story 1.2: Proper gRPC Middleware|Implement auth interceptor properly"] = """
## Description
Create a proper authentication interceptor that validates JWT tokens before any RPC handler runs.

## Steps to Implement
1. Create interceptor that extracts Authorization header
2. Validate JWT token using JWKS
3. Extract user claims and add to context
4. Return Unauthenticated error if invalid

## Example
```go
func AuthInterceptor() connect.UnaryInterceptorFunc {
    return func(next connect.UnaryFunc) connect.UnaryFunc {
        return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
            token := req.Header().Get("Authorization")
            user, err := validateToken(token)
            if err != nil {
                return nil, connect.NewError(connect.CodeUnauthenticated, err)
            }
            ctx = context.WithValue(ctx, userKey{}, user)
            return next(ctx, req)
        }
    }
}
```

## Acceptance Criteria
- [ ] All RPCs require valid JWT
- [ ] Invalid tokens return CodeUnauthenticated
- [ ] User info available in handler context
- [ ] Works for both unary and streaming RPCs
"""

    descriptions["Story 1.2: Proper gRPC Middleware|Add request ID middleware"] = """
## Description
Add a request ID to every request for distributed tracing and debugging.

## Steps to Implement
1. Check for existing X-Request-ID header
2. Generate UUID if not present
3. Add to context and response headers
4. Include in all log messages

## Example
```go
func RequestIDInterceptor() connect.UnaryInterceptorFunc {
    return func(next connect.UnaryFunc) connect.UnaryFunc {
        return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
            requestID := req.Header().Get("X-Request-ID")
            if requestID == "" {
                requestID = uuid.NewString()
            }
            ctx = context.WithValue(ctx, requestIDKey{}, requestID)
            resp, err := next(ctx, req)
            if resp != nil {
                resp.Header().Set("X-Request-ID", requestID)
            }
            return resp, err
        }
    }
}
```

## Acceptance Criteria
- [ ] Every request has a request ID
- [ ] Request ID in response headers
- [ ] Request ID in all related logs
- [ ] Existing request IDs preserved
"""

    descriptions["Story 1.2: Proper gRPC Middleware|Add logging middleware"] = """
## Description
Log all RPC calls with timing, status, and relevant context.

## Steps to Implement
1. Record start time before calling handler
2. Call handler and capture result/error
3. Log method, duration, status code, request ID
4. Use structured logging (JSON)

## Log Fields
- `method`: RPC method name
- `duration_ms`: Request duration
- `status`: gRPC status code
- `request_id`: Trace ID
- `user_id`: Authenticated user (if available)
- `error`: Error message (if failed)

## Acceptance Criteria
- [ ] All RPCs logged
- [ ] Duration captured accurately
- [ ] Error details included on failure
- [ ] Log level configurable (debug shows more)
"""

    descriptions["Story 1.2: Proper gRPC Middleware|Add panic recovery middleware"] = """
## Description
Recover from panics in handlers to prevent server crashes.

## Steps to Implement
1. Wrap handler call in defer/recover
2. Log panic with stack trace
3. Return Internal error to client
4. Continue serving other requests

## Example
```go
func RecoveryInterceptor() connect.UnaryInterceptorFunc {
    return func(next connect.UnaryFunc) connect.UnaryFunc {
        return func(ctx context.Context, req connect.AnyRequest) (resp connect.AnyResponse, err error) {
            defer func() {
                if r := recover(); r != nil {
                    log.Printf("Panic recovered: %v\\n%s", r, debug.Stack())
                    err = connect.NewError(connect.CodeInternal, fmt.Errorf("internal error"))
                }
            }()
            return next(ctx, req)
        }
    }
}
```

## Acceptance Criteria
- [ ] Panics don't crash server
- [ ] Stack trace logged
- [ ] Client receives Internal error
- [ ] Other requests unaffected
"""

    descriptions["Story 1.2: Proper gRPC Middleware|Add rate limiting middleware (per-user)"] = """
## Description
Implement rate limiting to prevent abuse and ensure fair usage.

## Steps to Implement
1. Use token bucket or sliding window algorithm
2. Key rate limits by user ID
3. Return ResourceExhausted when exceeded
4. Include Retry-After header

## Example
```go
func RateLimitInterceptor(limiter RateLimiter) connect.UnaryInterceptorFunc {
    return func(next connect.UnaryFunc) connect.UnaryFunc {
        return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
            userID := getUserFromContext(ctx)
            if !limiter.Allow(userID) {
                return nil, connect.NewError(connect.CodeResourceExhausted,
                    fmt.Errorf("rate limit exceeded"))
            }
            return next(ctx, req)
        }
    }
}
```

## Acceptance Criteria
- [ ] Rate limits enforced per user
- [ ] Returns ResourceExhausted (429)
- [ ] Configurable limits per endpoint
- [ ] Limits stored in Redis for distributed setup
"""

    descriptions["Story 1.2: Proper gRPC Middleware|Wire middleware chain correctly"] = """
## Description
Ensure middleware executes in the correct order.

## Correct Order (outside to inside)
1. Recovery (outermost - catches all panics)
2. Request ID (early - needed for logging)
3. Logging (sees all requests)
4. Rate Limiting (before auth to limit unauthenticated spam)
5. Authentication (validates user)
6. Handler (innermost)

## Example
```go
interceptors := connect.WithInterceptors(
    RecoveryInterceptor(),
    RequestIDInterceptor(),
    LoggingInterceptor(),
    RateLimitInterceptor(limiter),
    AuthInterceptor(),
)

path, handler := chatv1connect.NewChatServiceHandler(server, interceptors)
```

## Acceptance Criteria
- [ ] Middleware order documented
- [ ] Recovery is outermost
- [ ] Auth runs after rate limiting
- [ ] All middleware tested together
"""

    # Story 1.3: Enhanced Channel Management
    descriptions["Story 1.3: Enhanced Channel Management|Add CreateChannel RPC"] = """
## Description
Implement RPC to create new chat channels.

## Proto Definition
```protobuf
rpc CreateChannel(CreateChannelRequest) returns (CreateChannelResponse);

message CreateChannelRequest {
    string name = 1;
    ChannelType type = 2;
    bool is_private = 3;
}

message CreateChannelResponse {
    Channel channel = 1;
}
```

## Implementation
1. Validate channel name (length, characters)
2. Check user has permission to create
3. Generate unique channel ID
4. Store in repository
5. Return created channel

## Acceptance Criteria
- [ ] Channel created with unique ID
- [ ] Name validation (3-50 chars, alphanumeric)
- [ ] Duplicate names handled gracefully
- [ ] Creator automatically added as admin
"""

    descriptions["Story 1.3: Enhanced Channel Management|Add DeleteChannel RPC"] = """
## Description
Implement RPC to delete channels.

## Proto Definition
```protobuf
rpc DeleteChannel(DeleteChannelRequest) returns (DeleteChannelResponse);

message DeleteChannelRequest {
    string channel_id = 1;
}
```

## Implementation
1. Verify channel exists
2. Check user has admin permission
3. Notify all members of deletion
4. Remove from repository
5. Clean up related data (messages optional)

## Acceptance Criteria
- [ ] Only admins can delete
- [ ] Members notified before deletion
- [ ] Channel removed from all users' lists
- [ ] Returns NotFound if channel doesn't exist
"""

    descriptions["Story 1.3: Enhanced Channel Management|Add ListChannels RPC"] = """
## Description
Implement RPC to list available channels.

## Proto Definition
```protobuf
rpc ListChannels(ListChannelsRequest) returns (ListChannelsResponse);

message ListChannelsRequest {
    ChannelType type = 1;  // Filter by type
    int32 limit = 2;
    string cursor = 3;
}

message ListChannelsResponse {
    repeated Channel channels = 1;
    string next_cursor = 2;
}
```

## Implementation
1. Apply type filter if specified
2. Filter out private channels user can't see
3. Return paginated results
4. Include member count in response

## Acceptance Criteria
- [ ] Pagination works correctly
- [ ] Private channels filtered appropriately
- [ ] Type filter works
- [ ] Returns empty list (not error) if none found
"""

    descriptions["Story 1.3: Enhanced Channel Management|Add GetChannel RPC"] = """
## Description
Implement RPC to get channel details.

## Proto Definition
```protobuf
rpc GetChannel(GetChannelRequest) returns (GetChannelResponse);

message GetChannelRequest {
    string channel_id = 1;
}

message GetChannelResponse {
    Channel channel = 1;
    int32 member_count = 2;
    bool is_member = 3;
}
```

## Acceptance Criteria
- [ ] Returns full channel details
- [ ] Includes membership status
- [ ] Returns NotFound for invalid ID
- [ ] Private channels return PermissionDenied if not member
"""

    descriptions["Story 1.3: Enhanced Channel Management|Add LeaveChannel RPC"] = """
## Description
Implement RPC for users to leave channels.

## Proto Definition
```protobuf
rpc LeaveChannel(LeaveChannelRequest) returns (LeaveChannelResponse);

message LeaveChannelRequest {
    string channel_id = 1;
}
```

## Implementation
1. Remove user from channel members
2. Unsubscribe from channel messages
3. Notify other members (optional)
4. Handle last admin leaving

## Acceptance Criteria
- [ ] User removed from channel
- [ ] No more messages received
- [ ] Cannot leave if last admin (must transfer first)
- [ ] Idempotent (leaving twice doesn't error)
"""

    descriptions["Story 1.3: Enhanced Channel Management|Add channel types (global, zone, guild, party, whisper)"] = """
## Description
Implement different channel types with distinct behaviors.

## Channel Types
```protobuf
enum ChannelType {
    CHANNEL_TYPE_UNSPECIFIED = 0;
    CHANNEL_TYPE_GLOBAL = 1;      // Server-wide, everyone can see
    CHANNEL_TYPE_ZONE = 2;        // Geographic area in game
    CHANNEL_TYPE_GUILD = 3;       // Guild members only
    CHANNEL_TYPE_PARTY = 4;       // Temporary group
    CHANNEL_TYPE_WHISPER = 5;     // Private 1:1
}
```

## Behaviors per Type
- **Global**: Anyone can join, persisted forever
- **Zone**: Auto-join/leave based on location
- **Guild**: Members only, persisted
- **Party**: Temporary, deleted when empty
- **Whisper**: Two users only, persisted

## Acceptance Criteria
- [ ] All types defined in proto
- [ ] Type-specific join/leave logic
- [ ] Type-specific persistence rules
- [ ] Type shown in channel info
"""

    descriptions["Story 1.3: Enhanced Channel Management|Add channel permissions model"] = """
## Description
Implement permission system for channel operations.

## Permission Levels
```go
type ChannelPermission int

const (
    PermissionRead ChannelPermission = 1 << iota
    PermissionWrite
    PermissionInvite
    PermissionKick
    PermissionAdmin
)
```

## Roles
- **Member**: Read + Write
- **Moderator**: Read + Write + Kick
- **Admin**: All permissions
- **Owner**: Admin + cannot be removed

## Acceptance Criteria
- [ ] Permissions checked on all operations
- [ ] Role-based permission assignment
- [ ] Owner cannot be demoted
- [ ] Permission errors return PermissionDenied
"""

    # Story 1.4: Message Enhancements
    descriptions["Story 1.4: Message Enhancements|Add message IDs (UUID v7 for time-ordering)"] = """
## Description
Generate unique, time-ordered IDs for all messages.

## Why UUID v7?
- Time-ordered (lexicographic sort = chronological)
- Unique across distributed systems
- No coordination required

## Implementation
```go
import "github.com/google/uuid"

func GenerateMessageID() string {
    return uuid.Must(uuid.NewV7()).String()
}
```

## Acceptance Criteria
- [ ] All messages have unique ID
- [ ] IDs are time-ordered
- [ ] ID assigned server-side (not client)
- [ ] Messages retrievable by ID
"""

    descriptions["Story 1.4: Message Enhancements|Add message timestamps (server-side)"] = """
## Description
Add server-generated timestamps to all messages.

## Fields
```protobuf
message Message {
    string id = 1;
    google.protobuf.Timestamp created_at = 2;  // Server time
    google.protobuf.Timestamp edited_at = 3;   // If edited
}
```

## Implementation
- Use server time, not client time
- Store as Unix nanoseconds internally
- Convert to Timestamp proto for API

## Acceptance Criteria
- [ ] All messages have created_at
- [ ] Timestamp is server time (UTC)
- [ ] edited_at set when message modified
- [ ] Timestamps accurate to millisecond
"""

    descriptions["Story 1.4: Message Enhancements|Add message types (chat, emote, system, etc.)"] = """
## Description
Support different message types for varied rendering.

## Message Types
```protobuf
enum MessageType {
    MESSAGE_TYPE_CHAT = 0;      // Normal chat message
    MESSAGE_TYPE_EMOTE = 1;     // /me action
    MESSAGE_TYPE_SYSTEM = 2;    // System notification
    MESSAGE_TYPE_WHISPER = 3;   // Private message
    MESSAGE_TYPE_ROLL = 4;      // Dice roll result
    MESSAGE_TYPE_LOOT = 5;      // Item drop announcement
}
```

## Acceptance Criteria
- [ ] Type field in Message proto
- [ ] Default is CHAT
- [ ] Client can render differently per type
- [ ] System messages not from users
"""

    descriptions["Story 1.4: Message Enhancements|Add message metadata (location, level, class)"] = """
## Description
Include game context in messages for richer display.

## Metadata Fields
```protobuf
message MessageMetadata {
    string zone_name = 1;       // "Elwynn Forest"
    int32 player_level = 2;     // 60
    string player_class = 3;    // "Warrior"
    string guild_name = 4;      // "Epic Guild"
    string guild_rank = 5;      // "Officer"
}
```

## Implementation
- Metadata provided by game server
- Cached per session (not sent every message)
- Optional fields (empty if not applicable)

## Acceptance Criteria
- [ ] Metadata included in messages
- [ ] Efficient (not resent every message)
- [ ] Missing metadata handled gracefully
- [ ] Client can display [60 Warrior] style badges
"""

    descriptions["Story 1.4: Message Enhancements|Add message validation (length, content)"] = """
## Description
Validate messages before accepting them.

## Validation Rules
```go
type MessageValidator struct {
    MaxLength     int  // 500 characters
    MinLength     int  // 1 character
    AllowLinks    bool // false in most channels
    AllowMentions bool // true
}

func (v *MessageValidator) Validate(content string) error {
    if len(content) < v.MinLength {
        return ErrMessageTooShort
    }
    if len(content) > v.MaxLength {
        return ErrMessageTooLong
    }
    // Check for forbidden patterns...
}
```

## Acceptance Criteria
- [ ] Max length enforced (500 chars default)
- [ ] Empty messages rejected
- [ ] Link policy enforced per channel
- [ ] Clear error messages returned
"""

    descriptions["Story 1.4: Message Enhancements|Add profanity filter hook point"] = """
## Description
Add extensible hook for content filtering.

## Interface
```go
type ContentFilter interface {
    // Filter checks and optionally modifies content
    // Returns filtered content and whether to block
    Filter(ctx context.Context, content string) (filtered string, block bool, err error)
}

type NoOpFilter struct{}

func (f NoOpFilter) Filter(ctx context.Context, content string) (string, bool, error) {
    return content, false, nil
}
```

## Implementation Points
1. Call filter before storing message
2. If block=true, reject message
3. If filtered != content, use filtered version
4. Log blocked messages for review

## Acceptance Criteria
- [ ] Filter interface defined
- [ ] NoOp filter as default
- [ ] Easy to plug in real filter
- [ ] Blocked messages logged
"""

    # Story 1.5: Connection Management
    descriptions["Story 1.5: Connection Management|Implement proper connection lifecycle"] = """
## Description
Manage connection state from connect to disconnect.

## Lifecycle States
```go
type ConnectionState int

const (
    StateConnecting ConnectionState = iota
    StateConnected
    StateDisconnecting
    StateDisconnected
)
```

## Events
1. **OnConnect**: Validate auth, create session, join default channels
2. **OnDisconnect**: Leave channels, clean up session, notify presence

## Acceptance Criteria
- [ ] Connection state tracked
- [ ] Clean disconnect handled
- [ ] Abrupt disconnect detected
- [ ] Resources cleaned up on disconnect
"""

    descriptions["Story 1.5: Connection Management|Add heartbeat/ping mechanism"] = """
## Description
Implement ping/pong to detect dead connections.

## Implementation Options
1. **Application-level**: Send Ping message every 30s
2. **HTTP/2 PING frames**: Use transport-level pings
3. **gRPC keepalive**: Configure keepalive parameters

## Configuration
```go
keepalive.ServerParameters{
    Time:    30 * time.Second,  // Ping every 30s
    Timeout: 10 * time.Second,  // Wait 10s for pong
}
```

## Acceptance Criteria
- [ ] Dead connections detected within 60s
- [ ] Pings don't overload network
- [ ] Configurable intervals
- [ ] Works through load balancers
"""

    descriptions["Story 1.5: Connection Management|Add connection timeout handling"] = """
## Description
Clean up connections that are idle or unresponsive.

## Timeout Types
1. **Idle timeout**: No messages for 5 minutes
2. **Auth timeout**: Must authenticate within 30s
3. **Ping timeout**: No pong within 10s

## Implementation
```go
type ConnectionManager struct {
    idleTimeout time.Duration
    authTimeout time.Duration
}

func (m *ConnectionManager) MonitorConnection(conn *Connection) {
    timer := time.NewTimer(m.authTimeout)
    select {
    case <-conn.Authenticated:
        timer.Stop()
    case <-timer.C:
        conn.Close("authentication timeout")
    }
}
```

## Acceptance Criteria
- [ ] Unauthenticated connections closed after 30s
- [ ] Idle connections closed after 5m
- [ ] Timeouts configurable
- [ ] Clean disconnect on timeout
"""

    descriptions["Story 1.5: Connection Management|Add reconnection support (client-side hint)"] = """
## Description
Support graceful reconnection after disconnect.

## Server Behavior
1. Keep session alive briefly after disconnect
2. Allow reconnect with session token
3. Replay missed messages if possible
4. Send reconnect hint in disconnect message

## Disconnect Message
```protobuf
message DisconnectNotification {
    string reason = 1;
    bool can_reconnect = 2;
    int32 reconnect_delay_ms = 3;
    string session_token = 4;  // For session resume
}
```

## Acceptance Criteria
- [ ] Client receives reconnect hint
- [ ] Session survives brief disconnect (30s)
- [ ] Reconnect reuses session
- [ ] Message gap handled gracefully
"""

    descriptions["Story 1.5: Connection Management|Add max connections per user limit"] = """
## Description
Limit concurrent connections per user to prevent abuse.

## Implementation
```go
type ConnectionLimiter struct {
    maxPerUser int
    connections map[string]int
    mu          sync.Mutex
}

func (l *ConnectionLimiter) TryConnect(userID string) bool {
    l.mu.Lock()
    defer l.mu.Unlock()

    if l.connections[userID] >= l.maxPerUser {
        return false
    }
    l.connections[userID]++
    return true
}
```

## Acceptance Criteria
- [ ] Default limit: 3 connections per user
- [ ] Oldest connection kicked when limit exceeded (optional)
- [ ] Limit configurable per user type
- [ ] Returns ResourceExhausted when limit hit
"""

    descriptions["Story 1.5: Connection Management|Add backpressure handling"] = """
## Description
Handle slow clients without blocking the server.

## Problem
A slow client can cause message buffer to grow unbounded.

## Solutions
1. **Bounded buffer**: Drop messages when buffer full
2. **Adaptive rate**: Slow down sends to slow clients
3. **Disconnect**: Kick very slow clients

## Implementation
```go
func (s *Server) SendToUser(userID string, msg *Message) {
    stream := s.streams[userID]
    select {
    case stream <- msg:
        // Sent successfully
    default:
        // Buffer full - drop or disconnect
        log.Warn("Dropping message for slow client", "user", userID)
    }
}
```

## Acceptance Criteria
- [ ] Slow clients don't block server
- [ ] Message drops logged
- [ ] Client notified of drops (optional)
- [ ] Very slow clients disconnected
"""

    # =========================================================================
    # Epic 2: Persistence Layer
    # =========================================================================

    descriptions["Story 2.1: Database Infrastructure Setup|Add ScyllaDB driver dependency (gocql)"] = """
## Description
Add the ScyllaDB/Cassandra driver to the project.

## Steps
1. Add dependency: `go get github.com/gocql/gocql`
2. Create connection helper in `internal/db/scylla.go`
3. Add configuration for hosts, keyspace, auth

## Connection Example
```go
cluster := gocql.NewCluster("scylla1", "scylla2", "scylla3")
cluster.Keyspace = "hermes"
cluster.Consistency = gocql.LocalQuorum
session, err := cluster.CreateSession()
```

## Acceptance Criteria
- [ ] gocql added to go.mod
- [ ] Can connect to ScyllaDB cluster
- [ ] Connection pooling configured
- [ ] Proper error handling
"""

    descriptions["Story 2.1: Database Infrastructure Setup|Add PostgreSQL driver dependency (pgx)"] = """
## Description
Add the PostgreSQL driver for metadata storage.

## Steps
1. Add dependency: `go get github.com/jackc/pgx/v5`
2. Create connection helper in `internal/db/postgres.go`
3. Use connection pool (pgxpool)

## Connection Example
```go
config, err := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
config.MaxConns = 20
pool, err := pgxpool.NewWithConfig(ctx, config)
```

## Acceptance Criteria
- [ ] pgx v5 added to go.mod
- [ ] Connection pool configured
- [ ] SSL mode configurable
- [ ] Connection string from env var
"""

    descriptions["Story 2.1: Database Infrastructure Setup|Create database connection manager"] = """
## Description
Create a unified connection manager for all databases.

## Interface
```go
type DBManager struct {
    Scylla   *gocql.Session
    Postgres *pgxpool.Pool
    Redis    *redis.Client
}

func NewDBManager(cfg *config.DatabaseConfig) (*DBManager, error)
func (m *DBManager) Close() error
func (m *DBManager) HealthCheck(ctx context.Context) error
```

## Acceptance Criteria
- [ ] Single struct manages all connections
- [ ] Graceful shutdown closes all
- [ ] Health check for all databases
- [ ] Connection retry on startup
"""

    descriptions["Story 2.1: Database Infrastructure Setup|Add database migration tooling (golang-migrate)"] = """
## Description
Set up database migrations for schema versioning.

## Steps
1. Add dependency: `go get github.com/golang-migrate/migrate/v4`
2. Create migrations directory: `migrations/postgres/`, `migrations/scylla/`
3. Add migrate commands to Makefile

## Makefile Targets
```makefile
migrate-up:
    migrate -path migrations/postgres -database $(DATABASE_URL) up

migrate-down:
    migrate -path migrations/postgres -database $(DATABASE_URL) down 1

migrate-create:
    migrate create -ext sql -dir migrations/postgres $(name)
```

## Acceptance Criteria
- [ ] Migrations versioned in git
- [ ] Up/down migrations work
- [ ] Migration status visible
- [ ] CI runs migrations
"""

    descriptions["Story 2.1: Database Infrastructure Setup|Create Docker Compose for local development"] = """
## Description
Create Docker Compose file for local database setup.

## docker-compose.yml
```yaml
version: '3.8'
services:
  scylla:
    image: scylladb/scylla:5.2
    ports:
      - "9042:9042"

  postgres:
    image: postgres:15
    environment:
      POSTGRES_DB: hermes
      POSTGRES_USER: hermes
      POSTGRES_PASSWORD: hermes
    ports:
      - "5432:5432"

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
```

## Acceptance Criteria
- [ ] `docker-compose up` starts all databases
- [ ] Data persisted in volumes
- [ ] Health checks configured
- [ ] Works on Mac/Linux/Windows
"""

    descriptions["Story 2.1: Database Infrastructure Setup|Add database health check endpoint"] = """
## Description
Add /health endpoint that checks all database connections.

## Response Format
```json
{
  "status": "healthy",
  "checks": {
    "scylla": {"status": "up", "latency_ms": 2},
    "postgres": {"status": "up", "latency_ms": 1},
    "redis": {"status": "up", "latency_ms": 0}
  }
}
```

## Acceptance Criteria
- [ ] /health endpoint exists
- [ ] Checks all databases
- [ ] Returns 503 if any unhealthy
- [ ] Includes latency metrics
"""

    # Story 2.2: ScyllaDB Schema Design
    descriptions["Story 2.2: ScyllaDB Schema Design (Messages)|Design message table schema"] = """
## Description
Design ScyllaDB schema optimized for chat message storage.

## Schema
```sql
CREATE TABLE messages (
    channel_id text,
    bucket text,           -- Time bucket: "2024-01-15"
    message_id timeuuid,   -- Sorted by time within bucket
    user_id text,
    content text,
    message_type int,
    metadata map<text, text>,
    created_at timestamp,
    PRIMARY KEY ((channel_id, bucket), message_id)
) WITH CLUSTERING ORDER BY (message_id DESC);
```

## Query Patterns Supported
- Get recent messages: `SELECT * FROM messages WHERE channel_id = ? AND bucket = ? LIMIT 50`
- Get messages after ID: `SELECT * FROM messages WHERE channel_id = ? AND bucket = ? AND message_id > ?`

## Acceptance Criteria
- [ ] Schema supports time-range queries
- [ ] Efficient pagination
- [ ] Partition size bounded by bucket
- [ ] Query patterns documented
"""

    descriptions["Story 2.2: ScyllaDB Schema Design (Messages)|Design partition strategy (channel + time bucket)"] = """
## Description
Design partition key to ensure even data distribution.

## Strategy
- Partition by: `(channel_id, bucket)`
- Bucket format: `YYYY-MM-DD` (daily)
- High-traffic channels get more partitions naturally

## Bucket Calculation
```go
func getBucket(t time.Time) string {
    return t.Format("2006-01-02")
}

func getPartitionKey(channelID string, t time.Time) string {
    return channelID + ":" + getBucket(t)
}
```

## Acceptance Criteria
- [ ] No hot partitions
- [ ] Partition size < 100MB
- [ ] Easy to query recent messages
- [ ] Historical queries span buckets
"""

    descriptions["Story 2.2: ScyllaDB Schema Design (Messages)|Add TTL for message expiration"] = """
## Description
Configure automatic message expiration using ScyllaDB TTL.

## Implementation
```sql
-- Set TTL on insert (30 days = 2592000 seconds)
INSERT INTO messages (...) VALUES (...) USING TTL 2592000;

-- Or use default TTL on table
ALTER TABLE messages WITH default_time_to_live = 2592000;
```

## Configurable TTL per Channel Type
- Global chat: 7 days
- Zone chat: 1 day
- Whispers: 30 days
- Guild chat: 90 days

## Acceptance Criteria
- [ ] Messages auto-expire based on type
- [ ] TTL configurable per channel type
- [ ] Expiration doesn't impact reads
- [ ] Storage reclaimed automatically
"""

    descriptions["Story 2.2: ScyllaDB Schema Design (Messages)|Create materialized views if needed"] = """
## Description
Create materialized views for additional query patterns.

## Potential Views
```sql
-- Messages by user (for user history/moderation)
CREATE MATERIALIZED VIEW messages_by_user AS
    SELECT * FROM messages
    WHERE user_id IS NOT NULL AND channel_id IS NOT NULL AND bucket IS NOT NULL
    PRIMARY KEY (user_id, created_at, channel_id, bucket, message_id);
```

## When to Use
- Only if queries can't be served by main table
- MVs have consistency tradeoffs
- Consider denormalization instead

## Acceptance Criteria
- [ ] Views only created if needed
- [ ] Query patterns documented
- [ ] Consistency implications understood
- [ ] Alternative approaches evaluated
"""

    descriptions["Story 2.2: ScyllaDB Schema Design (Messages)|Write migration scripts"] = """
## Description
Create CQL migration scripts for ScyllaDB schema.

## Migration Files
```
migrations/scylla/
├── 001_create_keyspace.cql
├── 002_create_messages_table.cql
└── 003_create_messages_by_user_view.cql
```

## Example Migration
```sql
-- 001_create_keyspace.cql
CREATE KEYSPACE IF NOT EXISTS hermes
WITH replication = {
    'class': 'NetworkTopologyStrategy',
    'datacenter1': 3
};
```

## Acceptance Criteria
- [ ] Migrations idempotent (IF NOT EXISTS)
- [ ] Order enforced by numbering
- [ ] Can run on fresh cluster
- [ ] Documented rollback procedure
"""

    descriptions["Story 2.2: ScyllaDB Schema Design (Messages)|Add schema documentation"] = """
## Description
Document the ScyllaDB schema and usage patterns.

## Documentation Contents
1. Entity-relationship diagram
2. Table schemas with column descriptions
3. Partition key rationale
4. Query patterns and examples
5. TTL configuration
6. Consistency level recommendations

## Acceptance Criteria
- [ ] Schema diagram in docs/
- [ ] All tables documented
- [ ] Query examples provided
- [ ] Performance characteristics noted
"""

    # Story 2.3: PostgreSQL Schema Design
    descriptions["Story 2.3: PostgreSQL Schema Design (Metadata)|Design channels table"] = """
## Description
Create PostgreSQL table for channel metadata.

## Schema
```sql
CREATE TABLE channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(50) NOT NULL,
    type VARCHAR(20) NOT NULL,
    created_by UUID REFERENCES users(id),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    is_private BOOLEAN DEFAULT FALSE,
    metadata JSONB DEFAULT '{}'
);

CREATE INDEX idx_channels_type ON channels(type);
CREATE INDEX idx_channels_created_by ON channels(created_by);
```

## Acceptance Criteria
- [ ] UUID primary key
- [ ] Type enum enforced
- [ ] JSONB for flexible metadata
- [ ] Proper indexes for queries
"""

    descriptions["Story 2.3: PostgreSQL Schema Design (Metadata)|Design channel_members table"] = """
## Description
Create table to track channel membership.

## Schema
```sql
CREATE TABLE channel_members (
    channel_id UUID REFERENCES channels(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    role VARCHAR(20) DEFAULT 'member',
    joined_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (channel_id, user_id)
);

CREATE INDEX idx_channel_members_user ON channel_members(user_id);
```

## Acceptance Criteria
- [ ] Composite primary key
- [ ] Cascade delete on channel removal
- [ ] Role field for permissions
- [ ] Index for user's channels query
"""

    descriptions["Story 2.3: PostgreSQL Schema Design (Metadata)|Design user_preferences table"] = """
## Description
Store per-user chat preferences.

## Schema
```sql
CREATE TABLE user_preferences (
    user_id UUID PRIMARY KEY,
    muted_channels UUID[] DEFAULT '{}',
    notification_level VARCHAR(20) DEFAULT 'all',
    show_timestamps BOOLEAN DEFAULT TRUE,
    compact_mode BOOLEAN DEFAULT FALSE,
    preferences JSONB DEFAULT '{}',
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

## Acceptance Criteria
- [ ] One row per user
- [ ] Array type for muted channels
- [ ] JSONB for extensibility
- [ ] Sensible defaults
"""

    descriptions["Story 2.3: PostgreSQL Schema Design (Metadata)|Design blocked_users table"] = """
## Description
Track user blocks for filtering messages.

## Schema
```sql
CREATE TABLE blocked_users (
    blocker_id UUID NOT NULL,
    blocked_id UUID NOT NULL,
    blocked_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (blocker_id, blocked_id)
);

CREATE INDEX idx_blocked_users_blocked ON blocked_users(blocked_id);
```

## Usage
- Check before delivering message
- Filter blocked users from presence
- Prevent whispers from blocked users

## Acceptance Criteria
- [ ] Bidirectional lookup efficient
- [ ] Block is one-way (A blocks B, B can still see A unless B also blocks)
- [ ] Used in message filtering
"""

    descriptions["Story 2.3: PostgreSQL Schema Design (Metadata)|Add indexes for common queries"] = """
## Description
Add indexes to optimize common query patterns.

## Common Queries
1. Get user's channels
2. Get channel members
3. Check if user is member
4. Get user's blocked list

## Index Additions
```sql
-- Partial index for admins only
CREATE INDEX idx_channel_members_admins
ON channel_members(channel_id)
WHERE role = 'admin';

-- GIN index for array search
CREATE INDEX idx_user_prefs_muted
ON user_preferences USING GIN(muted_channels);
```

## Acceptance Criteria
- [ ] All common queries use indexes
- [ ] EXPLAIN ANALYZE verified
- [ ] No sequential scans on large tables
- [ ] Index bloat monitored
"""

    descriptions["Story 2.3: PostgreSQL Schema Design (Metadata)|Write migration scripts"] = """
## Description
Create SQL migration files for PostgreSQL.

## Migration Files
```
migrations/postgres/
├── 000001_create_channels.up.sql
├── 000001_create_channels.down.sql
├── 000002_create_channel_members.up.sql
├── 000002_create_channel_members.down.sql
└── ...
```

## Example
```sql
-- 000001_create_channels.up.sql
CREATE TABLE channels (...);
CREATE INDEX ...;

-- 000001_create_channels.down.sql
DROP TABLE IF EXISTS channels;
```

## Acceptance Criteria
- [ ] Up and down migrations for each
- [ ] Tested rollback works
- [ ] Data-safe migrations (no data loss)
- [ ] Run in CI before deploy
"""

    # Story 2.4: Repository Implementations
    descriptions["Story 2.4: Repository Implementations|Implement ScyllaMessageRepository"] = """
## Description
Implement MessageRepository interface using ScyllaDB.

## Implementation
```go
type ScyllaMessageRepository struct {
    session *gocql.Session
}

func (r *ScyllaMessageRepository) Save(ctx context.Context, msg *Message) error {
    bucket := getBucket(msg.CreatedAt)
    return r.session.Query(`
        INSERT INTO messages (channel_id, bucket, message_id, user_id, content, created_at)
        VALUES (?, ?, ?, ?, ?, ?)
        USING TTL ?
    `, msg.ChannelID, bucket, gocql.TimeUUID(), msg.UserID, msg.Content, msg.CreatedAt, msg.TTL).Exec()
}
```

## Acceptance Criteria
- [ ] All interface methods implemented
- [ ] Proper error handling
- [ ] Context cancellation respected
- [ ] TTL applied on insert
"""

    descriptions["Story 2.4: Repository Implementations|Implement PostgresChannelRepository"] = """
## Description
Implement ChannelRepository interface using PostgreSQL.

## Implementation
```go
type PostgresChannelRepository struct {
    pool *pgxpool.Pool
}

func (r *PostgresChannelRepository) Get(ctx context.Context, id string) (*Channel, error) {
    var ch Channel
    err := r.pool.QueryRow(ctx, `
        SELECT id, name, type, created_by, created_at, is_private
        FROM channels WHERE id = $1
    `, id).Scan(&ch.ID, &ch.Name, &ch.Type, &ch.CreatedBy, &ch.CreatedAt, &ch.IsPrivate)
    if err == pgx.ErrNoRows {
        return nil, ErrChannelNotFound
    }
    return &ch, err
}
```

## Acceptance Criteria
- [ ] All interface methods implemented
- [ ] Uses prepared statements
- [ ] Proper error mapping
- [ ] Transaction support where needed
"""

    descriptions["Story 2.4: Repository Implementations|Add write-through caching layer"] = """
## Description
Add caching to reduce database load for frequent reads.

## Strategy
```go
type CachedChannelRepository struct {
    repo  ChannelRepository
    cache *redis.Client
    ttl   time.Duration
}

func (r *CachedChannelRepository) Get(ctx context.Context, id string) (*Channel, error) {
    // Try cache first
    if cached, err := r.cache.Get(ctx, "channel:"+id).Result(); err == nil {
        var ch Channel
        json.Unmarshal([]byte(cached), &ch)
        return &ch, nil
    }

    // Cache miss - fetch from DB
    ch, err := r.repo.Get(ctx, id)
    if err != nil {
        return nil, err
    }

    // Write to cache
    data, _ := json.Marshal(ch)
    r.cache.Set(ctx, "channel:"+id, data, r.ttl)
    return ch, nil
}
```

## Acceptance Criteria
- [ ] Cache hit rate > 90% for hot channels
- [ ] Cache invalidation on update
- [ ] TTL prevents stale data
- [ ] Fallback to DB on cache failure
"""

    descriptions["Story 2.4: Repository Implementations|Add batch write support"] = """
## Description
Support efficient bulk inserts for high-throughput scenarios.

## Implementation
```go
func (r *ScyllaMessageRepository) SaveBatch(ctx context.Context, msgs []*Message) error {
    batch := r.session.NewBatch(gocql.LoggedBatch)

    for _, msg := range msgs {
        bucket := getBucket(msg.CreatedAt)
        batch.Query(`
            INSERT INTO messages (channel_id, bucket, message_id, ...)
            VALUES (?, ?, ?, ...)
        `, msg.ChannelID, bucket, ...)
    }

    return r.session.ExecuteBatch(batch)
}
```

## Use Cases
- Message history import
- Bulk message deletion
- High-traffic channel batching

## Acceptance Criteria
- [ ] Batch size configurable
- [ ] Atomic batch execution
- [ ] Error handling for partial failures
- [ ] Performance benchmarked
"""

    descriptions["Story 2.4: Repository Implementations|Add repository integration tests"] = """
## Description
Write integration tests that run against real databases.

## Test Setup
```go
//go:build integration

func TestScyllaMessageRepository(t *testing.T) {
    ctx := context.Background()

    // Start container
    container, _ := scylla.RunContainer(ctx)
    defer container.Terminate(ctx)

    // Create repository
    session := createSession(container)
    repo := NewScyllaMessageRepository(session)

    // Run tests
    t.Run("Save and Get", func(t *testing.T) {
        msg := &Message{Content: "test"}
        require.NoError(t, repo.Save(ctx, msg))
        // ...
    })
}
```

## Acceptance Criteria
- [ ] Tests use testcontainers
- [ ] Each test gets clean database
- [ ] Tests run in CI
- [ ] Covers all repository methods
"""

    descriptions["Story 2.4: Repository Implementations|Add repository benchmarks"] = """
## Description
Create benchmarks to establish performance baselines.

## Benchmark Tests
```go
func BenchmarkScyllaMessageRepository_Save(b *testing.B) {
    repo := setupRepo()
    msg := &Message{Content: "benchmark message"}

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        repo.Save(context.Background(), msg)
    }
}

func BenchmarkScyllaMessageRepository_GetHistory(b *testing.B) {
    repo := setupRepoWithData(1000)

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        repo.GetHistory(context.Background(), "channel-1", HistoryOptions{Limit: 50})
    }
}
```

## Acceptance Criteria
- [ ] Benchmarks for all critical paths
- [ ] Results tracked over time
- [ ] Run: `go test -bench=. ./...`
- [ ] Performance regression alerts
"""

    # Story 2.5: Message History API
    descriptions["Story 2.5: Message History API|Add GetMessageHistory RPC"] = """
## Description
Implement RPC to fetch historical messages.

## Proto
```protobuf
rpc GetMessageHistory(GetMessageHistoryRequest) returns (GetMessageHistoryResponse);

message GetMessageHistoryRequest {
    string channel_id = 1;
    int32 limit = 2;           // Max 100
    string cursor = 3;         // Pagination cursor
    Direction direction = 4;   // NEWER or OLDER
}

message GetMessageHistoryResponse {
    repeated Message messages = 1;
    string next_cursor = 2;
    bool has_more = 3;
}
```

## Acceptance Criteria
- [ ] Returns messages in chronological order
- [ ] Cursor-based pagination works
- [ ] Limit enforced (max 100)
- [ ] Includes sender info in messages
"""

    descriptions["Story 2.5: Message History API|Implement cursor-based pagination"] = """
## Description
Implement efficient cursor-based pagination for message history.

## Cursor Format
```go
// Cursor encodes position: "channel_id:bucket:message_id"
type Cursor struct {
    ChannelID string
    Bucket    string
    MessageID gocql.UUID
}

func EncodeCursor(c Cursor) string {
    return base64.StdEncoding.EncodeToString(
        []byte(fmt.Sprintf("%s:%s:%s", c.ChannelID, c.Bucket, c.MessageID)))
}

func DecodeCursor(s string) (Cursor, error) {
    // Decode and parse...
}
```

## Query with Cursor
```sql
SELECT * FROM messages
WHERE channel_id = ? AND bucket = ? AND message_id < ?
ORDER BY message_id DESC
LIMIT ?
```

## Acceptance Criteria
- [ ] Cursor is opaque to client
- [ ] Works across bucket boundaries
- [ ] No skipped messages
- [ ] Efficient (uses clustering order)
"""

    descriptions["Story 2.5: Message History API|Add time-range filtering"] = """
## Description
Allow fetching messages within a time range.

## Request Fields
```protobuf
message GetMessageHistoryRequest {
    // ... existing fields ...
    google.protobuf.Timestamp after = 5;   // Messages after this time
    google.protobuf.Timestamp before = 6;  // Messages before this time
}
```

## Implementation
```go
func (r *ScyllaMessageRepository) GetHistory(ctx context.Context, channelID string, opts HistoryOptions) ([]*Message, error) {
    // Calculate which buckets to query based on time range
    buckets := getBucketsBetween(opts.After, opts.Before)

    var messages []*Message
    for _, bucket := range buckets {
        // Query each bucket...
    }
    return messages, nil
}
```

## Acceptance Criteria
- [ ] Can filter by start time
- [ ] Can filter by end time
- [ ] Both combined works
- [ ] Efficient bucket selection
"""

    descriptions["Story 2.5: Message History API|Add message search (optional)"] = """
## Description
Add full-text search for messages (optional feature).

## Options
1. **Elasticsearch**: Best for full-text search
2. **PostgreSQL FTS**: If already using Postgres
3. **Application-level**: For simple keyword match

## Proto
```protobuf
rpc SearchMessages(SearchMessagesRequest) returns (SearchMessagesResponse);

message SearchMessagesRequest {
    string query = 1;
    string channel_id = 2;      // Optional filter
    int32 limit = 3;
}
```

## Acceptance Criteria
- [ ] Search implementation chosen
- [ ] Basic keyword search works
- [ ] Results ranked by relevance
- [ ] Rate limited to prevent abuse
"""

    descriptions["Story 2.5: Message History API|Add rate limiting for history requests"] = """
## Description
Rate limit history requests to prevent abuse.

## Limits
- 10 requests per second per user
- 100 requests per minute per user
- Burst of 5 allowed

## Implementation
```go
func (s *Server) GetMessageHistory(ctx context.Context, req *Request) (*Response, error) {
    userID := getUserFromContext(ctx)

    if !s.historyLimiter.Allow(userID) {
        return nil, connect.NewError(connect.CodeResourceExhausted,
            errors.New("too many history requests"))
    }

    // Process request...
}
```

## Acceptance Criteria
- [ ] Rate limits enforced
- [ ] Clear error message
- [ ] Limits configurable
- [ ] Separate from message send limits
"""

    descriptions["Story 2.5: Message History API|Cache recent messages in Redis"] = """
## Description
Cache recent messages for fast initial load.

## Cache Strategy
```go
// Cache last N messages per channel
const RecentMessagesCacheSize = 100

func (c *MessageCache) GetRecent(ctx context.Context, channelID string) ([]*Message, error) {
    key := fmt.Sprintf("messages:recent:%s", channelID)

    // Get from sorted set (scored by timestamp)
    results, err := c.redis.ZRevRangeWithScores(ctx, key, 0, RecentMessagesCacheSize-1).Result()
    // ...
}

func (c *MessageCache) AddMessage(ctx context.Context, msg *Message) error {
    key := fmt.Sprintf("messages:recent:%s", msg.ChannelID)

    // Add to sorted set with timestamp score
    c.redis.ZAdd(ctx, key, redis.Z{
        Score:  float64(msg.CreatedAt.UnixNano()),
        Member: msg.Serialize(),
    })

    // Trim to max size
    c.redis.ZRemRangeByRank(ctx, key, 0, -RecentMessagesCacheSize-1)
}
```

## Acceptance Criteria
- [ ] Initial load from cache (< 10ms)
- [ ] Cache updated on new messages
- [ ] Falls back to DB on miss
- [ ] TTL expires inactive channels
"""

    # =========================================================================
    # Epic 3: Distributed Messaging
    # =========================================================================

    descriptions["Story 3.1: Redis Infrastructure|Add Redis driver dependency (go-redis)"] = """
## Description
Add the Redis client library to the project.

## Steps
1. Add dependency: `go get github.com/redis/go-redis/v9`
2. Create connection helper in `internal/db/redis.go`
3. Configure for standalone and cluster modes

## Connection Example
```go
import "github.com/redis/go-redis/v9"

func NewRedisClient(cfg *RedisConfig) *redis.Client {
    return redis.NewClient(&redis.Options{
        Addr:     cfg.Addr,
        Password: cfg.Password,
        DB:       cfg.DB,
        PoolSize: cfg.PoolSize,
    })
}
```

## Acceptance Criteria
- [ ] go-redis v9 added to go.mod
- [ ] Connection pool configured
- [ ] Works with Redis 7+
- [ ] Supports Redis Cluster
"""

    descriptions["Story 3.1: Redis Infrastructure|Create Redis connection manager"] = """
## Description
Create a manager that handles Redis connections with failover.

## Features
- Connection pooling
- Automatic reconnection
- Health checks
- Cluster support

## Implementation
```go
type RedisManager struct {
    client redis.UniversalClient
}

func NewRedisManager(cfg *RedisConfig) (*RedisManager, error) {
    var client redis.UniversalClient

    if cfg.ClusterMode {
        client = redis.NewClusterClient(&redis.ClusterOptions{
            Addrs: cfg.Addrs,
        })
    } else {
        client = redis.NewClient(&redis.Options{
            Addr: cfg.Addrs[0],
        })
    }

    // Verify connection
    if err := client.Ping(context.Background()).Err(); err != nil {
        return nil, err
    }

    return &RedisManager{client: client}, nil
}
```

## Acceptance Criteria
- [ ] Single client or cluster mode
- [ ] Connection pool tuned
- [ ] Reconnection on failure
- [ ] Metrics exposed
"""

    descriptions["Story 3.1: Redis Infrastructure|Add Redis to Docker Compose"] = """
## Description
Add Redis to local development environment.

## docker-compose.yml Addition
```yaml
redis:
  image: redis:7-alpine
  command: redis-server --appendonly yes
  ports:
    - "6379:6379"
  volumes:
    - redis-data:/data
  healthcheck:
    test: ["CMD", "redis-cli", "ping"]
    interval: 10s
    timeout: 5s
    retries: 5

volumes:
  redis-data:
```

## Acceptance Criteria
- [ ] Redis starts with docker-compose
- [ ] Data persisted in volume
- [ ] Health check configured
- [ ] Accessible on localhost:6379
"""

    descriptions["Story 3.1: Redis Infrastructure|Add Redis Cluster configuration"] = """
## Description
Configure for Redis Cluster in production.

## Configuration
```go
type RedisConfig struct {
    Addrs       []string      // Multiple nodes for cluster
    Password    string
    ClusterMode bool
    PoolSize    int
    MaxRetries  int
    ReadTimeout time.Duration
}

// Environment variables
// REDIS_ADDRS=redis-1:6379,redis-2:6379,redis-3:6379
// REDIS_CLUSTER_MODE=true
```

## Acceptance Criteria
- [ ] Cluster mode configurable
- [ ] Multiple nodes supported
- [ ] Automatic slot routing
- [ ] Failover handled
"""

    descriptions["Story 3.1: Redis Infrastructure|Add Redis health check"] = """
## Description
Include Redis in application health checks.

## Implementation
```go
func (m *RedisManager) HealthCheck(ctx context.Context) error {
    start := time.Now()
    err := m.client.Ping(ctx).Err()
    latency := time.Since(start)

    if err != nil {
        return fmt.Errorf("redis unhealthy: %w", err)
    }

    log.Debug("redis health check", "latency_ms", latency.Milliseconds())
    return nil
}
```

## Acceptance Criteria
- [ ] Ping checks connectivity
- [ ] Latency measured
- [ ] Returns error on failure
- [ ] Included in /health endpoint
"""

    descriptions["Story 3.1: Redis Infrastructure|Add Redis connection retry logic"] = """
## Description
Implement retry logic for transient Redis failures.

## Implementation
```go
func (m *RedisManager) ExecuteWithRetry(ctx context.Context, fn func() error) error {
    var lastErr error
    for i := 0; i < m.maxRetries; i++ {
        if err := fn(); err != nil {
            lastErr = err
            if isRetryable(err) {
                time.Sleep(backoff(i))
                continue
            }
            return err
        }
        return nil
    }
    return fmt.Errorf("max retries exceeded: %w", lastErr)
}

func backoff(attempt int) time.Duration {
    return time.Duration(math.Pow(2, float64(attempt))) * 100 * time.Millisecond
}
```

## Acceptance Criteria
- [ ] Retries on connection errors
- [ ] Exponential backoff
- [ ] Max retries configurable
- [ ] Non-retryable errors fail fast
"""

    # Story 3.2: Pub/Sub Message Broadcaster
    descriptions["Story 3.2: Pub/Sub Message Broadcaster|Implement RedisMessageBroadcaster"] = """
## Description
Implement message broadcasting using Redis Pub/Sub.

## Implementation
```go
type RedisMessageBroadcaster struct {
    client    redis.UniversalClient
    pubsub    *redis.PubSub
    handlers  map[string][]MessageHandler
    mu        sync.RWMutex
}

func (b *RedisMessageBroadcaster) Broadcast(ctx context.Context, channelID string, msg *Message) error {
    data, err := proto.Marshal(msg)
    if err != nil {
        return err
    }

    redisChannel := fmt.Sprintf("chat:%s", channelID)
    return b.client.Publish(ctx, redisChannel, data).Err()
}

func (b *RedisMessageBroadcaster) Subscribe(ctx context.Context, channelID string, handler MessageHandler) error {
    redisChannel := fmt.Sprintf("chat:%s", channelID)
    b.pubsub.Subscribe(ctx, redisChannel)

    b.mu.Lock()
    b.handlers[channelID] = append(b.handlers[channelID], handler)
    b.mu.Unlock()

    return nil
}
```

## Acceptance Criteria
- [ ] Messages published to Redis channel
- [ ] Subscribers receive messages
- [ ] Works across server instances
- [ ] Protobuf serialization
"""

    descriptions["Story 3.2: Pub/Sub Message Broadcaster|Design channel naming convention"] = """
## Description
Define consistent Redis channel naming.

## Naming Convention
```
chat:{channel_id}           # Channel messages
presence:{user_id}          # User presence updates
system:broadcast            # Server-wide announcements
guild:{guild_id}:events     # Guild events
user:{user_id}:notifications # Direct user notifications
```

## Key Patterns
```go
const (
    ChatChannelPrefix     = "chat:"
    PresenceChannelPrefix = "presence:"
    SystemChannel         = "system:broadcast"
)

func ChatChannel(channelID string) string {
    return ChatChannelPrefix + channelID
}
```

## Acceptance Criteria
- [ ] Consistent naming documented
- [ ] No collisions possible
- [ ] Easy to debug (grep-able)
- [ ] Namespace isolation
"""

    descriptions["Story 3.2: Pub/Sub Message Broadcaster|Handle subscription management"] = """
## Description
Manage pub/sub subscriptions lifecycle.

## Implementation
```go
func (b *RedisMessageBroadcaster) startListener(ctx context.Context) {
    ch := b.pubsub.Channel()

    for {
        select {
        case <-ctx.Done():
            return
        case msg := <-ch:
            b.handleMessage(msg)
        }
    }
}

func (b *RedisMessageBroadcaster) handleMessage(msg *redis.Message) {
    channelID := strings.TrimPrefix(msg.Channel, ChatChannelPrefix)

    b.mu.RLock()
    handlers := b.handlers[channelID]
    b.mu.RUnlock()

    var chatMsg Message
    proto.Unmarshal([]byte(msg.Payload), &chatMsg)

    for _, handler := range handlers {
        go handler(&chatMsg)
    }
}
```

## Acceptance Criteria
- [ ] Subscriptions tracked correctly
- [ ] Unsubscribe cleans up
- [ ] No message loss during resubscribe
- [ ] Handles reconnection
"""

    descriptions["Story 3.2: Pub/Sub Message Broadcaster|Add message serialization (protobuf)"] = """
## Description
Use Protocol Buffers for efficient message serialization.

## Implementation
```go
func (b *RedisMessageBroadcaster) Broadcast(ctx context.Context, channelID string, msg *Message) error {
    // Serialize with protobuf
    data, err := proto.Marshal(msg)
    if err != nil {
        return fmt.Errorf("failed to serialize message: %w", err)
    }

    return b.client.Publish(ctx, ChatChannel(channelID), data).Err()
}

func deserializeMessage(data []byte) (*Message, error) {
    var msg Message
    if err := proto.Unmarshal(data, &msg); err != nil {
        return nil, err
    }
    return &msg, nil
}
```

## Why Protobuf?
- Smaller than JSON (30-50% reduction)
- Faster serialization
- Type safety
- Schema evolution support

## Acceptance Criteria
- [ ] Messages serialized with protobuf
- [ ] Backward compatible changes possible
- [ ] Smaller wire size than JSON
- [ ] Fast serialization (< 1ms)
"""

    descriptions["Story 3.2: Pub/Sub Message Broadcaster|Add pub/sub error handling"] = """
## Description
Handle Redis pub/sub errors gracefully.

## Error Scenarios
1. Connection lost during publish
2. Subscription channel closed
3. Message deserialization failure
4. Handler panic

## Implementation
```go
func (b *RedisMessageBroadcaster) Broadcast(ctx context.Context, channelID string, msg *Message) error {
    data, _ := proto.Marshal(msg)

    err := b.client.Publish(ctx, ChatChannel(channelID), data).Err()
    if err != nil {
        // Log but don't fail - message goes to local subscribers
        log.Warn("redis publish failed", "error", err, "channel", channelID)

        // Fallback to local broadcast only
        b.broadcastLocal(channelID, msg)
        return nil
    }
    return nil
}
```

## Acceptance Criteria
- [ ] Connection errors don't crash
- [ ] Automatic reconnection
- [ ] Local delivery continues
- [ ] Errors logged with context
"""

    descriptions["Story 3.2: Pub/Sub Message Broadcaster|Add pub/sub metrics"] = """
## Description
Track pub/sub performance metrics.

## Metrics to Track
```go
var (
    messagesPublished = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "chat_pubsub_messages_published_total",
        },
        []string{"channel_type"},
    )

    messagesReceived = promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "chat_pubsub_messages_received_total",
        },
        []string{"channel_type"},
    )

    publishLatency = promauto.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "chat_pubsub_publish_duration_seconds",
            Buckets: prometheus.DefBuckets,
        },
    )
)
```

## Acceptance Criteria
- [ ] Publish count tracked
- [ ] Receive count tracked
- [ ] Latency histogram
- [ ] Error count by type
"""

    # Story 3.3: Session Management
    descriptions["Story 3.3: Session Management|Design session data structure"] = """
## Description
Define what data to store per user session.

## Session Data
```go
type Session struct {
    UserID       string    `json:"user_id"`
    ServerID     string    `json:"server_id"`     // Which instance
    ConnectedAt  time.Time `json:"connected_at"`
    LastActivity time.Time `json:"last_activity"`
    Channels     []string  `json:"channels"`      // Subscribed channels
    Metadata     map[string]string `json:"metadata"` // Game context
}
```

## Redis Storage
```
Key: session:{user_id}
Value: JSON-encoded Session
TTL: 5 minutes (refreshed on activity)
```

## Acceptance Criteria
- [ ] Session struct defined
- [ ] Server instance tracked
- [ ] Subscriptions tracked
- [ ] Expiration configured
"""

    descriptions["Story 3.3: Session Management|Implement RedisSessionManager"] = """
## Description
Implement session management using Redis.

## Implementation
```go
type RedisSessionManager struct {
    client redis.UniversalClient
    ttl    time.Duration
}

func (m *RedisSessionManager) Register(ctx context.Context, userID, serverID string) (*Session, error) {
    session := &Session{
        UserID:      userID,
        ServerID:    serverID,
        ConnectedAt: time.Now(),
    }

    data, _ := json.Marshal(session)
    err := m.client.Set(ctx, sessionKey(userID), data, m.ttl).Err()
    return session, err
}

func (m *RedisSessionManager) GetSession(ctx context.Context, userID string) (*Session, error) {
    data, err := m.client.Get(ctx, sessionKey(userID)).Bytes()
    if err == redis.Nil {
        return nil, ErrSessionNotFound
    }

    var session Session
    json.Unmarshal(data, &session)
    return &session, nil
}
```

## Acceptance Criteria
- [ ] Sessions stored in Redis
- [ ] Fast lookups (< 5ms)
- [ ] Atomic operations
- [ ] Handles concurrent access
"""

    descriptions["Story 3.3: Session Management|Add session expiration (TTL)"] = """
## Description
Configure automatic session cleanup using Redis TTL.

## Implementation
```go
const SessionTTL = 5 * time.Minute

func (m *RedisSessionManager) Register(ctx context.Context, userID, serverID string) (*Session, error) {
    // ... create session ...

    // Set with TTL
    err := m.client.Set(ctx, sessionKey(userID), data, SessionTTL).Err()
    return session, err
}

func (m *RedisSessionManager) RefreshSession(ctx context.Context, userID string) error {
    // Extend TTL without modifying data
    return m.client.Expire(ctx, sessionKey(userID), SessionTTL).Err()
}
```

## Acceptance Criteria
- [ ] Sessions expire after inactivity
- [ ] TTL refreshed on activity
- [ ] Clean disconnect removes immediately
- [ ] Expired sessions auto-cleaned
"""

    descriptions["Story 3.3: Session Management|Add session refresh on activity"] = """
## Description
Extend session TTL when user is active.

## Activity Events
- Message sent
- Channel joined/left
- Presence update
- Periodic heartbeat

## Implementation
```go
func (s *Server) SendMessage(ctx context.Context, req *Request) (*Response, error) {
    userID := getUserFromContext(ctx)

    // Refresh session TTL
    s.sessions.RefreshSession(ctx, userID)

    // Process message...
}
```

## Acceptance Criteria
- [ ] Session extended on activity
- [ ] Not extended on every message (debounce)
- [ ] Background heartbeat refreshes
- [ ] Inactive sessions expire
"""

    descriptions["Story 3.3: Session Management|Track user's current server instance"] = """
## Description
Track which server instance a user is connected to.

## Use Cases
- Route whispers to correct server
- Know where to send notifications
- Detect duplicate connections

## Implementation
```go
func (m *RedisSessionManager) GetServerForUser(ctx context.Context, userID string) (string, error) {
    session, err := m.GetSession(ctx, userID)
    if err != nil {
        return "", err
    }
    return session.ServerID, nil
}

// Server ID assigned on startup
var serverID = uuid.NewString()
```

## Acceptance Criteria
- [ ] Server ID unique per instance
- [ ] Stored in session
- [ ] Queryable by user ID
- [ ] Updated on reconnect
"""

    descriptions["Story 3.3: Session Management|Handle session migration on reconnect"] = """
## Description
Handle user reconnecting to a different server instance.

## Scenario
1. User connected to Server A
2. Server A dies or user disconnects
3. User reconnects to Server B
4. Session should transfer cleanly

## Implementation
```go
func (s *Server) OnConnect(ctx context.Context, userID string) error {
    // Check for existing session
    existing, err := s.sessions.GetSession(ctx, userID)
    if err == nil && existing.ServerID != s.serverID {
        // User was on different server - notify old server
        s.notifySessionMigration(ctx, existing.ServerID, userID)
    }

    // Register new session on this server
    return s.sessions.Register(ctx, userID, s.serverID)
}
```

## Acceptance Criteria
- [ ] New session overwrites old
- [ ] Old server notified (cleanup)
- [ ] Channels resubscribed
- [ ] No duplicate message delivery
"""

    # Continue with Epic 4-11...
    # Story 3.4 and 3.5
    descriptions["Story 3.4: Distributed Channel State|Store channel membership in Redis"] = """
## Description
Store channel membership in Redis for fast distributed lookups.

## Data Structure
```
Key: channel:{channel_id}:members
Type: Redis Set
Values: user IDs
```

## Implementation
```go
func (r *RedisChannelRepository) AddMember(ctx context.Context, channelID, userID string) error {
    return r.client.SAdd(ctx, channelMembersKey(channelID), userID).Err()
}

func (r *RedisChannelRepository) GetMembers(ctx context.Context, channelID string) ([]string, error) {
    return r.client.SMembers(ctx, channelMembersKey(channelID)).Result()
}
```

## Acceptance Criteria
- [ ] Members stored in Redis Set
- [ ] O(1) add/remove operations
- [ ] Fast member count queries
- [ ] Synced with PostgreSQL
"""

    descriptions["Story 3.4: Distributed Channel State|Use Redis Sets for channel members"] = """
## Description
Use Redis Sets for efficient membership operations.

## Operations
```go
// Add member
SADD channel:{id}:members {user_id}

// Remove member
SREM channel:{id}:members {user_id}

// Check membership
SISMEMBER channel:{id}:members {user_id}

// Get all members
SMEMBERS channel:{id}:members

// Count members
SCARD channel:{id}:members
```

## Acceptance Criteria
- [ ] All operations use Sets
- [ ] O(1) membership check
- [ ] Atomic add/remove
- [ ] No duplicate members
"""

    descriptions["Story 3.4: Distributed Channel State|Sync channel state across instances"] = """
## Description
Ensure channel state is consistent across all server instances.

## Sync Strategy
1. Redis is source of truth for membership
2. PostgreSQL is source of truth for metadata
3. Local cache with short TTL

## Implementation
```go
func (s *Server) OnChannelUpdate(ctx context.Context, channelID string) {
    // Invalidate local cache
    s.channelCache.Delete(channelID)

    // Notify other instances via pub/sub
    s.broadcaster.Broadcast(ctx, "system:channel-updates", &ChannelUpdate{
        ChannelID: channelID,
        Type:      "invalidate",
    })
}
```

## Acceptance Criteria
- [ ] Changes visible across instances
- [ ] Eventual consistency (< 1 second)
- [ ] Cache invalidation works
- [ ] No stale membership data
"""

    descriptions["Story 3.4: Distributed Channel State|Handle split-brain scenarios"] = """
## Description
Handle network partitions gracefully.

## Strategies
1. **Last-write-wins**: Use timestamps for conflict resolution
2. **Merge**: Combine state from both sides
3. **Leader election**: One instance owns channel

## Implementation
```go
// Use Redis WATCH for optimistic locking
func (r *RedisChannelRepository) UpdateChannel(ctx context.Context, ch *Channel) error {
    key := channelKey(ch.ID)

    return r.client.Watch(ctx, func(tx *redis.Tx) error {
        // Check current version
        current, _ := tx.Get(ctx, key).Result()
        if current != "" && decodeVersion(current) > ch.Version {
            return ErrConcurrentModification
        }

        // Update
        _, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
            pipe.Set(ctx, key, encode(ch), 0)
            return nil
        })
        return err
    }, key)
}
```

## Acceptance Criteria
- [ ] Concurrent updates detected
- [ ] Clear conflict resolution
- [ ] No data loss
- [ ] Recovers automatically
"""

    descriptions["Story 3.4: Distributed Channel State|Add channel member count tracking"] = """
## Description
Track member counts efficiently for display.

## Implementation
```go
func (r *RedisChannelRepository) GetMemberCount(ctx context.Context, channelID string) (int64, error) {
    // Redis Set cardinality - O(1)
    return r.client.SCard(ctx, channelMembersKey(channelID)).Result()
}

// For batch queries
func (r *RedisChannelRepository) GetMemberCounts(ctx context.Context, channelIDs []string) (map[string]int64, error) {
    pipe := r.client.Pipeline()
    cmds := make(map[string]*redis.IntCmd)

    for _, id := range channelIDs {
        cmds[id] = pipe.SCard(ctx, channelMembersKey(id))
    }

    pipe.Exec(ctx)

    counts := make(map[string]int64)
    for id, cmd := range cmds {
        counts[id], _ = cmd.Result()
    }
    return counts, nil
}
```

## Acceptance Criteria
- [ ] O(1) count query
- [ ] Batch queries supported
- [ ] Used in channel listings
- [ ] Accurate count
"""

    # Story 3.5
    descriptions["Story 3.5: Load Balancing Support|Make server stateless (use Redis for all state)"] = """
## Description
Remove all local state to enable any-instance routing.

## State to Move to Redis
1. User sessions → RedisSessionManager
2. Channel membership → Redis Sets
3. Active subscriptions → Redis pub/sub
4. Rate limit counters → Redis

## Verification
```go
// Server should have no maps storing user/channel state
type Server struct {
    sessions    SessionManager      // Interface → Redis
    channels    ChannelRepository   // Interface → Redis/Postgres
    broadcaster MessageBroadcaster  // Interface → Redis pub/sub
    // NO: users map[string]*User
    // NO: channels map[string]*Channel
}
```

## Acceptance Criteria
- [ ] No local user state
- [ ] No local channel state
- [ ] Server restart doesn't lose state
- [ ] Any instance can serve any user
"""

    descriptions["Story 3.5: Load Balancing Support|Add instance registration/discovery"] = """
## Description
Register server instances for health monitoring.

## Implementation
```go
func (s *Server) RegisterInstance(ctx context.Context) error {
    instanceInfo := map[string]interface{}{
        "id":         s.instanceID,
        "host":       s.host,
        "port":       s.port,
        "started_at": time.Now().Unix(),
    }

    // Register with TTL (heartbeat renews)
    key := fmt.Sprintf("instances:%s", s.instanceID)
    return s.redis.Set(ctx, key, json.Marshal(instanceInfo), 30*time.Second).Err()
}

func (s *Server) StartHeartbeat(ctx context.Context) {
    ticker := time.NewTicker(10 * time.Second)
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.RegisterInstance(ctx)
        }
    }
}
```

## Acceptance Criteria
- [ ] Instances register on startup
- [ ] Heartbeat keeps registration alive
- [ ] Dead instances auto-removed (TTL)
- [ ] List of active instances queryable
"""

    descriptions["Story 3.5: Load Balancing Support|Add sticky sessions support (optional)"] = """
## Description
Support sticky sessions for load balancers that need it.

## Implementation
```go
// Return session token in response header
func (s *Server) OnConnect(ctx context.Context, userID string) {
    sessionToken := generateSessionToken(userID, s.instanceID)

    // Set header for load balancer
    ctx.ResponseHeader().Set("X-Session-Token", sessionToken)
}

// Session token encodes: userID + instanceID + timestamp + signature
func generateSessionToken(userID, instanceID string) string {
    payload := fmt.Sprintf("%s:%s:%d", userID, instanceID, time.Now().Unix())
    signature := hmacSign(payload, secretKey)
    return base64.StdEncoding.EncodeToString([]byte(payload + ":" + signature))
}
```

## Acceptance Criteria
- [ ] Session token in response
- [ ] Load balancer can route by token
- [ ] Works without sticky sessions too
- [ ] Token validated on subsequent requests
"""

    descriptions["Story 3.5: Load Balancing Support|Test with multiple instances"] = """
## Description
Verify distributed behavior with multiple server instances.

## Test Scenarios
1. User connects to Instance A, sends message
2. User on Instance B receives message
3. Instance A dies, user reconnects to B
4. Messages still delivered

## Test Setup
```yaml
# docker-compose.test.yml
services:
  hermes-1:
    build: .
    environment:
      - INSTANCE_ID=hermes-1
      - PORT=8081

  hermes-2:
    build: .
    environment:
      - INSTANCE_ID=hermes-2
      - PORT=8082
```

## Acceptance Criteria
- [ ] Cross-instance messaging works
- [ ] Failover works
- [ ] No message loss
- [ ] Session migration works
"""

    descriptions["Story 3.5: Load Balancing Support|Document scaling procedures"] = """
## Description
Document how to scale the chat service.

## Documentation Contents
1. **Adding instances**: Just start more containers
2. **Removing instances**: Graceful shutdown drains connections
3. **Monitoring**: Key metrics to watch
4. **Troubleshooting**: Common issues

## Example Runbook
```markdown
## Scaling Up

1. Start new instance:
   kubectl scale deployment hermes --replicas=5

2. Verify instance registered:
   redis-cli KEYS "instances:*"

3. Check load distribution:
   kubectl top pods | grep hermes

## Scaling Down

1. Mark instance for drain:
   kubectl annotate pod hermes-4 drain=true

2. Wait for connections to migrate (5 min)

3. Remove instance:
   kubectl scale deployment hermes --replicas=3
```

## Acceptance Criteria
- [ ] Scale up procedure documented
- [ ] Scale down procedure documented
- [ ] Troubleshooting guide
- [ ] Metrics to monitor listed
"""

    # =========================================================================
    # Epic 4: Presence System (abbreviated for length)
    # =========================================================================

    descriptions["Story 4.1: Presence Data Model|Define presence states (online, away, busy, offline)"] = """
## Description
Define the possible presence states for users.

## States
```protobuf
enum PresenceState {
    PRESENCE_STATE_UNKNOWN = 0;
    PRESENCE_STATE_ONLINE = 1;
    PRESENCE_STATE_AWAY = 2;
    PRESENCE_STATE_BUSY = 3;      // Do not disturb
    PRESENCE_STATE_OFFLINE = 4;
    PRESENCE_STATE_INVISIBLE = 5; // Online but appears offline
}
```

## Acceptance Criteria
- [ ] All states defined in proto
- [ ] Default is UNKNOWN
- [ ] Invisible hides from others
- [ ] State transitions validated
"""

    descriptions["Story 4.1: Presence Data Model|Design presence data structure"] = """
## Description
Define the data stored for user presence.

## Structure
```protobuf
message Presence {
    string user_id = 1;
    PresenceState state = 2;
    string status_message = 3;     // "In a raid"
    google.protobuf.Timestamp last_seen = 4;
    GamePresence game = 5;
}

message GamePresence {
    string zone = 1;               // "Stormwind"
    string activity = 2;           // "Questing"
    int32 level = 3;
    string class = 4;
}
```

## Acceptance Criteria
- [ ] Core presence fields defined
- [ ] Game-specific data included
- [ ] Custom status message supported
- [ ] Last seen timestamp tracked
"""

    # Add remaining stories with similar detail...
    # For brevity, I'll add key stories from each Epic

    # Epic 5: Advanced Chat Features
    descriptions["Story 5.1: Whisper (Private Messages)|Add SendWhisper RPC"] = """
## Description
Implement private messaging between two users.

## Proto
```protobuf
rpc SendWhisper(SendWhisperRequest) returns (SendWhisperResponse);

message SendWhisperRequest {
    string recipient_id = 1;
    string content = 2;
}
```

## Implementation
1. Validate recipient exists
2. Check not blocked
3. Create/get whisper channel
4. Route to recipient's server
5. Store for offline delivery

## Acceptance Criteria
- [ ] Whisper delivered to recipient
- [ ] Works across server instances
- [ ] Blocked users can't whisper
- [ ] History preserved
"""

    descriptions["Story 5.6: Moderation Tools|Add user mute functionality"] = """
## Description
Allow moderators to mute users in channels.

## Implementation
```go
type Mute struct {
    UserID    string
    ChannelID string    // Empty = global mute
    MutedBy   string
    Reason    string
    ExpiresAt time.Time // Zero = permanent
}

func (s *Server) MuteUser(ctx context.Context, mute *Mute) error {
    // Store mute in Redis with TTL
    key := fmt.Sprintf("mute:%s:%s", mute.ChannelID, mute.UserID)

    var ttl time.Duration
    if !mute.ExpiresAt.IsZero() {
        ttl = time.Until(mute.ExpiresAt)
    }

    return s.redis.Set(ctx, key, json.Marshal(mute), ttl).Err()
}

func (s *Server) IsUserMuted(ctx context.Context, channelID, userID string) bool {
    key := fmt.Sprintf("mute:%s:%s", channelID, userID)
    return s.redis.Exists(ctx, key).Val() > 0
}
```

## Acceptance Criteria
- [ ] Muted users can't send messages
- [ ] Mutes can be temporary or permanent
- [ ] Mute reason stored
- [ ] Mute audit trail
"""

    # Epic 6: API Gateway
    descriptions["Story 6.1: Gateway Selection & Setup|Evaluate Envoy vs Kong vs custom Go"] = """
## Description
Evaluate and select API gateway solution.

## Options

### Envoy
- Pros: High performance, gRPC-native, used by Istio
- Cons: Complex configuration, C++ (harder to debug)

### Kong
- Pros: Rich plugin ecosystem, easy admin API
- Cons: Lua plugins, less gRPC-native

### Custom Go
- Pros: Full control, same language as backend
- Cons: More work, reinventing wheel

## Recommendation
Envoy for production (gRPC-native, battle-tested)

## Acceptance Criteria
- [ ] Decision documented with rationale
- [ ] Proof of concept tested
- [ ] gRPC proxying verified
- [ ] Team agrees on choice
"""

    # Epic 7: Discord Integration
    descriptions["Story 7.1: Discord Bot Setup|Create Discord application and bot"] = """
## Description
Create and configure a Discord application for game integration.

## Steps
1. Go to https://discord.com/developers/applications
2. Create new application "Hermes Chat Bot"
3. Go to Bot section, create bot
4. Enable required intents (Guild Members, Message Content)
5. Generate invite URL with required permissions

## Required Permissions
- Send Messages
- Create Public Threads
- Manage Channels (for temp voice)
- Connect (voice)
- Speak (voice)

## Environment Variables
```
DISCORD_BOT_TOKEN=xxx
DISCORD_CLIENT_ID=xxx
DISCORD_CLIENT_SECRET=xxx
```

## Acceptance Criteria
- [ ] Application created
- [ ] Bot token secured
- [ ] Intents enabled
- [ ] Invite link works
"""

    # Epic 8: Observability
    descriptions["Story 8.1: Structured Logging|Add structured logging library (zerolog/zap)"] = """
## Description
Replace standard log with structured logging.

## Choice: zerolog
- Zero allocation in hot paths
- JSON output
- Context support

## Implementation
```go
import "github.com/rs/zerolog"

var log zerolog.Logger

func init() {
    log = zerolog.New(os.Stdout).
        With().
        Timestamp().
        Str("service", "hermes").
        Logger()
}

func (s *Server) SendMessage(ctx context.Context, req *Request) {
    log.Info().
        Str("user_id", getUserID(ctx)).
        Str("channel_id", req.ChannelID).
        Str("request_id", getRequestID(ctx)).
        Msg("message sent")
}
```

## Acceptance Criteria
- [ ] All logs are JSON
- [ ] Consistent field names
- [ ] Log level configurable
- [ ] Context fields included
"""

    # Epic 9: Deployment
    descriptions["Story 9.1: Containerization|Create multi-stage Dockerfile"] = """
## Description
Create optimized Docker image for the chat server.

## Dockerfile
```dockerfile
# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata
RUN adduser -D -g '' appuser

COPY --from=builder /server /server

USER appuser
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s \\
    CMD wget --spider -q http://localhost:8080/health || exit 1

ENTRYPOINT ["/server"]
```

## Acceptance Criteria
- [ ] Multi-stage build
- [ ] Non-root user
- [ ] Health check included
- [ ] Image size < 50MB
"""

    # Epic 10: Unreal Engine Integration
    descriptions["Story 10.1: Protocol Buffer Generation for C++|Set up protoc with C++ plugin"] = """
## Description
Generate C++ code from protobuf definitions.

## Steps
1. Install protoc and C++ plugin
2. Add CMake integration
3. Generate code to UnrealPlugin/Source/Generated/

## CMakeLists.txt
```cmake
find_package(Protobuf REQUIRED)
find_package(gRPC REQUIRED)

set(PROTO_FILES
    ${CMAKE_SOURCE_DIR}/proto/chat/v1/chat.proto
)

protobuf_generate(
    TARGET chat_proto
    LANGUAGE cpp
    PROTOS ${PROTO_FILES}
)

grpc_generate(
    TARGET chat_grpc
    LANGUAGE cpp
    PROTOS ${PROTO_FILES}
)
```

## Acceptance Criteria
- [ ] C++ code generates from proto
- [ ] Compiles with Unreal
- [ ] Types match Go server
- [ ] Build integrated with UE
"""

    # Epic 11: Future Planning
    descriptions["Story 11.1: Service Boundaries Definition|Document service boundaries"] = """
## Description
Plan future microservices architecture.

## Proposed Services
1. **Chat Service**: Core messaging, channels
2. **Presence Service**: Online status, game presence
3. **Auth Service**: JWT validation, session management
4. **Gateway Service**: API routing, rate limiting
5. **Discord Service**: Discord bot integration
6. **Notification Service**: Push notifications, email

## Service Communication
- Sync: gRPC for real-time operations
- Async: Redis pub/sub for events
- Data: Each service owns its data

## Acceptance Criteria
- [ ] Service boundaries documented
- [ ] Data ownership defined
- [ ] Communication patterns specified
- [ ] Migration path from monolith
"""

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
