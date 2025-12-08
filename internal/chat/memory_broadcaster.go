package chat

import (
	"context"
	"sync"

	chatv1 "Hermes/gen/chat/v1"
)

// MemoryMessageBroadcaster is an in-memory implementation of MessageBroadcaster
// It broadcasts messages directly to user sessions within the same process
type MemoryMessageBroadcaster struct {
	channelRepo    ChannelRepository
	sessionManager UserSessionManager
	messageRepo    MessageRepository

	// subscriptions for channel listeners (for testing/internal use)
	subscriptions map[string][]chan *chatv1.Message
	mu            sync.RWMutex
}

// NewMemoryMessageBroadcaster creates a new in-memory message broadcaster
func NewMemoryMessageBroadcaster(
	channelRepo ChannelRepository,
	sessionManager UserSessionManager,
	messageRepo MessageRepository,
) *MemoryMessageBroadcaster {
	return &MemoryMessageBroadcaster{
		channelRepo:    channelRepo,
		sessionManager: sessionManager,
		messageRepo:    messageRepo,
		subscriptions:  make(map[string][]chan *chatv1.Message),
	}
}

// Broadcast sends a message to all users in a channel
func (b *MemoryMessageBroadcaster) Broadcast(ctx context.Context, channelID string, msg *chatv1.Message) error {
	// Get all users in the channel
	users, err := b.channelRepo.GetUsers(ctx, channelID)
	if err != nil {
		return err
	}

	// Optionally save the message for history
	if b.messageRepo != nil && msg.Id != "" {
		_ = b.messageRepo.Save(ctx, msg)
	}

	// Send to all users in the channel
	for _, user := range users {
		msgChan, err := b.sessionManager.GetMessageChannel(ctx, user.Id)
		if err != nil {
			// User not connected, skip
			continue
		}

		// Safe non-blocking send (handles closed channels)
		safeSend(msgChan, msg)
	}

	// Also send to any direct subscribers
	b.mu.RLock()
	subs := b.subscriptions[channelID]
	b.mu.RUnlock()

	for _, sub := range subs {
		safeSend(sub, msg)
	}

	return nil
}

// safeSend sends a message to a channel without blocking or panicking on closed channel
func safeSend(ch chan *chatv1.Message, msg *chatv1.Message) (sent bool) {
	defer func() {
		if recover() != nil {
			// Channel was closed, that's okay
			sent = false
		}
	}()

	select {
	case ch <- msg:
		return true
	default:
		// Channel full
		return false
	}
}

// SendToUser sends a message directly to a specific user
func (b *MemoryMessageBroadcaster) SendToUser(ctx context.Context, userID string, msg *chatv1.Message) error {
	msgChan, err := b.sessionManager.GetMessageChannel(ctx, userID)
	if err != nil {
		return err
	}

	// Non-blocking send
	select {
	case msgChan <- msg:
		return nil
	default:
		// Channel full - could return an error or just drop
		return nil
	}
}

// Subscribe subscribes to messages for a channel
func (b *MemoryMessageBroadcaster) Subscribe(ctx context.Context, channelID string) (<-chan *chatv1.Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan *chatv1.Message, 100)
	b.subscriptions[channelID] = append(b.subscriptions[channelID], ch)
	return ch, nil
}

// Unsubscribe removes a channel subscription
func (b *MemoryMessageBroadcaster) Unsubscribe(ctx context.Context, channelID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Close all subscriptions for this channel
	if subs, exists := b.subscriptions[channelID]; exists {
		for _, sub := range subs {
			close(sub)
		}
		delete(b.subscriptions, channelID)
	}
	return nil
}

// Ensure MemoryMessageBroadcaster implements MessageBroadcaster
var _ MessageBroadcaster = (*MemoryMessageBroadcaster)(nil)
