package chat

import (
	"context"
	"sync"
	"testing"
	"time"

	chatv1 "Hermes/gen/chat/v1"
)

func TestCache_Basic(t *testing.T) {
	cache := NewCache[string, string](100, time.Minute, true)

	// Set and get
	cache.Set("key1", "value1")
	val, ok := cache.Get("key1")
	if !ok || val != "value1" {
		t.Errorf("Expected value1, got %s, ok=%v", val, ok)
	}

	// Get non-existent
	_, ok = cache.Get("nonexistent")
	if ok {
		t.Error("Expected miss for non-existent key")
	}
}

func TestCache_TTL(t *testing.T) {
	cache := NewCache[string, string](100, 50*time.Millisecond, true)

	cache.Set("key1", "value1")

	// Should exist immediately
	_, ok := cache.Get("key1")
	if !ok {
		t.Error("Expected hit for fresh entry")
	}

	// Wait for expiration
	time.Sleep(60 * time.Millisecond)

	// Should be expired
	_, ok = cache.Get("key1")
	if ok {
		t.Error("Expected miss for expired entry")
	}
}

func TestCache_CustomTTL(t *testing.T) {
	cache := NewCache[string, string](100, time.Minute, true)

	// Set with very short TTL
	cache.SetWithTTL("key1", "value1", 30*time.Millisecond)

	// Should exist
	_, ok := cache.Get("key1")
	if !ok {
		t.Error("Expected hit for fresh entry")
	}

	// Wait for expiration
	time.Sleep(50 * time.Millisecond)

	// Should be expired
	_, ok = cache.Get("key1")
	if ok {
		t.Error("Expected miss for expired entry")
	}
}

func TestCache_Eviction(t *testing.T) {
	cache := NewCache[string, string](3, time.Minute, true)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")
	cache.Set("key4", "value4") // Should evict one

	if cache.Size() > 3 {
		t.Errorf("Expected max 3 entries, got %d", cache.Size())
	}
}

func TestCache_Delete(t *testing.T) {
	cache := NewCache[string, string](100, time.Minute, true)

	cache.Set("key1", "value1")
	cache.Delete("key1")

	_, ok := cache.Get("key1")
	if ok {
		t.Error("Expected miss after delete")
	}
}

func TestCache_Clear(t *testing.T) {
	cache := NewCache[string, string](100, time.Minute, true)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Expected empty cache after clear, got %d", cache.Size())
	}
}

func TestCache_Cleanup(t *testing.T) {
	cache := NewCache[string, string](100, 30*time.Millisecond, true)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	// Wait for expiration
	time.Sleep(50 * time.Millisecond)

	count := cache.Cleanup()
	if count != 2 {
		t.Errorf("Expected 2 entries cleaned up, got %d", count)
	}

	if cache.Size() != 0 {
		t.Errorf("Expected empty cache after cleanup, got %d", cache.Size())
	}
}

func TestCache_Stats(t *testing.T) {
	cache := NewCache[string, string](100, time.Minute, true)

	cache.Set("key1", "value1")

	// Generate hits and misses
	cache.Get("key1") // hit
	cache.Get("key1") // hit
	cache.Get("key2") // miss
	cache.Get("key3") // miss

	stats := cache.GetStats()

	if stats.Hits != 2 {
		t.Errorf("Expected 2 hits, got %d", stats.Hits)
	}

	if stats.Misses != 2 {
		t.Errorf("Expected 2 misses, got %d", stats.Misses)
	}

	if stats.HitRate != 0.5 {
		t.Errorf("Expected hit rate 0.5, got %f", stats.HitRate)
	}
}

func TestCache_Concurrent(t *testing.T) {
	cache := NewCache[string, int](1000, time.Minute, true)

	var wg sync.WaitGroup
	numGoroutines := 50
	opsPerGoroutine := 100

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				key := string(rune('a' + (i % 26)))
				cache.Set(key, id*1000+i)
				cache.Get(key)
			}
		}(g)
	}

	wg.Wait()

	// Should complete without race conditions
	stats := cache.GetStats()
	t.Logf("Cache stats: hits=%d, misses=%d, size=%d", stats.Hits, stats.Misses, stats.Size)
}

func TestCachedChannelRepository_Get(t *testing.T) {
	ctx := context.Background()
	inner := NewMemoryChannelRepository()

	// Create a channel in inner repo
	channel, _ := inner.GetOrCreate(ctx, &chatv1.Channel{
		Id:    "test-channel",
		Label: "Test Channel",
	})

	config := &CacheConfig{
		ChannelTTL:      time.Minute,
		MaxChannels:     100,
		CleanupInterval: time.Minute,
		EnableStats:     true,
	}

	cached := NewCachedChannelRepository(inner, config)

	// First get - cache miss
	result, err := cached.Get(ctx, channel.Id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if result.Label != "Test Channel" {
		t.Errorf("Expected 'Test Channel', got '%s'", result.Label)
	}

	// Second get - cache hit
	result, err = cached.Get(ctx, channel.Id)
	if err != nil {
		t.Fatalf("Second Get failed: %v", err)
	}

	stats := cached.GetCacheStats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 cache hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 cache miss, got %d", stats.Misses)
	}
}

func TestCachedChannelRepository_Invalidation(t *testing.T) {
	ctx := context.Background()
	inner := NewMemoryChannelRepository()

	config := &CacheConfig{
		ChannelTTL:      time.Minute,
		MaxChannels:     100,
		CleanupInterval: time.Minute,
		EnableStats:     true,
	}

	cached := NewCachedChannelRepository(inner, config)

	// Create and cache a channel
	channel, _ := cached.GetOrCreate(ctx, &chatv1.Channel{
		Id:    "test-channel",
		Label: "Test Channel",
	})

	// Verify it's cached
	_, err := cached.Get(ctx, channel.Id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Add user should invalidate cache
	user := &chatv1.User{Id: "user-1", Username: "Test"}
	err = cached.AddUser(ctx, channel.Id, user)
	if err != nil {
		t.Fatalf("AddUser failed: %v", err)
	}

	// Get again - should be a cache miss (invalidated)
	stats := cached.GetCacheStats()
	initialMisses := stats.Misses

	_, _ = cached.Get(ctx, channel.Id)

	stats = cached.GetCacheStats()
	if stats.Misses != initialMisses+1 {
		t.Error("Expected cache miss after invalidation")
	}
}

func TestCachedChannelRepository_Delete(t *testing.T) {
	ctx := context.Background()
	inner := NewMemoryChannelRepository()

	config := DefaultCacheConfig()
	cached := NewCachedChannelRepository(inner, config)

	// Create a channel
	channel, _ := cached.GetOrCreate(ctx, &chatv1.Channel{
		Id:    "test-channel",
		Label: "Test Channel",
	})

	// Cache it
	_, _ = cached.Get(ctx, channel.Id)

	// Delete it
	err := cached.Delete(ctx, channel.Id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Should not find it
	_, err = cached.Get(ctx, channel.Id)
	if err != ErrChannelNotFound {
		t.Errorf("Expected ErrChannelNotFound after delete, got %v", err)
	}
}

func TestMessageCache_Basic(t *testing.T) {
	cache := NewMessageCache(10, time.Minute)

	msg1 := &chatv1.Message{Id: "msg-1", Content: "Hello"}
	msg2 := &chatv1.Message{Id: "msg-2", Content: "World"}

	cache.AddMessage("channel-1", msg1)
	cache.AddMessage("channel-1", msg2)

	msgs, ok := cache.GetMessages("channel-1", 10)
	if !ok {
		t.Error("Expected cache hit")
	}

	if len(msgs) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(msgs))
	}
}

func TestMessageCache_Limit(t *testing.T) {
	cache := NewMessageCache(5, time.Minute)

	// Add more messages than the limit
	for i := 0; i < 10; i++ {
		msg := &chatv1.Message{Id: string(rune('a' + i)), Content: "test"}
		cache.AddMessage("channel-1", msg)
	}

	msgs, ok := cache.GetMessages("channel-1", 10)
	if !ok {
		t.Error("Expected cache hit")
	}

	if len(msgs) != 5 {
		t.Errorf("Expected 5 messages (max limit), got %d", len(msgs))
	}

	// Should have the last 5 messages
	if msgs[0].Id != string(rune('a'+5)) {
		t.Errorf("Expected first message to be 'f', got %s", msgs[0].Id)
	}
}

func TestMessageCache_TTL(t *testing.T) {
	cache := NewMessageCache(10, 30*time.Millisecond)

	msg := &chatv1.Message{Id: "msg-1", Content: "Hello"}
	cache.AddMessage("channel-1", msg)

	// Should exist
	_, ok := cache.GetMessages("channel-1", 10)
	if !ok {
		t.Error("Expected cache hit for fresh entry")
	}

	// Wait for expiration
	time.Sleep(50 * time.Millisecond)

	// Should be expired
	_, ok = cache.GetMessages("channel-1", 10)
	if ok {
		t.Error("Expected cache miss for expired entry")
	}
}

func TestMessageCache_Invalidate(t *testing.T) {
	cache := NewMessageCache(10, time.Minute)

	msg := &chatv1.Message{Id: "msg-1", Content: "Hello"}
	cache.AddMessage("channel-1", msg)

	cache.InvalidateChannel("channel-1")

	_, ok := cache.GetMessages("channel-1", 10)
	if ok {
		t.Error("Expected cache miss after invalidation")
	}
}

func TestMessageCache_Cleanup(t *testing.T) {
	cache := NewMessageCache(10, 30*time.Millisecond)

	cache.AddMessage("channel-1", &chatv1.Message{Id: "msg-1"})
	cache.AddMessage("channel-2", &chatv1.Message{Id: "msg-2"})

	time.Sleep(50 * time.Millisecond)

	count := cache.Cleanup()
	if count != 2 {
		t.Errorf("Expected 2 channels cleaned up, got %d", count)
	}
}

func TestMessageCache_Stats(t *testing.T) {
	cache := NewMessageCache(10, time.Minute)

	cache.AddMessage("channel-1", &chatv1.Message{Id: "msg-1"})

	// Generate hits and misses
	cache.GetMessages("channel-1", 10) // hit
	cache.GetMessages("channel-1", 10) // hit
	cache.GetMessages("channel-2", 10) // miss

	stats := cache.GetStats()

	if stats.Hits != 2 {
		t.Errorf("Expected 2 hits, got %d", stats.Hits)
	}

	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
}

func TestMessageCache_Concurrent(t *testing.T) {
	cache := NewMessageCache(100, time.Minute)

	var wg sync.WaitGroup
	numGoroutines := 20
	opsPerGoroutine := 50

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			channelID := string(rune('a' + (id % 5)))
			for i := 0; i < opsPerGoroutine; i++ {
				msg := &chatv1.Message{Id: string(rune('0' + i)), Content: "test"}
				cache.AddMessage(channelID, msg)
				cache.GetMessages(channelID, 10)
			}
		}(g)
	}

	wg.Wait()

	// Should complete without race conditions
	stats := cache.GetStats()
	t.Logf("MessageCache stats: hits=%d, misses=%d, size=%d", stats.Hits, stats.Misses, stats.Size)
}
