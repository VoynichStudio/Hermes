// Package chat provides the core chat server functionality
package chat

import (
	"context"

	chatv1 "Hermes/gen/chat/v1"
)

// ChannelRepository defines operations for channel storage
type ChannelRepository interface {
	// Get retrieves a channel by ID
	Get(ctx context.Context, id string) (*chatv1.Channel, error)

	// Create creates a new channel
	Create(ctx context.Context, channel *chatv1.Channel) error

	// GetOrCreate returns an existing channel or creates a new one
	GetOrCreate(ctx context.Context, channel *chatv1.Channel) (*chatv1.Channel, error)

	// Delete removes a channel
	Delete(ctx context.Context, id string) error

	// List returns all channels
	List(ctx context.Context) ([]*chatv1.Channel, error)

	// AddUser adds a user to a channel
	AddUser(ctx context.Context, channelID string, user *chatv1.User) error

	// RemoveUser removes a user from a channel
	RemoveUser(ctx context.Context, channelID string, userID string) error

	// GetUsers returns all users in a channel
	GetUsers(ctx context.Context, channelID string) ([]*chatv1.User, error)
}

// MessageRepository defines operations for message storage
type MessageRepository interface {
	// Save stores a message
	Save(ctx context.Context, msg *chatv1.Message) error

	// GetByChannel retrieves messages for a channel with pagination
	GetByChannel(ctx context.Context, channelID string, limit int, cursor string) ([]*chatv1.Message, string, error)

	// GetByID retrieves a specific message
	GetByID(ctx context.Context, id string) (*chatv1.Message, error)

	// Delete removes a message
	Delete(ctx context.Context, id string) error
}

// UserSession represents an active user connection
type UserSession struct {
	UserID     string
	User       *chatv1.User
	ChannelIDs []string
	ServerID   string // Which server instance this session is on
}

// UserSessionManager defines operations for managing user sessions
type UserSessionManager interface {
	// Register creates a new session for a user and returns a message channel
	Register(ctx context.Context, user *chatv1.User) (chan *chatv1.Message, error)

	// Unregister removes a user's session
	Unregister(ctx context.Context, userID string) error

	// Get retrieves a user's session
	Get(ctx context.Context, userID string) (*UserSession, error)

	// GetMessageChannel returns the message channel for a user
	GetMessageChannel(ctx context.Context, userID string) (chan *chatv1.Message, error)

	// SendMessage safely sends a message to a user (handles closed channels)
	SendMessage(ctx context.Context, userID string, msg *chatv1.Message) bool

	// AddChannel adds a channel subscription to a user's session
	AddChannel(ctx context.Context, userID string, channelID string) error

	// RemoveChannel removes a channel subscription from a user's session
	RemoveChannel(ctx context.Context, userID string, channelID string) error

	// IsOnline checks if a user is currently connected
	IsOnline(ctx context.Context, userID string) (bool, error)
}

// MessageBroadcaster defines operations for distributing messages
type MessageBroadcaster interface {
	// Broadcast sends a message to all users in a channel
	Broadcast(ctx context.Context, channelID string, msg *chatv1.Message) error

	// SendToUser sends a message directly to a specific user
	SendToUser(ctx context.Context, userID string, msg *chatv1.Message) error

	// Subscribe subscribes to messages for a channel (used by server instances)
	Subscribe(ctx context.Context, channelID string) (<-chan *chatv1.Message, error)

	// Unsubscribe removes a channel subscription
	Unsubscribe(ctx context.Context, channelID string) error
}
