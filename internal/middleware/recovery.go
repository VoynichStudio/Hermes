package middleware

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"

	"connectrpc.com/connect"
)

// RecoveryConfig configures the panic recovery middleware
type RecoveryConfig struct {
	// LogStackTrace logs the full stack trace on panic
	LogStackTrace bool
	// OnPanic is called when a panic occurs (optional)
	OnPanic func(ctx context.Context, procedure string, panicVal interface{}, stack []byte)
}

// DefaultRecoveryConfig returns a default recovery configuration
func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{
		LogStackTrace: true,
		OnPanic:       nil,
	}
}

// NewRecoveryInterceptor returns an interceptor that recovers from panics
func NewRecoveryInterceptor(cfg RecoveryConfig) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (resp connect.AnyResponse, err error) {
			// Skip for client-side
			if req.Spec().IsClient {
				return next(ctx, req)
			}

			defer func() {
				if r := recover(); r != nil {
					procedure := req.Spec().Procedure
					requestID := RequestIDFromContext(ctx)
					stack := debug.Stack()

					// Log the panic
					log.Printf("[%s] PANIC in %s: %v", requestID, procedure, r)
					if cfg.LogStackTrace {
						log.Printf("[%s] Stack trace:\n%s", requestID, stack)
					}

					// Call optional handler
					if cfg.OnPanic != nil {
						cfg.OnPanic(ctx, procedure, r, stack)
					}

					// Return internal error
					err = connect.NewError(
						connect.CodeInternal,
						fmt.Errorf("internal server error"),
					)
				}
			}()

			return next(ctx, req)
		}
	}
}

// NewStreamingRecoveryInterceptor returns a streaming interceptor for panic recovery
func NewStreamingRecoveryInterceptor(cfg RecoveryConfig) connect.Interceptor {
	return &streamingRecoveryInterceptor{cfg: cfg}
}

type streamingRecoveryInterceptor struct {
	cfg RecoveryConfig
}

func (i *streamingRecoveryInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return next
}

func (i *streamingRecoveryInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *streamingRecoveryInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) (err error) {
		defer func() {
			if r := recover(); r != nil {
				procedure := conn.Spec().Procedure
				requestID := RequestIDFromContext(ctx)
				stack := debug.Stack()

				// Log the panic
				log.Printf("[%s] PANIC in stream %s: %v", requestID, procedure, r)
				if i.cfg.LogStackTrace {
					log.Printf("[%s] Stack trace:\n%s", requestID, stack)
				}

				// Call optional handler
				if i.cfg.OnPanic != nil {
					i.cfg.OnPanic(ctx, procedure, r, stack)
				}

				// Return internal error
				err = connect.NewError(
					connect.CodeInternal,
					fmt.Errorf("internal server error"),
				)
			}
		}()

		return next(ctx, conn)
	}
}
