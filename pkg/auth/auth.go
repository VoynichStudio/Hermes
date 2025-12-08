package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	chatv1 "Hermes/gen/chat/v1"

	"connectrpc.com/connect"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

const (
	authHeader = "Authorization"
)

var (
	// Configuration - can be overridden via environment variables
	cognitoRegion     = getEnv("COGNITO_REGION", "us-east-1")
	cognitoUserPoolID = getEnv("COGNITO_USER_POOL_ID", "us-east-1_example")
	cognitoIssuer     = fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s", cognitoRegion, cognitoUserPoolID)

	// tokenKeyFunc is the key function for validating tokens
	tokenKeyFunc jwt.Keyfunc

	// Errors
	ErrNoToken      = errors.New("authentication token missing")
	ErrInvalidToken = errors.New("invalid authentication token")

	// initialized tracks whether auth has been configured
	initialized bool
)

func init() {
	// Skip JWKS initialization in test mode
	if os.Getenv("GO_TEST_MODE") == "1" {
		log.Println("Auth: test mode - skipping JWKS initialization")
		return
	}

	if err := InitializeJWKS(); err != nil {
		log.Printf("Warning: Failed to initialize JWKS: %v", err)
	}
}

// InitializeJWKS loads the JWKS from Cognito
func InitializeJWKS() error {
	jwksURL := cognitoIssuer + "/.well-known/jwks.json"

	jwks, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return fmt.Errorf("failed to get JWKS from Cognito: %w", err)
	}

	tokenKeyFunc = jwks.Keyfunc
	initialized = true
	return nil
}

// SetKeyFunc allows setting a custom key function for testing
func SetKeyFunc(kf jwt.Keyfunc) {
	tokenKeyFunc = kf
	initialized = true
}

// UserContextKey is the key for storing user in context
type UserContextKey struct{}

// NewAuthInterceptor returns a unary interceptor that validates JWT tokens
func NewAuthInterceptor() connect.UnaryInterceptorFunc {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return connect.UnaryFunc(func(
			ctx context.Context,
			req connect.AnyRequest,
		) (connect.AnyResponse, error) {
			// Skip auth for client-side calls
			if req.Spec().IsClient {
				return next(ctx, req)
			}

			// Validate token on server-side
			user, err := Authenticate(req.Header())
			if err != nil {
				return nil, connect.NewError(connect.CodeUnauthenticated, err)
			}

			// Add user to context for handlers to use
			ctx = context.WithValue(ctx, UserContextKey{}, user)
			return next(ctx, req)
		})
	})
}

// NewStreamingAuthInterceptor returns a streaming interceptor that validates JWT tokens
func NewStreamingAuthInterceptor() connect.Interceptor {
	return streamingAuthInterceptor{}
}

type streamingAuthInterceptor struct{}

func (streamingAuthInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	// No-op for unary, just pass through (use NewAuthInterceptor for unary)
	return next
}

func (streamingAuthInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	// No-op for client-side, just pass through
	return next
}

func (streamingAuthInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return connect.StreamingHandlerFunc(func(
		ctx context.Context,
		conn connect.StreamingHandlerConn,
	) error {
		user, err := Authenticate(conn.RequestHeader())
		if err != nil {
			return connect.NewError(connect.CodeUnauthenticated, err)
		}

		ctx = context.WithValue(ctx, UserContextKey{}, user)
		return next(ctx, conn)
	})
}

// UserFromContext retrieves the authenticated user from context
func UserFromContext(ctx context.Context) (*chatv1.User, bool) {
	user, ok := ctx.Value(UserContextKey{}).(*chatv1.User)
	return user, ok
}

// Authenticate validates a JWT token from the Authorization header
func Authenticate(header http.Header) (*chatv1.User, error) {
	tokenStr := header.Get(authHeader)
	if tokenStr == "" {
		return nil, ErrNoToken
	}

	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")

	if !initialized || tokenKeyFunc == nil {
		return nil, errors.New("authentication not initialized")
	}

	token, err := jwt.Parse(tokenStr, tokenKeyFunc)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	return ParseClaims(token.Claims)
}

// ParseClaims extracts user information from JWT claims
func ParseClaims(claims jwt.Claims) (*chatv1.User, error) {
	mapClaims, ok := claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims format")
	}

	if iss, ok := mapClaims["iss"].(string); !ok || iss != cognitoIssuer {
		return nil, errors.New("invalid token issuer")
	}

	userID, ok := mapClaims["sub"].(string)
	if !ok {
		return nil, errors.New("missing 'sub' claim")
	}

	username, _ := mapClaims["cognito:username"].(string)

	return &chatv1.User{
		Id:       userID,
		Username: username,
	}, nil
}

// getEnv returns an environment variable value or a default
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetCognitoIssuer returns the configured Cognito issuer URL (for testing)
func GetCognitoIssuer() string {
	return cognitoIssuer
}
