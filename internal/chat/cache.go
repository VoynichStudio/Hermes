package chat

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	chatv1 "Hermes/gen/chat/v1"
)

// CacheConfig holds configuration for caching
type CacheConfig struct {
	ChannelTTL        time.Duration // How long to cache channel data
	UserTTL           time.Duration // How long to cache user data
	MessageTTL        time.Duration // How long to cache recent messages
	MaxChannels       int           // Maximum cached channels
	MaxUsers          int           // Maximum cached users
	MaxMessagesPerCh  int           // Maximum messages per channel to cache
	CleanupInterval   time.Duration // How often to clean expired entries
	EnableStats       bool          // Whether to track cache statistics
}

// DefaultCacheConfig returns sensible defaults for caching
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		ChannelTTL:       5 * time.Minute,
		UserTTL:          2 * time.Minute,
		MessageTTL:       1 * time.Minute,
		MaxChannels:      1000,
		MaxUsers:         10000,
		MaxMessagesPerCh: 100,
		CleanupInterval:  30 * time.Second,
		EnableStats:      true,
	}
}

// CacheEntry holds a cached value with metadata
type CacheEntry[T any] struct {
	Value     T
	ExpiresAt time.Time
	Hits      atomic.Int64
}

// IsExpired checks if the entry has expired
func (e *CacheEntry[T]) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// CacheStats tracks cache performance
type CacheStats struct {
	Hits       int64
	Misses     int64
	Evictions  int64
	Size       int
	HitRate    float64
}

// Cache provides a generic in-memory cache with TTL support
type Cache[K comparable, V any] struct {
	entries  map[K]*CacheEntry[V]
	maxSize  int
	ttl      time.Duration
	mu       sync.RWMutex
	stats    CacheStats
	statsMu  sync.RWMutex
	tracking bool
}

// NewCache creates a new generic cache
func NewCache[K comparable, V any](maxSize int, ttl time.Duration, trackStats bool) *Cache[K, V] {
	return &Cache[K, V]{
		entries:  make(map[K]*CacheEntry[V]),
		maxSize:  maxSize,
		ttl:      ttl,
		tracking: trackStats,
	}
}

// Get retrieves a value from the cache
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.RLock()
	entry, exists := c.entries[key]
	c.mu.RUnlock()

	var zero V
	if !exists {
		c.recordMiss()
		return zero, false
	}

	if entry.IsExpired() {
		c.Delete(key)
		c.recordMiss()
		return zero, false
	}

	c.recordHit()
	entry.Hits.Add(1)
	return entry.Value, true
}

// Set stores a value in the cache
func (c *Cache[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = &CacheEntry[V]{
		Value:     value,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

// SetWithTTL stores a value with a custom TTL
func (c *Cache[K, V]) SetWithTTL(key K, value V, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = &CacheEntry[V]{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}
}

// Delete removes a key from the cache
func (c *Cache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Clear removes all entries from the cache
func (c *Cache[K, V]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[K]*CacheEntry[V])
}

// Size returns the current number of cached entries
func (c *Cache[K, V]) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// evictOldest removes the oldest/least used entry
func (c *Cache[K, V]) evictOldest() {
	var oldestKey K
	var oldestTime time.Time
	first := true

	for key, entry := range c.entries {
		if first || entry.ExpiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.ExpiresAt
			first = false
		}
	}

	if !first {
		delete(c.entries, oldestKey)
		c.recordEviction()
	}
}

// Cleanup removes all expired entries
func (c *Cache[K, V]) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for key, entry := range c.entries {
		if entry.IsExpired() {
			delete(c.entries, key)
			count++
		}
	}
	return count
}

// GetStats returns current cache statistics
func (c *Cache[K, V]) GetStats() CacheStats {
	c.statsMu.RLock()
	defer c.statsMu.RUnlock()

	c.mu.RLock()
	size := len(c.entries)
	c.mu.RUnlock()

	stats := c.stats
	stats.Size = size
	total := stats.Hits + stats.Misses
	if total > 0 {
		stats.HitRate = float64(stats.Hits) / float64(total)
	}
	return stats
}

func (c *Cache[K, V]) recordHit() {
	if c.tracking {
		c.statsMu.Lock()
		c.stats.Hits++
		c.statsMu.Unlock()
	}
}

func (c *Cache[K, V]) recordMiss() {
	if c.tracking {
		c.statsMu.Lock()
		c.stats.Misses++
		c.statsMu.Unlock()
	}
}

func (c *Cache[K, V]) recordEviction() {
	if c.tracking {
		c.statsMu.Lock()
		c.stats.Evictions++
		c.statsMu.Unlock()
	}
}

// CachedChannelRepository wraps a ChannelRepository with caching
type CachedChannelRepository struct {
	inner    ChannelRepository
	cache    *Cache[string, *chatv1.Channel]
	config   *CacheConfig
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewCachedChannelRepository creates a cached channel repository
func NewCachedChannelRepository(inner ChannelRepository, config *CacheConfig) *CachedChannelRepository {
	if config == nil {
		config = DefaultCacheConfig()
	}

	repo := &CachedChannelRepository{
		inner:  inner,
		cache:  NewCache[string, *chatv1.Channel](config.MaxChannels, config.ChannelTTL, config.EnableStats),
		config: config,
		stopCh: make(chan struct{}),
	}

	return repo
}

// Start begins the background cleanup loop
func (r *CachedChannelRepository) Start() {
	r.wg.Add(1)
	go r.cleanupLoop()
}

// Stop stops the background cleanup loop
func (r *CachedChannelRepository) Stop() {
	close(r.stopCh)
	r.wg.Wait()
}

func (r *CachedChannelRepository) cleanupLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.cache.Cleanup()
		}
	}
}

// Create creates a new channel
func (r *CachedChannelRepository) Create(ctx context.Context, channel *chatv1.Channel) error {
	err := r.inner.Create(ctx, channel)
	if err != nil {
		return err
	}

	// Cache the new channel
	r.cache.Set(channel.Id, channel)
	return nil
}

// Get retrieves a channel, checking cache first
func (r *CachedChannelRepository) Get(ctx context.Context, channelID string) (*chatv1.Channel, error) {
	// Check cache
	if channel, ok := r.cache.Get(channelID); ok {
		return channel, nil
	}

	// Cache miss, fetch from inner
	channel, err := r.inner.Get(ctx, channelID)
	if err != nil {
		return nil, err
	}

	// Cache the result
	r.cache.Set(channelID, channel)
	return channel, nil
}

// GetOrCreate gets or creates a channel
func (r *CachedChannelRepository) GetOrCreate(ctx context.Context, channel *chatv1.Channel) (*chatv1.Channel, error) {
	// Try cache first
	if cached, ok := r.cache.Get(channel.Id); ok {
		return cached, nil
	}

	// Delegate to inner
	result, err := r.inner.GetOrCreate(ctx, channel)
	if err != nil {
		return nil, err
	}

	// Cache the result
	r.cache.Set(result.Id, result)
	return result, nil
}

// Delete removes a channel
func (r *CachedChannelRepository) Delete(ctx context.Context, channelID string) error {
	// Invalidate cache first
	r.cache.Delete(channelID)

	// Delegate to inner
	return r.inner.Delete(ctx, channelID)
}

// List returns all channels (not cached)
func (r *CachedChannelRepository) List(ctx context.Context) ([]*chatv1.Channel, error) {
	return r.inner.List(ctx)
}

// AddUser adds a user to a channel
func (r *CachedChannelRepository) AddUser(ctx context.Context, channelID string, user *chatv1.User) error {
	// Invalidate cache (users list changed)
	r.cache.Delete(channelID)

	return r.inner.AddUser(ctx, channelID, user)
}

// RemoveUser removes a user from a channel
func (r *CachedChannelRepository) RemoveUser(ctx context.Context, channelID string, userID string) error {
	// Invalidate cache
	r.cache.Delete(channelID)

	return r.inner.RemoveUser(ctx, channelID, userID)
}

// GetUsers returns users in a channel
func (r *CachedChannelRepository) GetUsers(ctx context.Context, channelID string) ([]*chatv1.User, error) {
	return r.inner.GetUsers(ctx, channelID)
}

// GetCacheStats returns cache statistics
func (r *CachedChannelRepository) GetCacheStats() CacheStats {
	return r.cache.GetStats()
}

// InvalidateChannel removes a channel from cache
func (r *CachedChannelRepository) InvalidateChannel(channelID string) {
	r.cache.Delete(channelID)
}

// Ensure CachedChannelRepository implements ChannelRepository
var _ ChannelRepository = (*CachedChannelRepository)(nil)

// MessageCache caches recent messages per channel
type MessageCache struct {
	messages   map[string][]*chatv1.Message // channelID -> messages
	timestamps map[string]time.Time         // channelID -> last update
	maxPerCh   int
	ttl        time.Duration
	mu         sync.RWMutex
	stats      CacheStats
	statsMu    sync.RWMutex
}

// NewMessageCache creates a message cache
func NewMessageCache(maxPerChannel int, ttl time.Duration) *MessageCache {
	return &MessageCache{
		messages:   make(map[string][]*chatv1.Message),
		timestamps: make(map[string]time.Time),
		maxPerCh:   maxPerChannel,
		ttl:        ttl,
	}
}

// AddMessage adds a message to the cache
func (mc *MessageCache) AddMessage(channelID string, msg *chatv1.Message) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	msgs := mc.messages[channelID]
	msgs = append(msgs, msg)

	// Trim if over limit
	if len(msgs) > mc.maxPerCh {
		msgs = msgs[len(msgs)-mc.maxPerCh:]
	}

	mc.messages[channelID] = msgs
	mc.timestamps[channelID] = time.Now()
}

// GetMessages returns cached messages for a channel
func (mc *MessageCache) GetMessages(channelID string, limit int) ([]*chatv1.Message, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	timestamp, exists := mc.timestamps[channelID]
	if !exists || time.Since(timestamp) > mc.ttl {
		mc.recordMiss()
		return nil, false
	}

	msgs := mc.messages[channelID]
	if len(msgs) == 0 {
		mc.recordMiss()
		return nil, false
	}

	mc.recordHit()

	// Return last 'limit' messages
	if limit > 0 && limit < len(msgs) {
		return msgs[len(msgs)-limit:], true
	}
	return msgs, true
}

// InvalidateChannel removes cached messages for a channel
func (mc *MessageCache) InvalidateChannel(channelID string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	delete(mc.messages, channelID)
	delete(mc.timestamps, channelID)
}

// Cleanup removes expired channel messages
func (mc *MessageCache) Cleanup() int {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	now := time.Now()
	count := 0

	for channelID, timestamp := range mc.timestamps {
		if now.Sub(timestamp) > mc.ttl {
			delete(mc.messages, channelID)
			delete(mc.timestamps, channelID)
			count++
		}
	}

	return count
}

// GetStats returns cache statistics
func (mc *MessageCache) GetStats() CacheStats {
	mc.statsMu.RLock()
	defer mc.statsMu.RUnlock()

	mc.mu.RLock()
	size := len(mc.messages)
	mc.mu.RUnlock()

	stats := mc.stats
	stats.Size = size
	total := stats.Hits + stats.Misses
	if total > 0 {
		stats.HitRate = float64(stats.Hits) / float64(total)
	}
	return stats
}

func (mc *MessageCache) recordHit() {
	mc.statsMu.Lock()
	mc.stats.Hits++
	mc.statsMu.Unlock()
}

func (mc *MessageCache) recordMiss() {
	mc.statsMu.Lock()
	mc.stats.Misses++
	mc.statsMu.Unlock()
}
