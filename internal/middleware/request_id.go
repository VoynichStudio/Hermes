package middleware

import (
	"context"

	"connectrpc.com/connect"
	"github.com/google/uuid"
)

// RequestIDKey is the context key for request ID
type RequestIDKey struct{}

// RequestIDHeader is the header name for request ID
const RequestIDHeader = "X-Request-ID"

// NewRequestIDInterceptor returns an interceptor that adds a unique request ID to each request
func NewRequestIDInterceptor() connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Skip for client-side
			if req.Spec().IsClient {
				return next(ctx, req)
			}

			// Get existing request ID from header or generate new one
			requestID := req.Header().Get(RequestIDHeader)
			if requestID == "" {
				requestID = uuid.New().String()
			}

			// Add to context
			ctx = context.WithValue(ctx, RequestIDKey{}, requestID)

			// Execute handler
			resp, err := next(ctx, req)

			// Add request ID to response header
			if resp != nil {
				resp.Header().Set(RequestIDHeader, requestID)
			}

			return resp, err
		}
	}
}

// NewStreamingRequestIDInterceptor returns a streaming interceptor for request IDs
func NewStreamingRequestIDInterceptor() connect.Interceptor {
	return &streamingRequestIDInterceptor{}
}

type streamingRequestIDInterceptor struct{}

func (i *streamingRequestIDInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return next
}

func (i *streamingRequestIDInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *streamingRequestIDInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		// Get existing request ID from header or generate new one
		requestID := conn.RequestHeader().Get(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		// Add to context
		ctx = context.WithValue(ctx, RequestIDKey{}, requestID)

		// Add request ID to response header
		conn.ResponseHeader().Set(RequestIDHeader, requestID)

		return next(ctx, conn)
	}
}

// RequestIDFromContext retrieves the request ID from context
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey{}).(string); ok {
		return id
	}
	return ""
}
