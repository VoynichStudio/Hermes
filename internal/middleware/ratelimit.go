package middleware

import (
	"context"
	"sync"
	"time"

	"connectrpc.com/connect"

	"Hermes/pkg/auth"
)

// RateLimitConfig configures the rate limiting middleware
type RateLimitConfig struct {
	// RequestsPerSecond is the maximum requests allowed per second per user
	RequestsPerSecond float64
	// BurstSize is the maximum burst size allowed
	BurstSize int
	// CleanupInterval is how often to clean up expired limiters
	CleanupInterval time.Duration
	// MaxIdleTime is how long a limiter can be idle before being cleaned up
	MaxIdleTime time.Duration
}

// DefaultRateLimitConfig returns a default rate limit configuration
func DefaultRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		RequestsPerSecond: 100,
		BurstSize:         20,
		CleanupInterval:   5 * time.Minute,
		MaxIdleTime:       10 * time.Minute,
	}
}

// tokenBucket implements a simple token bucket rate limiter
type tokenBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
	lastAccess time.Time
	mu         sync.Mutex
}

func newTokenBucket(maxTokens float64, refillRate float64) *tokenBucket {
	return &tokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
		lastAccess: time.Now(),
	}
}

func (tb *tokenBucket) allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	tb.lastAccess = now

	// Refill tokens based on time elapsed
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now

	// Check if we have tokens available
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}

	return false
}

// RateLimiter manages per-user rate limits
type RateLimiter struct {
	cfg      RateLimitConfig
	limiters map[string]*tokenBucket
	mu       sync.RWMutex
	stopCh   chan struct{}
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		cfg:      cfg,
		limiters: make(map[string]*tokenBucket),
		stopCh:   make(chan struct{}),
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

// Allow checks if a request from the given user is allowed
func (rl *RateLimiter) Allow(userID string) bool {
	rl.mu.RLock()
	limiter, exists := rl.limiters[userID]
	rl.mu.RUnlock()

	if !exists {
		rl.mu.Lock()
		// Double-check after acquiring write lock
		if limiter, exists = rl.limiters[userID]; !exists {
			limiter = newTokenBucket(float64(rl.cfg.BurstSize), rl.cfg.RequestsPerSecond)
			rl.limiters[userID] = limiter
		}
		rl.mu.Unlock()
	}

	return limiter.allow()
}

// cleanup removes idle limiters
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(rl.cfg.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rl.mu.Lock()
			now := time.Now()
			for userID, limiter := range rl.limiters {
				limiter.mu.Lock()
				if now.Sub(limiter.lastAccess) > rl.cfg.MaxIdleTime {
					delete(rl.limiters, userID)
				}
				limiter.mu.Unlock()
			}
			rl.mu.Unlock()
		case <-rl.stopCh:
			return
		}
	}
}

// Stop stops the rate limiter cleanup goroutine
func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

// NewRateLimitInterceptor returns an interceptor that enforces per-user rate limits
func NewRateLimitInterceptor(limiter *RateLimiter) connect.UnaryInterceptorFunc {
	return func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Skip for client-side
			if req.Spec().IsClient {
				return next(ctx, req)
			}

			// Get user from context (set by auth interceptor)
			user, ok := auth.UserFromContext(ctx)
			if !ok {
				// No user in context, use IP or allow
				return next(ctx, req)
			}

			// Check rate limit
			if !limiter.Allow(user.Id) {
				return nil, connect.NewError(
					connect.CodeResourceExhausted,
					errRateLimited,
				)
			}

			return next(ctx, req)
		}
	}
}

// NewStreamingRateLimitInterceptor returns a streaming interceptor for rate limiting
func NewStreamingRateLimitInterceptor(limiter *RateLimiter) connect.Interceptor {
	return &streamingRateLimitInterceptor{limiter: limiter}
}

type streamingRateLimitInterceptor struct {
	limiter *RateLimiter
}

func (i *streamingRateLimitInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return next
}

func (i *streamingRateLimitInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return next
}

func (i *streamingRateLimitInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return func(ctx context.Context, conn connect.StreamingHandlerConn) error {
		// Get user from context (set by auth interceptor)
		user, ok := auth.UserFromContext(ctx)
		if !ok {
			// No user in context, allow
			return next(ctx, conn)
		}

		// Check rate limit for stream initiation
		if !i.limiter.Allow(user.Id) {
			return connect.NewError(
				connect.CodeResourceExhausted,
				errRateLimited,
			)
		}

		return next(ctx, conn)
	}
}

var errRateLimited = &rateLimitError{}

type rateLimitError struct{}

func (e *rateLimitError) Error() string {
	return "rate limit exceeded"
}
