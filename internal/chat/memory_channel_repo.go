package chat

import (
	"context"
	"sync"

	chatv1 "Hermes/gen/chat/v1"
)

// MemoryChannelRepository is an in-memory implementation of ChannelRepository
type MemoryChannelRepository struct {
	channels map[string]*chatv1.Channel
	mu       sync.RWMutex
}

// NewMemoryChannelRepository creates a new in-memory channel repository
func NewMemoryChannelRepository() *MemoryChannelRepository {
	return &MemoryChannelRepository{
		channels: make(map[string]*chatv1.Channel),
	}
}

// Get retrieves a channel by ID
func (r *MemoryChannelRepository) Get(ctx context.Context, id string) (*chatv1.Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ch, exists := r.channels[id]
	if !exists {
		return nil, ErrChannelNotFound
	}
	return ch, nil
}

// Create creates a new channel
func (r *MemoryChannelRepository) Create(ctx context.Context, channel *chatv1.Channel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.channels[channel.Id]; exists {
		return ErrChannelExists
	}

	r.channels[channel.Id] = channel
	return nil
}

// GetOrCreate returns an existing channel or creates a new one
func (r *MemoryChannelRepository) GetOrCreate(ctx context.Context, channel *chatv1.Channel) (*chatv1.Channel, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if existing, exists := r.channels[channel.Id]; exists {
		return existing, nil
	}

	// Initialize Users slice if nil
	if channel.Users == nil {
		channel.Users = []*chatv1.User{}
	}

	r.channels[channel.Id] = channel
	return channel, nil
}

// Delete removes a channel
func (r *MemoryChannelRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.channels[id]; !exists {
		return ErrChannelNotFound
	}

	delete(r.channels, id)
	return nil
}

// List returns all channels
func (r *MemoryChannelRepository) List(ctx context.Context) ([]*chatv1.Channel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	channels := make([]*chatv1.Channel, 0, len(r.channels))
	for _, ch := range r.channels {
		channels = append(channels, ch)
	}
	return channels, nil
}

// AddUser adds a user to a channel
func (r *MemoryChannelRepository) AddUser(ctx context.Context, channelID string, user *chatv1.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch, exists := r.channels[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	// Check if user is already in the channel
	for _, u := range ch.Users {
		if u.Id == user.Id {
			return nil // User already in channel, not an error
		}
	}

	ch.Users = append(ch.Users, user)
	return nil
}

// RemoveUser removes a user from a channel
func (r *MemoryChannelRepository) RemoveUser(ctx context.Context, channelID string, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch, exists := r.channels[channelID]
	if !exists {
		return ErrChannelNotFound
	}

	for i, u := range ch.Users {
		if u.Id == userID {
			ch.Users = append(ch.Users[:i], ch.Users[i+1:]...)
			return nil
		}
	}

	return ErrUserNotInChannel
}

// GetUsers returns all users in a channel
func (r *MemoryChannelRepository) GetUsers(ctx context.Context, channelID string) ([]*chatv1.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ch, exists := r.channels[channelID]
	if !exists {
		return nil, ErrChannelNotFound
	}

	// Return a copy to avoid race conditions
	users := make([]*chatv1.User, len(ch.Users))
	copy(users, ch.Users)
	return users, nil
}

// Ensure MemoryChannelRepository implements ChannelRepository
var _ ChannelRepository = (*MemoryChannelRepository)(nil)
