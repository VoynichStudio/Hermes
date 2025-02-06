package auth

import (
	chatv1 "Hermes/gen/chat/v1"
	"context"
	"errors"
	"net/http"
	"strings"
	"log"
	"fmt"

	"connectrpc.com/connect"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)
const (
	cognitoRegion     = "us-east-1"
	cognitoUserPoolID = "us-east-1_example"
	tokenHeader = "Acme-Token"
)

var jwks keyfunc.Keyfunc
var (
	errNoToken = errors.New("Authentication token missing")
	cognitoIssuer = fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s", cognitoRegion, cognitoUserPoolID)
)

func init()  {
	jwksURL := cognitoIssuer+"/.well-known/jwks.json"
	var err error

	jwks, err = keyfunc.NewDefault([]string{jwksURL})

	if err != nil {
		log.Fatalf("Failed to get JWKS from Cognito: %v", err)
	}
}

type authInterceptor struct {}

func NewAuthInterceptor() *authInterceptor {
	return &authInterceptor{}
}

func NewAuthIntercepto() connect.UnaryInterceptorFunc {
  interceptor := func(next connect.UnaryFunc) connect.UnaryFunc {
    return connect.UnaryFunc(func(
      ctx context.Context,
      req connect.AnyRequest,
    ) (connect.AnyResponse, error) {
      if req.Spec().IsClient {
        // Send a token with client requests.
        req.Header().Set(tokenHeader, "sample")
      } else if req.Header().Get(tokenHeader) == "" {
        // Check token in handlers.
        return nil, connect.NewError(
          connect.CodeUnauthenticated,
          errors.New("no token provided"),
        )
      }
      return next(ctx, req)
    })
  }
  return connect.UnaryInterceptorFunc(interceptor)
}
func (i *authInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
  // Same as previous UnaryInterceptorFunc.
  return connect.UnaryFunc(func(
    ctx context.Context,
    req connect.AnyRequest,
  ) (connect.AnyResponse, error) {
    if req.Spec().IsClient {
      // Send a token with client requests.
      req.Header().Set(tokenHeader, "sample")
    } else if req.Header().Get(tokenHeader) == "" {
      // Check token in handlers.
      return nil, connect.NewError(connect.CodeUnauthenticated, errNoToken)
    }
    return next(ctx, req)
  })
}

func (*authInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
  return connect.StreamingClientFunc(func(
    ctx context.Context,
    spec connect.Spec,
  ) connect.StreamingClientConn {
    conn := next(ctx, spec)
    conn.RequestHeader().Set(tokenHeader, "sample")
    return conn
  })
}

func (i *authInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
  return connect.StreamingHandlerFunc(func(
    ctx context.Context,
    conn connect.StreamingHandlerConn,
  ) error {
    if conn.RequestHeader().Get(tokenHeader) == "" {
      return connect.NewError(connect.CodeUnauthenticated, errNoToken)
    }
    return next(ctx, conn)
  })
}

func Authenticate(header http.Header)  (*chatv1.User, error) {
	tokenStr := header.Get("Authorization")
	if tokenStr == "" {
		return nil, errors.New("Missing Authorization token")
	}

	tokenStr = strings.TrimPrefix(tokenStr, "Bearer ")
	
	token, err := jwt.Parse(tokenStr, jwks.Keyfunc)

	if err != nil {
		return nil, errors.New("invalid token: " + err.Error())
	}

	if !token.Valid {
		return nil, errors.New("invalid JWT token")
	}

	claims, ok := token.Claims.(jwt.MapClaims)

	if !ok {
		return nil, errors.New("invalid claims format")
	}

	if iss, ok := claims["iss"].(string); !ok || iss != cognitoIssuer {
		return nil, errors.New("Invalid token issuer")
	}
	
	userID, ok := claims["sub"].(string)
	if !ok {
		return nil, errors.New("missing 'sub' claim")
	}

	username, _ := claims["cognito:username"].(string) // Use Cognito username if available

	return &chatv1.User{
		Id:       userID,
		Username: username,
	}, nil
}

