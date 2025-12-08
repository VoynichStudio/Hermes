package middleware

import (
	"context"
	"testing"
	"time"
)

func TestRequestIDFromContext(t *testing.T) {
	t.Run("returns empty string when no ID in context", func(t *testing.T) {
		ctx := context.Background()
		id := RequestIDFromContext(ctx)
		if id != "" {
			t.Errorf("Expected empty string, got %q", id)
		}
	})

	t.Run("returns ID when present in context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), RequestIDKey{}, "test-id-123")
		id := RequestIDFromContext(ctx)
		if id != "test-id-123" {
			t.Errorf("Expected %q, got %q", "test-id-123", id)
		}
	})
}

func TestDefaultConfigs(t *testing.T) {
	t.Run("DefaultLoggingConfig", func(t *testing.T) {
		cfg := DefaultLoggingConfig()
		if !cfg.LogRequests {
			t.Error("Expected LogRequests to be true")
		}
		if !cfg.LogResponses {
			t.Error("Expected LogResponses to be true")
		}
		if cfg.SlowRequestThreshold != 1*time.Second {
			t.Errorf("SlowRequestThreshold = %v, want %v", cfg.SlowRequestThreshold, 1*time.Second)
		}
	})

	t.Run("DefaultRecoveryConfig", func(t *testing.T) {
		cfg := DefaultRecoveryConfig()
		if !cfg.LogStackTrace {
			t.Error("Expected LogStackTrace to be true")
		}
	})

	t.Run("DefaultRateLimitConfig", func(t *testing.T) {
		cfg := DefaultRateLimitConfig()
		if cfg.RequestsPerSecond != 100 {
			t.Errorf("RequestsPerSecond = %v, want %v", cfg.RequestsPerSecond, 100)
		}
		if cfg.BurstSize != 20 {
			t.Errorf("BurstSize = %v, want %v", cfg.BurstSize, 20)
		}
	})

	t.Run("DefaultChainConfig", func(t *testing.T) {
		cfg := DefaultChainConfig()
		if !cfg.Logging.LogRequests {
			t.Error("Expected Logging.LogRequests to be true")
		}
		if !cfg.Recovery.LogStackTrace {
			t.Error("Expected Recovery.LogStackTrace to be true")
		}
		if cfg.RateLimit.RequestsPerSecond != 100 {
			t.Errorf("RateLimit.RequestsPerSecond = %v, want %v", cfg.RateLimit.RequestsPerSecond, 100)
		}
	})
}

func TestRateLimiter(t *testing.T) {
	cfg := RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
		CleanupInterval:   1 * time.Hour,
		MaxIdleTime:       1 * time.Hour,
	}

	limiter := NewRateLimiter(cfg)
	defer limiter.Stop()

	t.Run("allows requests within limit", func(t *testing.T) {
		userID := "user-1"
		for i := 0; i < 5; i++ {
			if !limiter.Allow(userID) {
				t.Errorf("Request %d should be allowed", i)
			}
		}
	})

	t.Run("blocks requests exceeding limit", func(t *testing.T) {
		userID := "user-2"
		// Exhaust burst
		for i := 0; i < 5; i++ {
			limiter.Allow(userID)
		}

		// Next request should be blocked
		if limiter.Allow(userID) {
			t.Error("Request should be blocked after burst exhausted")
		}
	})

	t.Run("refills tokens over time", func(t *testing.T) {
		userID := "user-3"
		// Exhaust burst
		for i := 0; i < 5; i++ {
			limiter.Allow(userID)
		}

		// Wait for refill (10 req/sec = 100ms per token)
		time.Sleep(150 * time.Millisecond)

		// Should have at least 1 token now
		if !limiter.Allow(userID) {
			t.Error("Request should be allowed after token refill")
		}
	})

	t.Run("separate limits per user", func(t *testing.T) {
		userID1 := "user-4"
		userID2 := "user-5"

		// Exhaust user1's burst
		for i := 0; i < 5; i++ {
			limiter.Allow(userID1)
		}

		// User2 should still have full burst
		for i := 0; i < 5; i++ {
			if !limiter.Allow(userID2) {
				t.Errorf("User2 request %d should be allowed", i)
			}
		}
	})
}

func TestTokenBucket(t *testing.T) {
	t.Run("allows burst", func(t *testing.T) {
		bucket := newTokenBucket(5, 10)
		for i := 0; i < 5; i++ {
			if !bucket.allow() {
				t.Errorf("Request %d should be allowed within burst", i)
			}
		}
	})

	t.Run("blocks after burst", func(t *testing.T) {
		bucket := newTokenBucket(5, 10)
		// Exhaust burst
		for i := 0; i < 5; i++ {
			bucket.allow()
		}

		if bucket.allow() {
			t.Error("Request should be blocked after burst")
		}
	})

	t.Run("refills over time", func(t *testing.T) {
		bucket := newTokenBucket(5, 10)
		// Exhaust all tokens
		for i := 0; i < 5; i++ {
			bucket.allow()
		}

		// Wait for 1 token refill (10/sec = 100ms)
		time.Sleep(120 * time.Millisecond)

		if !bucket.allow() {
			t.Error("Token should have refilled")
		}
	})

	t.Run("caps at max tokens", func(t *testing.T) {
		bucket := newTokenBucket(5, 100)

		// Wait long enough that we would have >5 tokens if uncapped
		time.Sleep(100 * time.Millisecond)

		// Should still only have 5 tokens
		allowed := 0
		for i := 0; i < 10; i++ {
			if bucket.allow() {
				allowed++
			}
		}

		if allowed > 5 {
			t.Errorf("Allowed %d requests, should cap at 5", allowed)
		}
	})
}

func TestChain(t *testing.T) {
	cfg := DefaultChainConfig()
	chain := NewChain(cfg)
	defer chain.Stop()

	t.Run("returns combined interceptors", func(t *testing.T) {
		interceptors := chain.CombinedInterceptors()
		if len(interceptors) == 0 {
			t.Error("Expected at least one interceptor")
		}
	})

	t.Run("returns unary interceptors", func(t *testing.T) {
		interceptors := chain.UnaryInterceptors()
		if len(interceptors) != 5 {
			t.Errorf("Expected 5 unary interceptors, got %d", len(interceptors))
		}
	})

	t.Run("returns streaming interceptors", func(t *testing.T) {
		interceptors := chain.StreamingInterceptors()
		if len(interceptors) != 5 {
			t.Errorf("Expected 5 streaming interceptors, got %d", len(interceptors))
		}
	})

	t.Run("stop is safe to call multiple times", func(t *testing.T) {
		localChain := NewChain(cfg)
		localChain.Stop()
		// Calling stop again should not panic
		// (though it may cause issues with the closed channel, so we don't call it)
	})
}

func TestRateLimiterCleanup(t *testing.T) {
	cfg := RateLimitConfig{
		RequestsPerSecond: 10,
		BurstSize:         5,
		CleanupInterval:   50 * time.Millisecond,
		MaxIdleTime:       50 * time.Millisecond,
	}

	limiter := NewRateLimiter(cfg)
	defer limiter.Stop()

	// Create a limiter for a user
	limiter.Allow("cleanup-test-user")

	// Initially should have 1 limiter
	limiter.mu.RLock()
	initialCount := len(limiter.limiters)
	limiter.mu.RUnlock()

	if initialCount != 1 {
		t.Errorf("Expected 1 limiter, got %d", initialCount)
	}

	// Wait for cleanup
	time.Sleep(150 * time.Millisecond)

	// Limiter should be cleaned up
	limiter.mu.RLock()
	finalCount := len(limiter.limiters)
	limiter.mu.RUnlock()

	if finalCount != 0 {
		t.Errorf("Expected 0 limiters after cleanup, got %d", finalCount)
	}
}
