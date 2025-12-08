// Package chat provides the core chat server functionality
package chat

import (
	"context"

	chatv1 "Hermes/gen/chat/v1"
	"Hermes/gen/chat/v1/chatv1connect"
)

// Server handles chat operations including channels and message broadcasting
type Server struct {
	chatv1connect.UnimplementedChatServiceHandler

	// Dependencies injected via constructor
	channelRepo    ChannelRepository
	messageRepo    MessageRepository
	sessionManager UserSessionManager
	broadcaster    MessageBroadcaster
}

// ServerConfig holds configuration for creating a new server
type ServerConfig struct {
	ChannelRepo    ChannelRepository
	MessageRepo    MessageRepository
	SessionManager UserSessionManager
	Broadcaster    MessageBroadcaster
}

// NewServer creates a new chat server with the provided dependencies
func NewServer(cfg ServerConfig) *Server {
	return &Server{
		channelRepo:    cfg.ChannelRepo,
		messageRepo:    cfg.MessageRepo,
		sessionManager: cfg.SessionManager,
		broadcaster:    cfg.Broadcaster,
	}
}

// NewServerWithDefaults creates a new chat server with default in-memory implementations
func NewServerWithDefaults() *Server {
	channelRepo := NewMemoryChannelRepository()
	messageRepo := NewMemoryMessageRepository()
	sessionManager := NewMemoryUserSessionManager("local")
	broadcaster := NewMemoryMessageBroadcaster(channelRepo, sessionManager, messageRepo)

	// Create default general channel
	ctx := context.Background()
	_, _ = channelRepo.GetOrCreate(ctx, &chatv1.Channel{
		Id:    "general",
		Label: "General",
		Users: []*chatv1.User{},
	})

	return NewServer(ServerConfig{
		ChannelRepo:    channelRepo,
		MessageRepo:    messageRepo,
		SessionManager: sessionManager,
		Broadcaster:    broadcaster,
	})
}

// ChannelRepo returns the channel repository (for testing)
func (s *Server) ChannelRepo() ChannelRepository {
	return s.channelRepo
}

// MessageRepo returns the message repository (for testing)
func (s *Server) MessageRepo() MessageRepository {
	return s.messageRepo
}

// SessionManager returns the session manager (for testing)
func (s *Server) SessionManager() UserSessionManager {
	return s.sessionManager
}

// Broadcaster returns the message broadcaster (for testing)
func (s *Server) Broadcaster() MessageBroadcaster {
	return s.broadcaster
}
