// Package chat provides the core chat server functionality
package chat

import (
	"sync"

	chatv1 "Hermes/gen/chat/v1"
	"Hermes/gen/chat/v1/chatv1connect"
)

// Server handles chat operations including channels and message broadcasting
type Server struct {
	chatv1connect.UnimplementedChatServiceHandler

	// channels stores all active channels by ID
	channels map[string]*chatv1.Channel

	// userStreams maps user IDs to their message channels
	userStreams map[string]chan *chatv1.Message

	// mu protects concurrent access to channels and userStreams
	mu sync.RWMutex
}

// NewServer creates a new chat server with default channels
func NewServer() *Server {
	channels := make(map[string]*chatv1.Channel)
	channels["general"] = &chatv1.Channel{
		Id:    "general",
		Label: "General",
		Users: []*chatv1.User{},
	}

	return &Server{
		channels:    channels,
		userStreams: make(map[string]chan *chatv1.Message),
	}
}

// GetChannel returns a channel by ID, creating it if it doesn't exist
func (s *Server) GetChannel(id string) (*chatv1.Channel, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ch, exists := s.channels[id]
	return ch, exists
}

// GetOrCreateChannel returns an existing channel or creates a new one
func (s *Server) GetOrCreateChannel(channel *chatv1.Channel) *chatv1.Channel {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, exists := s.channels[channel.Id]; exists {
		return existing
	}

	s.channels[channel.Id] = channel
	return channel
}

// AddUserToChannel adds a user to a channel
func (s *Server) AddUserToChannel(channelID string, user *chatv1.User) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ch, exists := s.channels[channelID]; exists {
		ch.Users = append(ch.Users, user)
	}
}

// RegisterUserStream creates a message stream for a user
func (s *Server) RegisterUserStream(userID string) chan *chatv1.Message {
	s.mu.Lock()
	defer s.mu.Unlock()

	stream := make(chan *chatv1.Message, 10)
	s.userStreams[userID] = stream
	return stream
}

// UnregisterUserStream removes a user's message stream
func (s *Server) UnregisterUserStream(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.userStreams, userID)
}

// BroadcastToChannel sends a message to all users in a channel
func (s *Server) BroadcastToChannel(channelID string, msg *chatv1.Message) error {
	s.mu.RLock()
	ch, exists := s.channels[channelID]
	if !exists {
		s.mu.RUnlock()
		return ErrChannelNotFound
	}

	// Copy user IDs while holding the lock
	userIDs := make([]string, len(ch.Users))
	for i, u := range ch.Users {
		userIDs[i] = u.Id
	}
	s.mu.RUnlock()

	// Send messages without holding the lock (channel sends can block)
	for _, userID := range userIDs {
		s.mu.RLock()
		userStream, ok := s.userStreams[userID]
		s.mu.RUnlock()
		if ok {
			select {
			case userStream <- msg:
			default:
				// Channel full, skip to avoid blocking
			}
		}
	}

	return nil
}
