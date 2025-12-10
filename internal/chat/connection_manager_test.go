package chat

import (
	"context"
	"sync"
	"testing"
	"time"

	chatv1 "Hermes/gen/chat/v1"
)

func TestConnectionManager_Connect(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	cm := NewConnectionManager(nil, sessionManager)
	defer cm.Stop()

	user := &chatv1.User{Id: "user-1", Username: "TestUser"}

	conn, err := cm.Connect(ctx, user, nil)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if conn.UserID != user.Id {
		t.Errorf("Expected UserID %s, got %s", user.Id, conn.UserID)
	}

	if conn.State != ConnectionStateConnected {
		t.Errorf("Expected state Connected, got %s", conn.State.String())
	}

	if conn.SessionToken == "" {
		t.Error("Expected session token to be generated")
	}

	// Verify connection is tracked
	if !cm.IsConnected(user.Id) {
		t.Error("Expected user to be connected")
	}

	if cm.GetConnectionCount() != 1 {
		t.Errorf("Expected 1 connection, got %d", cm.GetConnectionCount())
	}
}

func TestConnectionManager_ConnectWithClientInfo(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	cm := NewConnectionManager(nil, sessionManager)
	defer cm.Stop()

	user := &chatv1.User{Id: "user-1", Username: "TestUser"}
	clientInfo := map[string]string{
		"version":     "1.0.0",
		"platform":    "Windows",
		"remote_addr": "192.168.1.1",
	}

	conn, err := cm.Connect(ctx, user, clientInfo)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	if conn.ClientVersion != "1.0.0" {
		t.Errorf("Expected version 1.0.0, got %s", conn.ClientVersion)
	}

	if conn.Platform != "Windows" {
		t.Errorf("Expected platform Windows, got %s", conn.Platform)
	}

	if conn.RemoteAddr != "192.168.1.1" {
		t.Errorf("Expected remote addr 192.168.1.1, got %s", conn.RemoteAddr)
	}
}

func TestConnectionManager_Disconnect(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	cm := NewConnectionManager(nil, sessionManager)
	defer cm.Stop()

	user := &chatv1.User{Id: "user-1", Username: "TestUser"}

	_, err := cm.Connect(ctx, user, nil)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	err = cm.Disconnect(ctx, user.Id)
	if err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	if cm.IsConnected(user.Id) {
		t.Error("Expected user to be disconnected")
	}

	if cm.GetConnectionCount() != 0 {
		t.Errorf("Expected 0 connections, got %d", cm.GetConnectionCount())
	}
}

func TestConnectionManager_DisconnectNotFound(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	cm := NewConnectionManager(nil, sessionManager)
	defer cm.Stop()

	err := cm.Disconnect(ctx, "nonexistent")
	if err != ErrUserNotFound {
		t.Errorf("Expected ErrUserNotFound, got %v", err)
	}
}

func TestConnectionManager_Heartbeat(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	cm := NewConnectionManager(nil, sessionManager)
	defer cm.Stop()

	user := &chatv1.User{Id: "user-1", Username: "TestUser"}

	conn, err := cm.Connect(ctx, user, nil)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	initialHeartbeat := conn.LastHeartbeat
	time.Sleep(10 * time.Millisecond)

	err = cm.Heartbeat(ctx, user.Id)
	if err != nil {
		t.Fatalf("Heartbeat failed: %v", err)
	}

	conn, _ = cm.GetConnection(user.Id)
	if !conn.LastHeartbeat.After(initialHeartbeat) {
		t.Error("Expected heartbeat time to be updated")
	}
}

func TestConnectionManager_RecordActivity(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	cm := NewConnectionManager(nil, sessionManager)
	defer cm.Stop()

	user := &chatv1.User{Id: "user-1", Username: "TestUser"}

	conn, err := cm.Connect(ctx, user, nil)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	initialActivity := conn.LastActivity
	time.Sleep(10 * time.Millisecond)

	err = cm.RecordActivity(ctx, user.Id)
	if err != nil {
		t.Fatalf("RecordActivity failed: %v", err)
	}

	conn, _ = cm.GetConnection(user.Id)
	if !conn.LastActivity.After(initialActivity) {
		t.Error("Expected activity time to be updated")
	}
}

func TestConnectionManager_Reconnect(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	config := &ConnectionConfig{
		HeartbeatInterval:    30 * time.Second,
		HeartbeatTimeout:     10 * time.Second,
		InactivityTimeout:    5 * time.Minute,
		SessionRecoveryTime:  1 * time.Minute,
		MaxReconnectAttempts: 5,
	}
	cm := NewConnectionManager(config, sessionManager)
	defer cm.Stop()

	user := &chatv1.User{Id: "user-1", Username: "TestUser"}

	// Connect
	conn, err := cm.Connect(ctx, user, nil)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	sessionToken := conn.SessionToken
	originalChannels := []string{"channel-1", "channel-2"}
	for _, ch := range originalChannels {
		_ = cm.AddChannel(ctx, user.Id, ch)
	}

	// Disconnect
	err = cm.Disconnect(ctx, user.Id)
	if err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	// Reconnect
	conn, err = cm.Reconnect(ctx, user, sessionToken)
	if err != nil {
		t.Fatalf("Reconnect failed: %v", err)
	}

	if conn.State != ConnectionStateConnected {
		t.Errorf("Expected state Connected, got %s", conn.State.String())
	}

	if conn.ReconnectCount != 1 {
		t.Errorf("Expected reconnect count 1, got %d", conn.ReconnectCount)
	}

	// Verify channels were preserved
	if len(conn.ChannelIDs) != len(originalChannels) {
		t.Errorf("Expected %d channels, got %d", len(originalChannels), len(conn.ChannelIDs))
	}

	// New session token should be issued
	if conn.SessionToken == sessionToken {
		t.Error("Expected new session token after reconnect")
	}
}

func TestConnectionManager_ReconnectExpired(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	config := &ConnectionConfig{
		HeartbeatInterval:    30 * time.Second,
		HeartbeatTimeout:     10 * time.Second,
		InactivityTimeout:    5 * time.Minute,
		SessionRecoveryTime:  10 * time.Millisecond, // Very short for testing
		MaxReconnectAttempts: 5,
	}
	cm := NewConnectionManager(config, sessionManager)
	defer cm.Stop()

	user := &chatv1.User{Id: "user-1", Username: "TestUser"}

	conn, err := cm.Connect(ctx, user, nil)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	sessionToken := conn.SessionToken

	err = cm.Disconnect(ctx, user.Id)
	if err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	// Wait for session to expire
	time.Sleep(20 * time.Millisecond)

	_, err = cm.Reconnect(ctx, user, sessionToken)
	if err != ErrSessionExpired {
		t.Errorf("Expected ErrSessionExpired, got %v", err)
	}
}

func TestConnectionManager_ReconnectInvalidToken(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	cm := NewConnectionManager(nil, sessionManager)
	defer cm.Stop()

	user := &chatv1.User{Id: "user-1", Username: "TestUser"}

	_, err := cm.Connect(ctx, user, nil)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	err = cm.Disconnect(ctx, user.Id)
	if err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	_, err = cm.Reconnect(ctx, user, "invalid-token")
	if err != ErrInvalidSessionToken {
		t.Errorf("Expected ErrInvalidSessionToken, got %v", err)
	}
}

func TestConnectionManager_StateChange(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	cm := NewConnectionManager(nil, sessionManager)
	defer cm.Stop()

	var stateChanges []ConnectionState
	var mu sync.Mutex

	cm.OnStateChange(func(userID string, oldState, newState ConnectionState) {
		mu.Lock()
		stateChanges = append(stateChanges, newState)
		mu.Unlock()
	})

	user := &chatv1.User{Id: "user-1", Username: "TestUser"}

	_, err := cm.Connect(ctx, user, nil)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	err = cm.Disconnect(ctx, user.Id)
	if err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	// Should see: Connecting -> Connected -> Disconnecting -> Disconnected
	expected := []ConnectionState{
		ConnectionStateConnecting,
		ConnectionStateConnected,
		ConnectionStateDisconnecting,
		ConnectionStateDisconnected,
	}

	if len(stateChanges) != len(expected) {
		t.Errorf("Expected %d state changes, got %d", len(expected), len(stateChanges))
		return
	}

	for i, s := range expected {
		if stateChanges[i] != s {
			t.Errorf("State change %d: expected %s, got %s", i, s.String(), stateChanges[i].String())
		}
	}
}

func TestConnectionManager_ForceDisconnectOnReconnect(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	cm := NewConnectionManager(nil, sessionManager)
	defer cm.Stop()

	user := &chatv1.User{Id: "user-1", Username: "TestUser"}

	// First connection
	conn1, err := cm.Connect(ctx, user, nil)
	if err != nil {
		t.Fatalf("First connect failed: %v", err)
	}
	token1 := conn1.SessionToken

	// Second connection (should disconnect first)
	conn2, err := cm.Connect(ctx, user, nil)
	if err != nil {
		t.Fatalf("Second connect failed: %v", err)
	}

	// Should have new session token
	if conn2.SessionToken == token1 {
		t.Error("Expected new session token for new connection")
	}

	// Should only have one active connection
	if cm.GetConnectionCount() != 1 {
		t.Errorf("Expected 1 connection, got %d", cm.GetConnectionCount())
	}
}

func TestConnectionManager_AddRemoveChannel(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	cm := NewConnectionManager(nil, sessionManager)
	defer cm.Stop()

	user := &chatv1.User{Id: "user-1", Username: "TestUser"}

	_, err := cm.Connect(ctx, user, nil)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Add channels
	err = cm.AddChannel(ctx, user.Id, "channel-1")
	if err != nil {
		t.Fatalf("AddChannel failed: %v", err)
	}

	err = cm.AddChannel(ctx, user.Id, "channel-2")
	if err != nil {
		t.Fatalf("AddChannel failed: %v", err)
	}

	conn, _ := cm.GetConnection(user.Id)
	if len(conn.ChannelIDs) != 2 {
		t.Errorf("Expected 2 channels, got %d", len(conn.ChannelIDs))
	}

	// Add duplicate (should be no-op)
	err = cm.AddChannel(ctx, user.Id, "channel-1")
	if err != nil {
		t.Fatalf("AddChannel (duplicate) failed: %v", err)
	}

	conn, _ = cm.GetConnection(user.Id)
	if len(conn.ChannelIDs) != 2 {
		t.Errorf("Expected 2 channels after duplicate, got %d", len(conn.ChannelIDs))
	}

	// Remove channel
	err = cm.RemoveChannel(ctx, user.Id, "channel-1")
	if err != nil {
		t.Fatalf("RemoveChannel failed: %v", err)
	}

	conn, _ = cm.GetConnection(user.Id)
	if len(conn.ChannelIDs) != 1 {
		t.Errorf("Expected 1 channel after remove, got %d", len(conn.ChannelIDs))
	}
}

func TestConnectionManager_GetMetrics(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	cm := NewConnectionManager(nil, sessionManager)
	defer cm.Stop()

	user1 := &chatv1.User{Id: "user-1", Username: "TestUser1"}
	user2 := &chatv1.User{Id: "user-2", Username: "TestUser2"}

	_, err := cm.Connect(ctx, user1, nil)
	if err != nil {
		t.Fatalf("Connect user1 failed: %v", err)
	}

	_, err = cm.Connect(ctx, user2, nil)
	if err != nil {
		t.Fatalf("Connect user2 failed: %v", err)
	}

	metrics := cm.GetMetrics()

	if metrics.ActiveConnections != 2 {
		t.Errorf("Expected 2 active connections, got %d", metrics.ActiveConnections)
	}

	// Disconnect one user
	_ = cm.Disconnect(ctx, user1.Id)

	metrics = cm.GetMetrics()

	if metrics.ActiveConnections != 1 {
		t.Errorf("Expected 1 active connection, got %d", metrics.ActiveConnections)
	}

	if metrics.DisconnectedSessions != 1 {
		t.Errorf("Expected 1 disconnected session, got %d", metrics.DisconnectedSessions)
	}
}

func TestConnectionManager_HeartbeatTimeout(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	config := &ConnectionConfig{
		HeartbeatInterval:    10 * time.Millisecond,
		HeartbeatTimeout:     5 * time.Millisecond,
		InactivityTimeout:    1 * time.Hour, // Don't trigger this
		SessionRecoveryTime:  1 * time.Minute,
		MaxReconnectAttempts: 5,
	}
	cm := NewConnectionManager(config, sessionManager)
	cm.Start()
	defer cm.Stop()

	user := &chatv1.User{Id: "user-1", Username: "TestUser"}

	_, err := cm.Connect(ctx, user, nil)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Wait for heartbeat timeout
	time.Sleep(50 * time.Millisecond)

	if cm.IsConnected(user.Id) {
		t.Error("Expected user to be disconnected due to heartbeat timeout")
	}
}

func TestConnectionManager_ConcurrentConnections(t *testing.T) {
	ctx := context.Background()
	sessionManager := NewMemoryUserSessionManager("test")
	cm := NewConnectionManager(nil, sessionManager)
	cm.Start()
	defer cm.Stop()

	const numUsers = 50
	var wg sync.WaitGroup

	// Connect many users concurrently
	for i := 0; i < numUsers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			user := &chatv1.User{
				Id:       string(rune('0' + id)),
				Username: "User" + string(rune('0'+id)),
			}
			_, _ = cm.Connect(ctx, user, nil)
			_ = cm.Heartbeat(ctx, user.Id)
			_ = cm.RecordActivity(ctx, user.Id)
		}(i)
	}

	wg.Wait()

	// All connections should be established
	if cm.GetConnectionCount() != numUsers {
		t.Errorf("Expected %d connections, got %d", numUsers, cm.GetConnectionCount())
	}
}

func TestConnectionState_String(t *testing.T) {
	tests := []struct {
		state    ConnectionState
		expected string
	}{
		{ConnectionStateUnknown, "unknown"},
		{ConnectionStateConnecting, "connecting"},
		{ConnectionStateConnected, "connected"},
		{ConnectionStateDisconnecting, "disconnecting"},
		{ConnectionStateDisconnected, "disconnected"},
		{ConnectionStateReconnecting, "reconnecting"},
	}

	for _, tt := range tests {
		if tt.state.String() != tt.expected {
			t.Errorf("ConnectionState(%d).String(): expected %s, got %s", tt.state, tt.expected, tt.state.String())
		}
	}
}
