package middleware

import (
	"connectrpc.com/connect"

	"Hermes/pkg/auth"
)

// ChainConfig holds configuration for all middleware
type ChainConfig struct {
	Logging   LoggingConfig
	Recovery  RecoveryConfig
	RateLimit RateLimitConfig
}

// DefaultChainConfig returns the default middleware configuration
func DefaultChainConfig() ChainConfig {
	return ChainConfig{
		Logging:   DefaultLoggingConfig(),
		Recovery:  DefaultRecoveryConfig(),
		RateLimit: DefaultRateLimitConfig(),
	}
}

// Chain holds all middleware components
type Chain struct {
	cfg         ChainConfig
	rateLimiter *RateLimiter
}

// NewChain creates a new middleware chain with the given configuration
func NewChain(cfg ChainConfig) *Chain {
	return &Chain{
		cfg:         cfg,
		rateLimiter: NewRateLimiter(cfg.RateLimit),
	}
}

// Stop cleans up middleware resources
func (c *Chain) Stop() {
	if c.rateLimiter != nil {
		c.rateLimiter.Stop()
	}
}

// UnaryInterceptors returns all unary interceptors in the correct order
// Order: Recovery -> RequestID -> Logging -> Auth -> RateLimit -> Handler
// This ensures:
// - Recovery catches panics from all other middleware
// - RequestID is available for logging
// - Logging captures all requests including auth failures
// - Auth runs before rate limiting (so we know who to rate limit)
// - RateLimit is last before the actual handler
func (c *Chain) UnaryInterceptors() []connect.Interceptor {
	return []connect.Interceptor{
		NewRecoveryInterceptor(c.cfg.Recovery),
		NewRequestIDInterceptor(),
		NewLoggingInterceptor(c.cfg.Logging),
		auth.NewAuthInterceptor(),
		NewRateLimitInterceptor(c.rateLimiter),
	}
}

// StreamingInterceptors returns all streaming interceptors in the correct order
func (c *Chain) StreamingInterceptors() []connect.Interceptor {
	return []connect.Interceptor{
		NewStreamingRecoveryInterceptor(c.cfg.Recovery),
		NewStreamingRequestIDInterceptor(),
		NewStreamingLoggingInterceptor(c.cfg.Logging),
		auth.NewStreamingAuthInterceptor(),
		NewStreamingRateLimitInterceptor(c.rateLimiter),
	}
}

// AllInterceptors returns all interceptors (both unary and streaming combined)
// For use with connect.WithInterceptors()
func (c *Chain) AllInterceptors() []connect.Interceptor {
	// Combine unary-wrapped interceptors with streaming interceptors
	// The streaming interceptors also handle unary through their WrapUnary method
	return []connect.Interceptor{
		// Recovery - catches panics
		NewStreamingRecoveryInterceptor(c.cfg.Recovery),
		NewRecoveryInterceptor(c.cfg.Recovery),
		// Request ID - adds tracing
		NewStreamingRequestIDInterceptor(),
		NewRequestIDInterceptor(),
		// Logging - logs all requests
		NewStreamingLoggingInterceptor(c.cfg.Logging),
		NewLoggingInterceptor(c.cfg.Logging),
		// Auth - validates tokens
		auth.NewStreamingAuthInterceptor(),
		auth.NewAuthInterceptor(),
		// Rate limiting - prevents abuse
		NewStreamingRateLimitInterceptor(c.rateLimiter),
		NewRateLimitInterceptor(c.rateLimiter),
	}
}

// CombinedInterceptors returns a single list of interceptors that handle both
// unary and streaming calls. This is the recommended way to use the middleware.
func (c *Chain) CombinedInterceptors() []connect.Interceptor {
	return []connect.Interceptor{
		// Combined interceptors that handle both unary and streaming
		&combinedInterceptor{
			recovery:    c.cfg.Recovery,
			requestID:   true,
			logging:     c.cfg.Logging,
			auth:        true,
			rateLimiter: c.rateLimiter,
		},
	}
}

// combinedInterceptor wraps all middleware into a single interceptor
type combinedInterceptor struct {
	recovery    RecoveryConfig
	requestID   bool
	logging     LoggingConfig
	auth        bool
	rateLimiter *RateLimiter
}

func (c *combinedInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	// Chain in reverse order (last wrapper runs first)
	handler := next

	// Rate limit (innermost)
	if c.rateLimiter != nil {
		handler = NewRateLimitInterceptor(c.rateLimiter)(handler)
	}

	// Auth
	if c.auth {
		handler = auth.NewAuthInterceptor()(handler)
	}

	// Logging
	handler = NewLoggingInterceptor(c.logging)(handler)

	// Request ID
	if c.requestID {
		handler = NewRequestIDInterceptor()(handler)
	}

	// Recovery (outermost)
	handler = NewRecoveryInterceptor(c.recovery)(handler)

	return handler
}

func (c *combinedInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (c *combinedInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	// Chain streaming interceptors
	handler := next

	// Rate limit (innermost)
	if c.rateLimiter != nil {
		handler = NewStreamingRateLimitInterceptor(c.rateLimiter).WrapStreamingHandler(handler)
	}

	// Auth
	if c.auth {
		handler = auth.NewStreamingAuthInterceptor().WrapStreamingHandler(handler)
	}

	// Logging
	handler = NewStreamingLoggingInterceptor(c.logging).WrapStreamingHandler(handler)

	// Request ID
	if c.requestID {
		handler = NewStreamingRequestIDInterceptor().WrapStreamingHandler(handler)
	}

	// Recovery (outermost)
	handler = NewStreamingRecoveryInterceptor(c.recovery).WrapStreamingHandler(handler)

	return handler
}
