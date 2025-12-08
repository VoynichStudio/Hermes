package chat

import (
	"context"
	"sync"

	chatv1 "Hermes/gen/chat/v1"
)

// MemoryUserSessionManager is an in-memory implementation of UserSessionManager
type MemoryUserSessionManager struct {
	sessions    map[string]*UserSession
	msgChannels map[string]chan *chatv1.Message
	serverID    string
	mu          sync.RWMutex
}

// NewMemoryUserSessionManager creates a new in-memory session manager
func NewMemoryUserSessionManager(serverID string) *MemoryUserSessionManager {
	return &MemoryUserSessionManager{
		sessions:    make(map[string]*UserSession),
		msgChannels: make(map[string]chan *chatv1.Message),
		serverID:    serverID,
	}
}

// Register creates a new session for a user and returns a message channel
func (m *MemoryUserSessionManager) Register(ctx context.Context, user *chatv1.User) (chan *chatv1.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close existing channel if user reconnects
	if existingChan, exists := m.msgChannels[user.Id]; exists {
		close(existingChan)
	}

	// Create new session
	session := &UserSession{
		UserID:     user.Id,
		User:       user,
		ChannelIDs: []string{},
		ServerID:   m.serverID,
	}

	// Create message channel with buffer
	msgChan := make(chan *chatv1.Message, 100)

	m.sessions[user.Id] = session
	m.msgChannels[user.Id] = msgChan

	return msgChan, nil
}

// Unregister removes a user's session
func (m *MemoryUserSessionManager) Unregister(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if msgChan, exists := m.msgChannels[userID]; exists {
		close(msgChan)
		delete(m.msgChannels, userID)
	}

	delete(m.sessions, userID)
	return nil
}

// Get retrieves a user's session
func (m *MemoryUserSessionManager) Get(ctx context.Context, userID string) (*UserSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[userID]
	if !exists {
		return nil, ErrUserNotFound
	}
	return session, nil
}

// GetMessageChannel returns the message channel for a user
func (m *MemoryUserSessionManager) GetMessageChannel(ctx context.Context, userID string) (chan *chatv1.Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	msgChan, exists := m.msgChannels[userID]
	if !exists {
		return nil, ErrUserNotFound
	}
	return msgChan, nil
}

// AddChannel adds a channel subscription to a user's session
func (m *MemoryUserSessionManager) AddChannel(ctx context.Context, userID string, channelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[userID]
	if !exists {
		return ErrUserNotFound
	}

	// Check if already subscribed
	for _, ch := range session.ChannelIDs {
		if ch == channelID {
			return nil
		}
	}

	session.ChannelIDs = append(session.ChannelIDs, channelID)
	return nil
}

// RemoveChannel removes a channel subscription from a user's session
func (m *MemoryUserSessionManager) RemoveChannel(ctx context.Context, userID string, channelID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[userID]
	if !exists {
		return ErrUserNotFound
	}

	for i, ch := range session.ChannelIDs {
		if ch == channelID {
			session.ChannelIDs = append(session.ChannelIDs[:i], session.ChannelIDs[i+1:]...)
			return nil
		}
	}

	return nil // Not subscribed, not an error
}

// IsOnline checks if a user is currently connected
func (m *MemoryUserSessionManager) IsOnline(ctx context.Context, userID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.sessions[userID]
	return exists, nil
}

// GetAllSessions returns all active sessions (useful for internal operations)
func (m *MemoryUserSessionManager) GetAllSessions() []*UserSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]*UserSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// Ensure MemoryUserSessionManager implements UserSessionManager
var _ UserSessionManager = (*MemoryUserSessionManager)(nil)
