package chat

import (
	"context"
	"sort"
	"sync"

	chatv1 "Hermes/gen/chat/v1"
)

// MemoryMessageRepository is an in-memory implementation of MessageRepository
type MemoryMessageRepository struct {
	// messages stored by ID
	messages map[string]*chatv1.Message
	// channelMessages maps channelID to ordered message IDs
	channelMessages map[string][]string
	mu              sync.RWMutex
}

// NewMemoryMessageRepository creates a new in-memory message repository
func NewMemoryMessageRepository() *MemoryMessageRepository {
	return &MemoryMessageRepository{
		messages:        make(map[string]*chatv1.Message),
		channelMessages: make(map[string][]string),
	}
}

// Save stores a message
func (r *MemoryMessageRepository) Save(ctx context.Context, msg *chatv1.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Store the message
	r.messages[msg.Id] = msg

	// Add to channel's message list
	r.channelMessages[msg.ChannelId] = append(r.channelMessages[msg.ChannelId], msg.Id)

	return nil
}

// GetByChannel retrieves messages for a channel with pagination
// Returns messages, next cursor, and error
func (r *MemoryMessageRepository) GetByChannel(ctx context.Context, channelID string, limit int, cursor string) ([]*chatv1.Message, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	msgIDs, exists := r.channelMessages[channelID]
	if !exists || len(msgIDs) == 0 {
		return []*chatv1.Message{}, "", nil
	}

	// Find start index based on cursor
	startIdx := 0
	if cursor != "" {
		for i, id := range msgIDs {
			if id == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	// If start is beyond available messages, return empty
	if startIdx >= len(msgIDs) {
		return []*chatv1.Message{}, "", nil
	}

	// Determine end index
	endIdx := startIdx + limit
	if endIdx > len(msgIDs) {
		endIdx = len(msgIDs)
	}

	// Collect messages
	messages := make([]*chatv1.Message, 0, endIdx-startIdx)
	for i := startIdx; i < endIdx; i++ {
		if msg, ok := r.messages[msgIDs[i]]; ok {
			messages = append(messages, msg)
		}
	}

	// Determine next cursor
	nextCursor := ""
	if endIdx < len(msgIDs) {
		nextCursor = msgIDs[endIdx-1]
	}

	return messages, nextCursor, nil
}

// GetByID retrieves a specific message
func (r *MemoryMessageRepository) GetByID(ctx context.Context, id string) (*chatv1.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	msg, exists := r.messages[id]
	if !exists {
		return nil, ErrMessageNotFound
	}
	return msg, nil
}

// Delete removes a message
func (r *MemoryMessageRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	msg, exists := r.messages[id]
	if !exists {
		return ErrMessageNotFound
	}

	// Remove from channel's message list
	channelID := msg.ChannelId
	msgIDs := r.channelMessages[channelID]
	for i, msgID := range msgIDs {
		if msgID == id {
			r.channelMessages[channelID] = append(msgIDs[:i], msgIDs[i+1:]...)
			break
		}
	}

	// Remove the message itself
	delete(r.messages, id)
	return nil
}

// GetRecentByChannel retrieves the most recent messages for a channel
func (r *MemoryMessageRepository) GetRecentByChannel(ctx context.Context, channelID string, limit int) ([]*chatv1.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	msgIDs, exists := r.channelMessages[channelID]
	if !exists || len(msgIDs) == 0 {
		return []*chatv1.Message{}, nil
	}

	// Get the most recent messages (from the end)
	startIdx := len(msgIDs) - limit
	if startIdx < 0 {
		startIdx = 0
	}

	messages := make([]*chatv1.Message, 0, len(msgIDs)-startIdx)
	for i := startIdx; i < len(msgIDs); i++ {
		if msg, ok := r.messages[msgIDs[i]]; ok {
			messages = append(messages, msg)
		}
	}

	// Sort by timestamp (oldest first for display)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp < messages[j].Timestamp
	})

	return messages, nil
}

// Ensure MemoryMessageRepository implements MessageRepository
var _ MessageRepository = (*MemoryMessageRepository)(nil)
