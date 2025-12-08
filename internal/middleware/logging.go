package middleware

import (
	"context"
	"log"
	"time"

	"connectrpc.com/connect"
)

// LoggingConfig configures the logging middleware
type LoggingConfig struct {
	// LogRequests enables request logging
	LogRequests bool
	// LogResponses enables response logging
	LogResponses bool
	// SlowRequestThreshold logs a warning if request takes longer than this
	SlowRequestThreshold time.Duration
}

// DefaultLoggingConfig returns a default logging configuration
func DefaultLoggingConfig() LoggingConfig {
	return LoggingConfig{
		LogRequests:          true,
		LogResponses:         true,
		SlowRequestThreshold: 1 * time.Second,
	}
}

// NewLoggingInterceptor returns an interceptor that logs all RPC calls
func NewLoggingInterceptor(cfg LoggingConfig) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Skip for client-side
			if req.Spec().IsClient {
				return next(ctx, req)
			}

			start := time.Now()
			procedure := req.Spec().Procedure
			requestID := RequestIDFromContext(ctx)

			if cfg.LogRequests {
				log.Printf("[%s] RPC started: %s", requestID, procedure)
			}

			// Execute handler
			resp, err := next(ctx, req)

			duration := time.Since(start)

			// Log result
			if cfg.LogResponses {
				if err != nil {
					log.Printf("[%s] RPC failed: %s duration=%v error=%v",
						requestID, procedure, duration, err)
				} else {
					log.Printf("[%s] RPC completed: %s duration=%v",
						requestID, procedure, duration)
				}
			}

			// Warn for slow requests
			if cfg.SlowRequestThreshold > 0 && duration > cfg.SlowRequestThreshold {
				log.Printf("[%s] SLOW RPC: %s took %v (threshold: %v)",
					requestID, procedure, duration, cfg.SlowRequestThreshold)
			}

			return resp, err
		}
	}
}

// NewStreamingLoggingInterceptor returns a streaming interceptor for logging
func NewStreamingLoggingInterceptor(cfg LoggingConfig) connect.Interceptor {
	return &streamingLoggingInterceptor{cfg: cfg}
}

type streamingLoggingInterceptor struct {
	cfg LoggingConfig
}

func (i *streamingLoggingInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return next
}

func (i *streamingLoggingInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *streamingLoggingInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		start := time.Now()
		procedure := conn.Spec().Procedure
		requestID := RequestIDFromContext(ctx)

		if i.cfg.LogRequests {
			log.Printf("[%s] Stream started: %s", requestID, procedure)
		}

		// Execute handler
		err := next(ctx, conn)

		duration := time.Since(start)

		// Log result
		if i.cfg.LogResponses {
			if err != nil {
				log.Printf("[%s] Stream ended with error: %s duration=%v error=%v",
					requestID, procedure, duration, err)
			} else {
				log.Printf("[%s] Stream ended: %s duration=%v",
					requestID, procedure, duration)
			}
		}

		return err
	}
}
