package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	chatv1 "Hermes/gen/chat/v1"

	"connectrpc.com/connect"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

const (
	cognitoRegion     = "us-east-1"
	cognitoUserPoolID = "us-east-1_example"
	authHeader        = "Authorization"
)

var jwks keyfunc.Keyfunc
var (
	errNoToken      = errors.New("authentication token missing")
	errInvalidToken = errors.New("invalid authentication token")
	cognitoIssuer   = fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s", cognitoRegion, cognitoUserPoolID)
)

func init() {
	jwksURL := cognitoIssuer + "/.well-known/jwks.json"
	var err error

	jwks, err = keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		log.Fatalf("Failed to get JWKS from Cognito: %v", err)
	}
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
func NewStreamingAuthInterceptor() connect.StreamingHandlerInterceptor {
	return streamingAuthInterceptor{}
}

type streamingAuthInterceptor struct{}

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
		return nil, errNoToken
	}

	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")

	token, err := jwt.Parse(tokenStr, jwks.Keyfunc)
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if !token.Valid {
		return nil, errInvalidToken
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims format")
	}

	if iss, ok := claims["iss"].(string); !ok || iss != cognitoIssuer {
		return nil, errors.New("invalid token issuer")
	}

	userID, ok := claims["sub"].(string)
	if !ok {
		return nil, errors.New("missing 'sub' claim")
	}

	username, _ := claims["cognito:username"].(string)

	return &chatv1.User{
		Id:       userID,
		Username: username,
	}, nil
}
