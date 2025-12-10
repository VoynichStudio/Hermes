package chat

import (
	"context"
	"sync"
	"time"

	chatv1 "Hermes/gen/chat/v1"
)

// BatchConfig holds configuration for message batching
type BatchConfig struct {
	MaxBatchSize     int           // Maximum messages per batch
	MaxBatchWait     time.Duration // Maximum time to wait before flushing
	FlushInterval    time.Duration // How often to check for flush
	EnableBatching   bool          // Whether batching is enabled
	PriorityBypass   bool          // Allow priority messages to bypass batching
}

// DefaultBatchConfig returns sensible defaults for message batching
func DefaultBatchConfig() *BatchConfig {
	return &BatchConfig{
		MaxBatchSize:   50,
		MaxBatchWait:   100 * time.Millisecond,
		FlushInterval:  20 * time.Millisecond,
		EnableBatching: true,
		PriorityBypass: true,
	}
}

// MessageBatch represents a batch of messages for a channel
type MessageBatch struct {
	ChannelID string
	Messages  []*chatv1.Message
	CreatedAt time.Time
}

// BatchedBroadcaster wraps a MessageBroadcaster with batching capability
type BatchedBroadcaster struct {
	inner        MessageBroadcaster
	config       *BatchConfig
	batches      map[string]*MessageBatch // channelID -> batch
	mu           sync.Mutex
	flushCh      chan string // Channel IDs to flush
	stopCh       chan struct{}
	wg           sync.WaitGroup
	metrics      *BatchMetrics
}

// BatchMetrics tracks batching performance
type BatchMetrics struct {
	TotalMessages      int64
	BatchedMessages    int64
	ImmediateMessages  int64
	BatchesFlushed     int64
	AverageBatchSize   float64
	mu                 sync.RWMutex
}

// NewBatchedBroadcaster creates a new batched broadcaster
func NewBatchedBroadcaster(inner MessageBroadcaster, config *BatchConfig) *BatchedBroadcaster {
	if config == nil {
		config = DefaultBatchConfig()
	}

	bb := &BatchedBroadcaster{
		inner:   inner,
		config:  config,
		batches: make(map[string]*MessageBatch),
		flushCh: make(chan string, 1000),
		stopCh:  make(chan struct{}),
		metrics: &BatchMetrics{},
	}

	return bb
}

// Start begins the background flush loop
func (bb *BatchedBroadcaster) Start() {
	if !bb.config.EnableBatching {
		return
	}

	bb.wg.Add(2)
	go bb.flushLoop()
	go bb.timerLoop()
}

// Stop gracefully stops the batcher, flushing remaining messages
func (bb *BatchedBroadcaster) Stop() {
	close(bb.stopCh)
	bb.wg.Wait()

	// Flush any remaining batches
	bb.mu.Lock()
	defer bb.mu.Unlock()

	ctx := context.Background()
	for channelID, batch := range bb.batches {
		if len(batch.Messages) > 0 {
			bb.flushBatchLocked(ctx, channelID)
		}
	}
}

// Broadcast sends a message, potentially batching it
func (bb *BatchedBroadcaster) Broadcast(ctx context.Context, channelID string, msg *chatv1.Message) error {
	bb.metrics.mu.Lock()
	bb.metrics.TotalMessages++
	bb.metrics.mu.Unlock()

	// If batching is disabled, send immediately
	if !bb.config.EnableBatching {
		bb.metrics.mu.Lock()
		bb.metrics.ImmediateMessages++
		bb.metrics.mu.Unlock()
		return bb.inner.Broadcast(ctx, channelID, msg)
	}

	// Check for priority bypass
	if bb.config.PriorityBypass && isPriorityMessage(msg) {
		bb.metrics.mu.Lock()
		bb.metrics.ImmediateMessages++
		bb.metrics.mu.Unlock()
		return bb.inner.Broadcast(ctx, channelID, msg)
	}

	// Add to batch
	bb.mu.Lock()
	batch, exists := bb.batches[channelID]
	if !exists {
		batch = &MessageBatch{
			ChannelID: channelID,
			Messages:  make([]*chatv1.Message, 0, bb.config.MaxBatchSize),
			CreatedAt: time.Now(),
		}
		bb.batches[channelID] = batch
	}

	batch.Messages = append(batch.Messages, msg)
	bb.metrics.mu.Lock()
	bb.metrics.BatchedMessages++
	bb.metrics.mu.Unlock()

	// Check if batch should be flushed
	shouldFlush := len(batch.Messages) >= bb.config.MaxBatchSize

	bb.mu.Unlock()

	if shouldFlush {
		select {
		case bb.flushCh <- channelID:
		default:
			// Channel full, will be handled by timer
		}
	}

	return nil
}

// flushLoop handles flush requests
func (bb *BatchedBroadcaster) flushLoop() {
	defer bb.wg.Done()

	ctx := context.Background()

	for {
		select {
		case <-bb.stopCh:
			return
		case channelID := <-bb.flushCh:
			bb.mu.Lock()
			bb.flushBatchLocked(ctx, channelID)
			bb.mu.Unlock()
		}
	}
}

// timerLoop periodically flushes batches that have waited too long
func (bb *BatchedBroadcaster) timerLoop() {
	defer bb.wg.Done()

	ticker := time.NewTicker(bb.config.FlushInterval)
	defer ticker.Stop()

	ctx := context.Background()

	for {
		select {
		case <-bb.stopCh:
			return
		case <-ticker.C:
			bb.flushExpiredBatches(ctx)
		}
	}
}

// flushExpiredBatches flushes batches that have exceeded MaxBatchWait
func (bb *BatchedBroadcaster) flushExpiredBatches(ctx context.Context) {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	now := time.Now()
	for channelID, batch := range bb.batches {
		if len(batch.Messages) > 0 && now.Sub(batch.CreatedAt) >= bb.config.MaxBatchWait {
			bb.flushBatchLocked(ctx, channelID)
		}
	}
}

// flushBatchLocked sends all messages in a batch (must hold lock)
func (bb *BatchedBroadcaster) flushBatchLocked(ctx context.Context, channelID string) {
	batch, exists := bb.batches[channelID]
	if !exists || len(batch.Messages) == 0 {
		return
	}

	// Send all messages in the batch
	for _, msg := range batch.Messages {
		_ = bb.inner.Broadcast(ctx, channelID, msg)
	}

	// Update metrics
	bb.metrics.mu.Lock()
	bb.metrics.BatchesFlushed++
	totalBatched := float64(bb.metrics.BatchedMessages)
	totalFlushed := float64(bb.metrics.BatchesFlushed)
	if totalFlushed > 0 {
		bb.metrics.AverageBatchSize = totalBatched / totalFlushed
	}
	bb.metrics.mu.Unlock()

	// Reset batch
	batch.Messages = batch.Messages[:0]
	batch.CreatedAt = time.Now()
}

// Flush immediately flushes all pending batches
func (bb *BatchedBroadcaster) Flush(ctx context.Context) {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	for channelID := range bb.batches {
		bb.flushBatchLocked(ctx, channelID)
	}
}

// FlushChannel flushes messages for a specific channel
func (bb *BatchedBroadcaster) FlushChannel(ctx context.Context, channelID string) {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	bb.flushBatchLocked(ctx, channelID)
}

// GetMetrics returns current batching metrics
func (bb *BatchedBroadcaster) GetMetrics() BatchMetrics {
	bb.metrics.mu.RLock()
	defer bb.metrics.mu.RUnlock()

	return BatchMetrics{
		TotalMessages:     bb.metrics.TotalMessages,
		BatchedMessages:   bb.metrics.BatchedMessages,
		ImmediateMessages: bb.metrics.ImmediateMessages,
		BatchesFlushed:    bb.metrics.BatchesFlushed,
		AverageBatchSize:  bb.metrics.AverageBatchSize,
	}
}

// GetPendingCount returns the number of pending messages across all batches
func (bb *BatchedBroadcaster) GetPendingCount() int {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	count := 0
	for _, batch := range bb.batches {
		count += len(batch.Messages)
	}
	return count
}

// isPriorityMessage checks if a message should bypass batching
func isPriorityMessage(msg *chatv1.Message) bool {
	// System messages always bypass
	if msg.Type == chatv1.MessageType_MESSAGE_TYPE_SYSTEM {
		return true
	}

	// Check for urgent flag
	for _, flag := range msg.Flags {
		if flag == chatv1.MessageFlag_MESSAGE_FLAG_URGENT ||
			flag == chatv1.MessageFlag_MESSAGE_FLAG_BROADCAST {
			return true
		}
	}

	return false
}

// SendToUser sends a message directly to a specific user (not batched)
func (bb *BatchedBroadcaster) SendToUser(ctx context.Context, userID string, msg *chatv1.Message) error {
	return bb.inner.SendToUser(ctx, userID, msg)
}

// Subscribe subscribes to messages for a channel
func (bb *BatchedBroadcaster) Subscribe(ctx context.Context, channelID string) (<-chan *chatv1.Message, error) {
	return bb.inner.Subscribe(ctx, channelID)
}

// Unsubscribe removes a channel subscription
func (bb *BatchedBroadcaster) Unsubscribe(ctx context.Context, channelID string) error {
	return bb.inner.Unsubscribe(ctx, channelID)
}

// Ensure BatchedBroadcaster implements MessageBroadcaster
var _ MessageBroadcaster = (*BatchedBroadcaster)(nil)
