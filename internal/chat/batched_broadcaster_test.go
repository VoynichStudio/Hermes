package chat

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	chatv1 "Hermes/gen/chat/v1"
)

// mockBroadcaster is a simple mock for testing
type mockBroadcaster struct {
	messages []*chatv1.Message
	mu       sync.Mutex
	count    atomic.Int32
}

func newMockBroadcaster() *mockBroadcaster {
	return &mockBroadcaster{
		messages: make([]*chatv1.Message, 0),
	}
}

func (m *mockBroadcaster) Broadcast(ctx context.Context, channelID string, msg *chatv1.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	m.count.Add(1)
	return nil
}

func (m *mockBroadcaster) SendToUser(ctx context.Context, userID string, msg *chatv1.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	m.count.Add(1)
	return nil
}

func (m *mockBroadcaster) Subscribe(ctx context.Context, channelID string) (<-chan *chatv1.Message, error) {
	return make(chan *chatv1.Message), nil
}

func (m *mockBroadcaster) Unsubscribe(ctx context.Context, channelID string) error {
	return nil
}

func (m *mockBroadcaster) GetMessages() []*chatv1.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]*chatv1.Message, len(m.messages))
	copy(result, m.messages)
	return result
}

func (m *mockBroadcaster) GetCount() int {
	return int(m.count.Load())
}

func TestBatchedBroadcaster_Disabled(t *testing.T) {
	ctx := context.Background()
	inner := newMockBroadcaster()

	config := &BatchConfig{
		EnableBatching: false,
	}

	bb := NewBatchedBroadcaster(inner, config)

	// Messages should be sent immediately
	for i := 0; i < 10; i++ {
		msg := &chatv1.Message{Id: string(rune('a' + i)), Content: "test"}
		err := bb.Broadcast(ctx, "channel-1", msg)
		if err != nil {
			t.Fatalf("Broadcast failed: %v", err)
		}
	}

	if inner.GetCount() != 10 {
		t.Errorf("Expected 10 immediate messages, got %d", inner.GetCount())
	}
}

func TestBatchedBroadcaster_BatchBySize(t *testing.T) {
	ctx := context.Background()
	inner := newMockBroadcaster()

	config := &BatchConfig{
		MaxBatchSize:   5,
		MaxBatchWait:   1 * time.Hour, // Long wait to ensure size triggers flush
		FlushInterval:  10 * time.Millisecond,
		EnableBatching: true,
		PriorityBypass: false,
	}

	bb := NewBatchedBroadcaster(inner, config)
	bb.Start()
	defer bb.Stop()

	// Send 5 messages (should trigger batch flush)
	for i := 0; i < 5; i++ {
		msg := &chatv1.Message{Id: string(rune('a' + i)), Content: "test"}
		err := bb.Broadcast(ctx, "channel-1", msg)
		if err != nil {
			t.Fatalf("Broadcast failed: %v", err)
		}
	}

	// Wait for flush
	time.Sleep(50 * time.Millisecond)

	if inner.GetCount() != 5 {
		t.Errorf("Expected 5 batched messages, got %d", inner.GetCount())
	}
}

func TestBatchedBroadcaster_BatchByTime(t *testing.T) {
	ctx := context.Background()
	inner := newMockBroadcaster()

	config := &BatchConfig{
		MaxBatchSize:   100, // Large batch size
		MaxBatchWait:   30 * time.Millisecond,
		FlushInterval:  10 * time.Millisecond,
		EnableBatching: true,
		PriorityBypass: false,
	}

	bb := NewBatchedBroadcaster(inner, config)
	bb.Start()
	defer bb.Stop()

	// Send 3 messages (less than batch size)
	for i := 0; i < 3; i++ {
		msg := &chatv1.Message{Id: string(rune('a' + i)), Content: "test"}
		err := bb.Broadcast(ctx, "channel-1", msg)
		if err != nil {
			t.Fatalf("Broadcast failed: %v", err)
		}
	}

	// Initially no messages should be sent
	if inner.GetCount() != 0 {
		t.Errorf("Expected 0 messages before timeout, got %d", inner.GetCount())
	}

	// Wait for time-based flush
	time.Sleep(60 * time.Millisecond)

	if inner.GetCount() != 3 {
		t.Errorf("Expected 3 messages after timeout, got %d", inner.GetCount())
	}
}

func TestBatchedBroadcaster_PriorityBypass(t *testing.T) {
	ctx := context.Background()
	inner := newMockBroadcaster()

	config := &BatchConfig{
		MaxBatchSize:   100,
		MaxBatchWait:   1 * time.Hour,
		FlushInterval:  10 * time.Millisecond,
		EnableBatching: true,
		PriorityBypass: true,
	}

	bb := NewBatchedBroadcaster(inner, config)
	bb.Start()
	defer bb.Stop()

	// Send a system message (should bypass batching)
	msg := &chatv1.Message{
		Id:      "sys-1",
		Content: "System announcement",
		Type:    chatv1.MessageType_MESSAGE_TYPE_SYSTEM,
	}
	err := bb.Broadcast(ctx, "channel-1", msg)
	if err != nil {
		t.Fatalf("Broadcast failed: %v", err)
	}

	// Should be sent immediately
	if inner.GetCount() != 1 {
		t.Errorf("Expected 1 immediate message for system type, got %d", inner.GetCount())
	}

	// Send an urgent flagged message
	urgentMsg := &chatv1.Message{
		Id:      "urgent-1",
		Content: "Urgent message",
		Flags:   []chatv1.MessageFlag{chatv1.MessageFlag_MESSAGE_FLAG_URGENT},
	}
	err = bb.Broadcast(ctx, "channel-1", urgentMsg)
	if err != nil {
		t.Fatalf("Broadcast failed: %v", err)
	}

	// Should be sent immediately
	if inner.GetCount() != 2 {
		t.Errorf("Expected 2 immediate messages, got %d", inner.GetCount())
	}
}

func TestBatchedBroadcaster_MultipleChannels(t *testing.T) {
	ctx := context.Background()
	inner := newMockBroadcaster()

	config := &BatchConfig{
		MaxBatchSize:   3,
		MaxBatchWait:   1 * time.Hour,
		FlushInterval:  10 * time.Millisecond,
		EnableBatching: true,
		PriorityBypass: false,
	}

	bb := NewBatchedBroadcaster(inner, config)
	bb.Start()
	defer bb.Stop()

	// Send 3 messages to channel-1 (should trigger flush)
	for i := 0; i < 3; i++ {
		msg := &chatv1.Message{Id: "ch1-" + string(rune('a'+i)), Content: "test"}
		_ = bb.Broadcast(ctx, "channel-1", msg)
	}

	// Send 2 messages to channel-2 (should not flush yet)
	for i := 0; i < 2; i++ {
		msg := &chatv1.Message{Id: "ch2-" + string(rune('a'+i)), Content: "test"}
		_ = bb.Broadcast(ctx, "channel-2", msg)
	}

	time.Sleep(50 * time.Millisecond)

	// Only channel-1 should have flushed
	if inner.GetCount() != 3 {
		t.Errorf("Expected 3 messages (only channel-1 flushed), got %d", inner.GetCount())
	}

	// Check pending count for channel-2
	pending := bb.GetPendingCount()
	if pending != 2 {
		t.Errorf("Expected 2 pending messages, got %d", pending)
	}
}

func TestBatchedBroadcaster_ManualFlush(t *testing.T) {
	ctx := context.Background()
	inner := newMockBroadcaster()

	config := &BatchConfig{
		MaxBatchSize:   100,
		MaxBatchWait:   1 * time.Hour,
		FlushInterval:  1 * time.Hour,
		EnableBatching: true,
		PriorityBypass: false,
	}

	bb := NewBatchedBroadcaster(inner, config)
	// Don't start background loops for this test

	// Send messages
	for i := 0; i < 5; i++ {
		msg := &chatv1.Message{Id: string(rune('a' + i)), Content: "test"}
		_ = bb.Broadcast(ctx, "channel-1", msg)
	}

	// Nothing should be sent yet
	if inner.GetCount() != 0 {
		t.Errorf("Expected 0 messages before flush, got %d", inner.GetCount())
	}

	// Manual flush
	bb.Flush(ctx)

	if inner.GetCount() != 5 {
		t.Errorf("Expected 5 messages after flush, got %d", inner.GetCount())
	}
}

func TestBatchedBroadcaster_FlushChannel(t *testing.T) {
	ctx := context.Background()
	inner := newMockBroadcaster()

	config := &BatchConfig{
		MaxBatchSize:   100,
		MaxBatchWait:   1 * time.Hour,
		FlushInterval:  1 * time.Hour,
		EnableBatching: true,
		PriorityBypass: false,
	}

	bb := NewBatchedBroadcaster(inner, config)

	// Send messages to two channels
	for i := 0; i < 3; i++ {
		_ = bb.Broadcast(ctx, "channel-1", &chatv1.Message{Id: "c1-" + string(rune('a'+i))})
		_ = bb.Broadcast(ctx, "channel-2", &chatv1.Message{Id: "c2-" + string(rune('a'+i))})
	}

	// Flush only channel-1
	bb.FlushChannel(ctx, "channel-1")

	if inner.GetCount() != 3 {
		t.Errorf("Expected 3 messages after FlushChannel, got %d", inner.GetCount())
	}

	// Channel-2 should still have pending
	if bb.GetPendingCount() != 3 {
		t.Errorf("Expected 3 pending messages, got %d", bb.GetPendingCount())
	}
}

func TestBatchedBroadcaster_Metrics(t *testing.T) {
	ctx := context.Background()
	inner := newMockBroadcaster()

	config := &BatchConfig{
		MaxBatchSize:   5,
		MaxBatchWait:   1 * time.Hour,
		FlushInterval:  10 * time.Millisecond,
		EnableBatching: true,
		PriorityBypass: true,
	}

	bb := NewBatchedBroadcaster(inner, config)
	bb.Start()
	defer bb.Stop()

	// Send a system message (immediate)
	_ = bb.Broadcast(ctx, "channel-1", &chatv1.Message{
		Id:   "sys",
		Type: chatv1.MessageType_MESSAGE_TYPE_SYSTEM,
	})

	// Send 5 regular messages (batched)
	for i := 0; i < 5; i++ {
		_ = bb.Broadcast(ctx, "channel-1", &chatv1.Message{Id: string(rune('a' + i))})
	}

	time.Sleep(50 * time.Millisecond)

	metrics := bb.GetMetrics()

	if metrics.TotalMessages != 6 {
		t.Errorf("Expected TotalMessages=6, got %d", metrics.TotalMessages)
	}

	if metrics.ImmediateMessages != 1 {
		t.Errorf("Expected ImmediateMessages=1, got %d", metrics.ImmediateMessages)
	}

	if metrics.BatchedMessages != 5 {
		t.Errorf("Expected BatchedMessages=5, got %d", metrics.BatchedMessages)
	}

	if metrics.BatchesFlushed < 1 {
		t.Errorf("Expected at least 1 batch flushed, got %d", metrics.BatchesFlushed)
	}
}

func TestBatchedBroadcaster_StopFlushesRemaining(t *testing.T) {
	ctx := context.Background()
	inner := newMockBroadcaster()

	config := &BatchConfig{
		MaxBatchSize:   100,
		MaxBatchWait:   1 * time.Hour,
		FlushInterval:  1 * time.Hour,
		EnableBatching: true,
		PriorityBypass: false,
	}

	bb := NewBatchedBroadcaster(inner, config)
	bb.Start()

	// Send messages
	for i := 0; i < 5; i++ {
		_ = bb.Broadcast(ctx, "channel-1", &chatv1.Message{Id: string(rune('a' + i))})
	}

	// Stop should flush remaining
	bb.Stop()

	if inner.GetCount() != 5 {
		t.Errorf("Expected 5 messages after Stop, got %d", inner.GetCount())
	}
}

func TestBatchedBroadcaster_Concurrent(t *testing.T) {
	ctx := context.Background()
	inner := newMockBroadcaster()

	config := &BatchConfig{
		MaxBatchSize:   10,
		MaxBatchWait:   50 * time.Millisecond,
		FlushInterval:  10 * time.Millisecond,
		EnableBatching: true,
		PriorityBypass: true,
	}

	bb := NewBatchedBroadcaster(inner, config)
	bb.Start()
	defer bb.Stop()

	const numGoroutines = 10
	const messagesPerGoroutine = 100
	totalExpected := numGoroutines * messagesPerGoroutine

	var wg sync.WaitGroup

	for g := 0; g < numGoroutines; g++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for i := 0; i < messagesPerGoroutine; i++ {
				msg := &chatv1.Message{
					Id:      string(rune('A' + goroutineID)),
					Content: "test",
				}
				_ = bb.Broadcast(ctx, "channel-1", msg)
			}
		}(g)
	}

	wg.Wait()

	// Wait for all batches to flush
	time.Sleep(100 * time.Millisecond)
	bb.Flush(ctx)

	if inner.GetCount() != totalExpected {
		t.Errorf("Expected %d messages, got %d", totalExpected, inner.GetCount())
	}
}

func TestIsPriorityMessage(t *testing.T) {
	tests := []struct {
		name     string
		msg      *chatv1.Message
		expected bool
	}{
		{
			name: "regular message",
			msg:  &chatv1.Message{Type: chatv1.MessageType_MESSAGE_TYPE_CHAT},
			expected: false,
		},
		{
			name: "system message",
			msg:  &chatv1.Message{Type: chatv1.MessageType_MESSAGE_TYPE_SYSTEM},
			expected: true,
		},
		{
			name: "urgent flag",
			msg: &chatv1.Message{
				Flags: []chatv1.MessageFlag{chatv1.MessageFlag_MESSAGE_FLAG_URGENT},
			},
			expected: true,
		},
		{
			name: "broadcast flag",
			msg: &chatv1.Message{
				Flags: []chatv1.MessageFlag{chatv1.MessageFlag_MESSAGE_FLAG_BROADCAST},
			},
			expected: true,
		},
		{
			name: "pinned flag (not priority)",
			msg: &chatv1.Message{
				Flags: []chatv1.MessageFlag{chatv1.MessageFlag_MESSAGE_FLAG_PINNED},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPriorityMessage(tt.msg)
			if result != tt.expected {
				t.Errorf("isPriorityMessage: expected %v, got %v", tt.expected, result)
			}
		})
	}
}
