package chat

import (
	"context"
	"errors"
	"sync"
	"time"

	chatv1 "Hermes/gen/chat/v1"
)

var (
	// ErrConnectionClosed is returned when the connection is closed
	ErrConnectionClosed = errors.New("connection closed")

	// ErrSessionExpired is returned when trying to reconnect to an expired session
	ErrSessionExpired = errors.New("session expired")

	// ErrInvalidSessionToken is returned when the session token is invalid
	ErrInvalidSessionToken = errors.New("invalid session token")
)

// ConnectionState represents the current state of a connection
type ConnectionState int32

const (
	// ConnectionStateUnknown is the default state
	ConnectionStateUnknown ConnectionState = iota
	// ConnectionStateConnecting indicates the connection is being established
	ConnectionStateConnecting
	// ConnectionStateConnected indicates the connection is active
	ConnectionStateConnected
	// ConnectionStateDisconnecting indicates the connection is being closed
	ConnectionStateDisconnecting
	// ConnectionStateDisconnected indicates the connection is closed
	ConnectionStateDisconnected
	// ConnectionStateReconnecting indicates a reconnection attempt is in progress
	ConnectionStateReconnecting
)

// String returns the string representation of ConnectionState
func (s ConnectionState) String() string {
	switch s {
	case ConnectionStateConnecting:
		return "connecting"
	case ConnectionStateConnected:
		return "connected"
	case ConnectionStateDisconnecting:
		return "disconnecting"
	case ConnectionStateDisconnected:
		return "disconnected"
	case ConnectionStateReconnecting:
		return "reconnecting"
	default:
		return "unknown"
	}
}

// ConnectionInfo holds information about a user's connection
type ConnectionInfo struct {
	UserID          string
	User            *chatv1.User
	State           ConnectionState
	ConnectedAt     time.Time
	LastHeartbeat   time.Time
	LastActivity    time.Time
	SessionToken    string // For reconnection
	ChannelIDs      []string
	DisconnectedAt  time.Time // Set when disconnected, for session recovery window
	ReconnectCount  int
	ClientVersion   string
	Platform        string
	RemoteAddr      string
}

// ConnectionConfig holds configuration for the connection manager
type ConnectionConfig struct {
	HeartbeatInterval   time.Duration // How often to send heartbeats
	HeartbeatTimeout    time.Duration // How long to wait for heartbeat response
	InactivityTimeout   time.Duration // Disconnect after this period of inactivity
	SessionRecoveryTime time.Duration // How long to keep session for reconnection
	MaxReconnectAttempts int          // Maximum number of reconnection attempts
}

// DefaultConnectionConfig returns sensible defaults for connection management
func DefaultConnectionConfig() *ConnectionConfig {
	return &ConnectionConfig{
		HeartbeatInterval:    30 * time.Second,
		HeartbeatTimeout:     10 * time.Second,
		InactivityTimeout:    5 * time.Minute,
		SessionRecoveryTime:  2 * time.Minute,
		MaxReconnectAttempts: 5,
	}
}

// ConnectionStateHandler is called when connection state changes
type ConnectionStateHandler func(userID string, oldState, newState ConnectionState)

// ConnectionManager manages connection lifecycle and state
type ConnectionManager struct {
	config          *ConnectionConfig
	connections     map[string]*ConnectionInfo
	disconnected    map[string]*ConnectionInfo // For session recovery
	stateHandlers   []ConnectionStateHandler
	sessionManager  UserSessionManager
	mu              sync.RWMutex
	stopCh          chan struct{}
	wg              sync.WaitGroup
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(config *ConnectionConfig, sessionManager UserSessionManager) *ConnectionManager {
	if config == nil {
		config = DefaultConnectionConfig()
	}
	cm := &ConnectionManager{
		config:         config,
		connections:    make(map[string]*ConnectionInfo),
		disconnected:   make(map[string]*ConnectionInfo),
		stateHandlers:  make([]ConnectionStateHandler, 0),
		sessionManager: sessionManager,
		stopCh:         make(chan struct{}),
	}
	return cm
}

// Start begins the connection manager background tasks
func (cm *ConnectionManager) Start() {
	cm.wg.Add(2)
	go cm.heartbeatLoop()
	go cm.cleanupLoop()
}

// Stop gracefully stops the connection manager
func (cm *ConnectionManager) Stop() {
	close(cm.stopCh)
	cm.wg.Wait()
}

// OnStateChange registers a handler for connection state changes
func (cm *ConnectionManager) OnStateChange(handler ConnectionStateHandler) {
	cm.mu.Lock()
	defer cm.mu.Unlock()
	cm.stateHandlers = append(cm.stateHandlers, handler)
}

// Connect registers a new connection
func (cm *ConnectionManager) Connect(ctx context.Context, user *chatv1.User, clientInfo map[string]string) (*ConnectionInfo, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	sessionToken := generateSessionToken()

	// Check if this is a reconnection
	if existing, ok := cm.disconnected[user.Id]; ok {
		// Attempt to recover session
		if now.Sub(existing.DisconnectedAt) < cm.config.SessionRecoveryTime {
			existing.State = ConnectionStateReconnecting
			existing.ReconnectCount++
			existing.LastHeartbeat = now
			existing.LastActivity = now
			existing.SessionToken = sessionToken

			// Move back to active connections
			cm.connections[user.Id] = existing
			delete(cm.disconnected, user.Id)

			cm.notifyStateChange(user.Id, ConnectionStateDisconnected, ConnectionStateConnected)
			existing.State = ConnectionStateConnected
			return existing, nil
		}
		// Session expired, remove it
		delete(cm.disconnected, user.Id)
	}

	// Check for existing active connection
	if existing, ok := cm.connections[user.Id]; ok {
		// Force disconnect old connection
		cm.disconnectLocked(user.Id, existing)
	}

	conn := &ConnectionInfo{
		UserID:        user.Id,
		User:          user,
		State:         ConnectionStateConnecting,
		ConnectedAt:   now,
		LastHeartbeat: now,
		LastActivity:  now,
		SessionToken:  sessionToken,
		ChannelIDs:    make([]string, 0),
	}

	// Set client info if provided
	if clientInfo != nil {
		conn.ClientVersion = clientInfo["version"]
		conn.Platform = clientInfo["platform"]
		conn.RemoteAddr = clientInfo["remote_addr"]
	}

	cm.connections[user.Id] = conn
	cm.notifyStateChange(user.Id, ConnectionStateUnknown, ConnectionStateConnecting)

	conn.State = ConnectionStateConnected
	cm.notifyStateChange(user.Id, ConnectionStateConnecting, ConnectionStateConnected)

	return conn, nil
}

// Disconnect handles a user disconnection
func (cm *ConnectionManager) Disconnect(ctx context.Context, userID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, ok := cm.connections[userID]
	if !ok {
		return ErrUserNotFound
	}

	return cm.disconnectLocked(userID, conn)
}

// disconnectLocked performs disconnect while holding the lock
func (cm *ConnectionManager) disconnectLocked(userID string, conn *ConnectionInfo) error {
	oldState := conn.State
	conn.State = ConnectionStateDisconnecting
	cm.notifyStateChange(userID, oldState, ConnectionStateDisconnecting)

	// Move to disconnected for potential reconnection
	conn.DisconnectedAt = time.Now()
	conn.State = ConnectionStateDisconnected
	cm.disconnected[userID] = conn
	delete(cm.connections, userID)

	cm.notifyStateChange(userID, ConnectionStateDisconnecting, ConnectionStateDisconnected)

	return nil
}

// Heartbeat updates the last heartbeat time for a connection
func (cm *ConnectionManager) Heartbeat(ctx context.Context, userID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, ok := cm.connections[userID]
	if !ok {
		return ErrUserNotFound
	}

	now := time.Now()
	conn.LastHeartbeat = now
	conn.LastActivity = now

	return nil
}

// RecordActivity records user activity (message sent, etc.)
func (cm *ConnectionManager) RecordActivity(ctx context.Context, userID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, ok := cm.connections[userID]
	if !ok {
		return ErrUserNotFound
	}

	conn.LastActivity = time.Now()
	return nil
}

// Reconnect attempts to reconnect with a session token
func (cm *ConnectionManager) Reconnect(ctx context.Context, user *chatv1.User, sessionToken string) (*ConnectionInfo, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, ok := cm.disconnected[user.Id]
	if !ok {
		return nil, ErrSessionExpired
	}

	// Validate session token
	if conn.SessionToken != sessionToken {
		return nil, ErrInvalidSessionToken
	}

	// Check if session is still valid
	if time.Since(conn.DisconnectedAt) > cm.config.SessionRecoveryTime {
		delete(cm.disconnected, user.Id)
		return nil, ErrSessionExpired
	}

	// Check reconnect limit
	if conn.ReconnectCount >= cm.config.MaxReconnectAttempts {
		delete(cm.disconnected, user.Id)
		return nil, ErrSessionExpired
	}

	// Restore connection
	conn.State = ConnectionStateReconnecting
	conn.ReconnectCount++
	conn.LastHeartbeat = time.Now()
	conn.LastActivity = time.Now()
	conn.SessionToken = generateSessionToken() // New token for security

	cm.connections[user.Id] = conn
	delete(cm.disconnected, user.Id)

	cm.notifyStateChange(user.Id, ConnectionStateDisconnected, ConnectionStateConnected)
	conn.State = ConnectionStateConnected

	return conn, nil
}

// GetConnection returns connection info for a user
func (cm *ConnectionManager) GetConnection(userID string) (*ConnectionInfo, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	conn, ok := cm.connections[userID]
	if !ok {
		return nil, ErrUserNotFound
	}
	return conn, nil
}

// GetAllConnections returns all active connections
func (cm *ConnectionManager) GetAllConnections() []*ConnectionInfo {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	conns := make([]*ConnectionInfo, 0, len(cm.connections))
	for _, conn := range cm.connections {
		conns = append(conns, conn)
	}
	return conns
}

// IsConnected checks if a user is currently connected
func (cm *ConnectionManager) IsConnected(userID string) bool {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	conn, ok := cm.connections[userID]
	return ok && conn.State == ConnectionStateConnected
}

// GetConnectionCount returns the number of active connections
func (cm *ConnectionManager) GetConnectionCount() int {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	return len(cm.connections)
}

// AddChannel adds a channel to a user's connection
func (cm *ConnectionManager) AddChannel(ctx context.Context, userID string, channelID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, ok := cm.connections[userID]
	if !ok {
		return ErrUserNotFound
	}

	// Check if already subscribed
	for _, ch := range conn.ChannelIDs {
		if ch == channelID {
			return nil
		}
	}

	conn.ChannelIDs = append(conn.ChannelIDs, channelID)
	return nil
}

// RemoveChannel removes a channel from a user's connection
func (cm *ConnectionManager) RemoveChannel(ctx context.Context, userID string, channelID string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	conn, ok := cm.connections[userID]
	if !ok {
		return ErrUserNotFound
	}

	for i, ch := range conn.ChannelIDs {
		if ch == channelID {
			conn.ChannelIDs = append(conn.ChannelIDs[:i], conn.ChannelIDs[i+1:]...)
			return nil
		}
	}

	return nil
}

// notifyStateChange notifies all handlers of a state change
func (cm *ConnectionManager) notifyStateChange(userID string, oldState, newState ConnectionState) {
	for _, handler := range cm.stateHandlers {
		handler(userID, oldState, newState)
	}
}

// heartbeatLoop checks for heartbeat timeouts
func (cm *ConnectionManager) heartbeatLoop() {
	defer cm.wg.Done()

	ticker := time.NewTicker(cm.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopCh:
			return
		case <-ticker.C:
			cm.checkHeartbeats()
		}
	}
}

// checkHeartbeats disconnects users that haven't sent a heartbeat
func (cm *ConnectionManager) checkHeartbeats() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	var toDisconnect []string

	for userID, conn := range cm.connections {
		// Check heartbeat timeout
		if now.Sub(conn.LastHeartbeat) > cm.config.HeartbeatTimeout {
			toDisconnect = append(toDisconnect, userID)
			continue
		}

		// Check inactivity timeout
		if now.Sub(conn.LastActivity) > cm.config.InactivityTimeout {
			toDisconnect = append(toDisconnect, userID)
		}
	}

	// Disconnect timed out connections
	for _, userID := range toDisconnect {
		if conn, ok := cm.connections[userID]; ok {
			_ = cm.disconnectLocked(userID, conn)
		}
	}
}

// cleanupLoop removes expired disconnected sessions
func (cm *ConnectionManager) cleanupLoop() {
	defer cm.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopCh:
			return
		case <-ticker.C:
			cm.cleanupExpiredSessions()
		}
	}
}

// cleanupExpiredSessions removes sessions that can no longer be recovered
func (cm *ConnectionManager) cleanupExpiredSessions() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	now := time.Now()
	for userID, conn := range cm.disconnected {
		if now.Sub(conn.DisconnectedAt) > cm.config.SessionRecoveryTime {
			delete(cm.disconnected, userID)
		}
	}
}

// generateSessionToken creates a unique session token
func generateSessionToken() string {
	return generateUUIDv7() // Reuse the UUID v7 generator from message_processor.go
}

// ConnectionMetrics holds metrics about connections
type ConnectionMetrics struct {
	ActiveConnections     int
	DisconnectedSessions  int
	TotalReconnects       int
	AverageSessionTime    time.Duration
}

// GetMetrics returns current connection metrics
func (cm *ConnectionManager) GetMetrics() ConnectionMetrics {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	metrics := ConnectionMetrics{
		ActiveConnections:    len(cm.connections),
		DisconnectedSessions: len(cm.disconnected),
	}

	var totalReconnects int
	var totalSessionTime time.Duration
	now := time.Now()

	for _, conn := range cm.connections {
		totalReconnects += conn.ReconnectCount
		totalSessionTime += now.Sub(conn.ConnectedAt)
	}

	metrics.TotalReconnects = totalReconnects
	if len(cm.connections) > 0 {
		metrics.AverageSessionTime = totalSessionTime / time.Duration(len(cm.connections))
	}

	return metrics
}
