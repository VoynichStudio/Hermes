package chat

import (
	"context"
	"sync"
	"testing"
	"time"

	chatv1 "Hermes/gen/chat/v1"
)

func TestNewServerWithDefaults(t *testing.T) {
	server := NewServerWithDefaults()

	if server == nil {
		t.Fatal("NewServerWithDefaults() returned nil")
	}

	// Should have default "general" channel
	ctx := context.Background()
	ch, err := server.ChannelRepo().Get(ctx, "general")
	if err != nil {
		t.Errorf("NewServerWithDefaults() should create default 'general' channel: %v", err)
	}
	if ch.Id != "general" {
		t.Errorf("general channel Id = %q, want %q", ch.Id, "general")
	}
	if ch.Label != "General" {
		t.Errorf("general channel Label = %q, want %q", ch.Label, "General")
	}
}

func TestNewServer_WithCustomDependencies(t *testing.T) {
	channelRepo := NewMemoryChannelRepository()
	messageRepo := NewMemoryMessageRepository()
	sessionManager := NewMemoryUserSessionManager("test-server")
	broadcaster := NewMemoryMessageBroadcaster(channelRepo, sessionManager, messageRepo)

	server := NewServer(ServerConfig{
		ChannelRepo:    channelRepo,
		MessageRepo:    messageRepo,
		SessionManager: sessionManager,
		Broadcaster:    broadcaster,
	})

	if server == nil {
		t.Fatal("NewServer() returned nil")
	}

	// Verify dependencies are wired correctly
	if server.ChannelRepo() != channelRepo {
		t.Error("ChannelRepo not correctly wired")
	}
	if server.MessageRepo() != messageRepo {
		t.Error("MessageRepo not correctly wired")
	}
	if server.SessionManager() != sessionManager {
		t.Error("SessionManager not correctly wired")
	}
	if server.Broadcaster() != broadcaster {
		t.Error("Broadcaster not correctly wired")
	}
}

func TestMemoryChannelRepository_Get(t *testing.T) {
	repo := NewMemoryChannelRepository()
	ctx := context.Background()

	// Create a channel first
	channel := &chatv1.Channel{Id: "test-channel", Label: "Test"}
	_ = repo.Create(ctx, channel)

	tests := []struct {
		name      string
		channelID string
		wantErr   error
	}{
		{
			name:      "existing channel",
			channelID: "test-channel",
			wantErr:   nil,
		},
		{
			name:      "non-existing channel",
			channelID: "random",
			wantErr:   ErrChannelNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.Get(ctx, tt.channelID)
			if err != tt.wantErr {
				t.Errorf("Get(%q) error = %v, want %v", tt.channelID, err, tt.wantErr)
			}
		})
	}
}

func TestMemoryChannelRepository_GetOrCreate(t *testing.T) {
	repo := NewMemoryChannelRepository()
	ctx := context.Background()

	// Test creating a new channel
	newChannel := &chatv1.Channel{
		Id:    "test-channel",
		Label: "Test Channel",
	}

	result, err := repo.GetOrCreate(ctx, newChannel)
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}
	if result.Id != "test-channel" {
		t.Errorf("GetOrCreate() Id = %q, want %q", result.Id, "test-channel")
	}

	// Test getting existing channel
	existingChannel := &chatv1.Channel{
		Id:    "test-channel",
		Label: "Different Label", // Should not update
	}

	result, err = repo.GetOrCreate(ctx, existingChannel)
	if err != nil {
		t.Fatalf("GetOrCreate() error = %v", err)
	}
	if result.Label != "Test Channel" {
		t.Errorf("GetOrCreate() should return existing channel, Label = %q, want %q", result.Label, "Test Channel")
	}
}

func TestMemoryChannelRepository_AddUser(t *testing.T) {
	repo := NewMemoryChannelRepository()
	ctx := context.Background()

	// Create channel first
	channel := &chatv1.Channel{Id: "general", Label: "General", Users: []*chatv1.User{}}
	_, _ = repo.GetOrCreate(ctx, channel)

	user := &chatv1.User{
		Id:       "user-123",
		Username: "testuser",
	}

	err := repo.AddUser(ctx, "general", user)
	if err != nil {
		t.Fatalf("AddUser() error = %v", err)
	}

	users, _ := repo.GetUsers(ctx, "general")
	if len(users) != 1 {
		t.Errorf("AddUser() user count = %d, want 1", len(users))
	}
	if users[0].Id != "user-123" {
		t.Errorf("AddUser() user Id = %q, want %q", users[0].Id, "user-123")
	}
}

func TestMemoryChannelRepository_AddUser_NonExistentChannel(t *testing.T) {
	repo := NewMemoryChannelRepository()
	ctx := context.Background()

	user := &chatv1.User{
		Id:       "user-123",
		Username: "testuser",
	}

	err := repo.AddUser(ctx, "non-existent", user)
	if err != ErrChannelNotFound {
		t.Errorf("AddUser() error = %v, want %v", err, ErrChannelNotFound)
	}
}

func TestMemoryUserSessionManager_Register(t *testing.T) {
	manager := NewMemoryUserSessionManager("test-server")
	ctx := context.Background()

	user := &chatv1.User{Id: "user-123", Username: "testuser"}
	stream, err := manager.Register(ctx, user)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if stream == nil {
		t.Fatal("Register() returned nil stream")
	}

	// Stream should be buffered
	select {
	case stream <- &chatv1.Message{Content: "test"}:
		// Success - channel is buffered
	default:
		t.Error("Register() stream should be buffered")
	}
}

func TestMemoryUserSessionManager_Unregister(t *testing.T) {
	manager := NewMemoryUserSessionManager("test-server")
	ctx := context.Background()

	user := &chatv1.User{Id: "user-123", Username: "testuser"}
	_, _ = manager.Register(ctx, user)
	_ = manager.Unregister(ctx, "user-123")

	// Re-registering should work
	stream, err := manager.Register(ctx, user)
	if err != nil {
		t.Fatalf("Register() after Unregister() error = %v", err)
	}
	if stream == nil {
		t.Error("Register() after Unregister() returned nil stream")
	}
}

func TestMemoryMessageBroadcaster_Broadcast(t *testing.T) {
	channelRepo := NewMemoryChannelRepository()
	sessionManager := NewMemoryUserSessionManager("test-server")
	messageRepo := NewMemoryMessageRepository()
	broadcaster := NewMemoryMessageBroadcaster(channelRepo, sessionManager, messageRepo)
	ctx := context.Background()

	// Create channel
	channel := &chatv1.Channel{Id: "general", Label: "General", Users: []*chatv1.User{}}
	_, _ = channelRepo.GetOrCreate(ctx, channel)

	// Add users to channel
	user1 := &chatv1.User{Id: "user-1", Username: "user1"}
	user2 := &chatv1.User{Id: "user-2", Username: "user2"}
	_ = channelRepo.AddUser(ctx, "general", user1)
	_ = channelRepo.AddUser(ctx, "general", user2)

	// Register streams
	stream1, _ := sessionManager.Register(ctx, user1)
	stream2, _ := sessionManager.Register(ctx, user2)

	// Broadcast message
	msg := &chatv1.Message{
		Id:        "msg-1",
		ChannelId: "general",
		Content:   "Hello everyone!",
	}

	err := broadcaster.Broadcast(ctx, "general", msg)
	if err != nil {
		t.Fatalf("Broadcast() error = %v", err)
	}

	// Check both streams received the message
	select {
	case received := <-stream1:
		if received.Content != "Hello everyone!" {
			t.Errorf("stream1 received Content = %q, want %q", received.Content, "Hello everyone!")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("stream1 did not receive message")
	}

	select {
	case received := <-stream2:
		if received.Content != "Hello everyone!" {
			t.Errorf("stream2 received Content = %q, want %q", received.Content, "Hello everyone!")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("stream2 did not receive message")
	}
}

func TestMemoryMessageBroadcaster_Broadcast_ChannelNotFound(t *testing.T) {
	channelRepo := NewMemoryChannelRepository()
	sessionManager := NewMemoryUserSessionManager("test-server")
	messageRepo := NewMemoryMessageRepository()
	broadcaster := NewMemoryMessageBroadcaster(channelRepo, sessionManager, messageRepo)
	ctx := context.Background()

	msg := &chatv1.Message{Content: "test"}
	err := broadcaster.Broadcast(ctx, "non-existent", msg)

	if err != ErrChannelNotFound {
		t.Errorf("Broadcast() error = %v, want %v", err, ErrChannelNotFound)
	}
}

func TestMemoryMessageBroadcaster_Broadcast_FullBuffer(t *testing.T) {
	channelRepo := NewMemoryChannelRepository()
	sessionManager := NewMemoryUserSessionManager("test-server")
	messageRepo := NewMemoryMessageRepository()
	broadcaster := NewMemoryMessageBroadcaster(channelRepo, sessionManager, messageRepo)
	ctx := context.Background()

	// Create channel and user
	channel := &chatv1.Channel{Id: "general", Label: "General", Users: []*chatv1.User{}}
	_, _ = channelRepo.GetOrCreate(ctx, channel)

	user := &chatv1.User{Id: "user-1", Username: "user1"}
	_ = channelRepo.AddUser(ctx, "general", user)
	stream, _ := sessionManager.Register(ctx, user)

	// Fill the buffer (capacity is 100)
	for i := 0; i < 100; i++ {
		stream <- &chatv1.Message{Content: "filler"}
	}

	// This broadcast should not block even with full buffer
	msg := &chatv1.Message{Content: "overflow"}
	done := make(chan struct{})
	go func() {
		broadcaster.Broadcast(ctx, "general", msg)
		close(done)
	}()

	select {
	case <-done:
		// Success - did not block
	case <-time.After(100 * time.Millisecond):
		t.Error("Broadcast() blocked on full buffer")
	}
}

func TestMemoryMessageRepository_SaveAndGet(t *testing.T) {
	repo := NewMemoryMessageRepository()
	ctx := context.Background()

	msg := &chatv1.Message{
		Id:        "msg-1",
		ChannelId: "general",
		Content:   "Hello",
		Timestamp: 12345,
	}

	err := repo.Save(ctx, msg)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	retrieved, err := repo.GetByID(ctx, "msg-1")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if retrieved.Content != "Hello" {
		t.Errorf("GetByID() Content = %q, want %q", retrieved.Content, "Hello")
	}
}

func TestMemoryMessageRepository_GetByChannel(t *testing.T) {
	repo := NewMemoryMessageRepository()
	ctx := context.Background()

	// Save multiple messages
	for i := 0; i < 5; i++ {
		msg := &chatv1.Message{
			Id:        "msg-" + string(rune('0'+i)),
			ChannelId: "general",
			Content:   "Message " + string(rune('0'+i)),
		}
		_ = repo.Save(ctx, msg)
	}

	messages, cursor, err := repo.GetByChannel(ctx, "general", 3, "")
	if err != nil {
		t.Fatalf("GetByChannel() error = %v", err)
	}
	if len(messages) != 3 {
		t.Errorf("GetByChannel() returned %d messages, want 3", len(messages))
	}
	if cursor == "" {
		t.Error("GetByChannel() should return cursor when more messages available")
	}
}

func TestConcurrentAccess(t *testing.T) {
	channelRepo := NewMemoryChannelRepository()
	sessionManager := NewMemoryUserSessionManager("test-server")
	messageRepo := NewMemoryMessageRepository()
	broadcaster := NewMemoryMessageBroadcaster(channelRepo, sessionManager, messageRepo)
	ctx := context.Background()

	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent channel operations
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// Create channel
			ch := &chatv1.Channel{
				Id:    "channel-" + string(rune('0'+id%10)),
				Label: "Test",
				Users: []*chatv1.User{},
			}
			_, _ = channelRepo.GetOrCreate(ctx, ch)

			// Add user
			user := &chatv1.User{Id: "user-" + string(rune('0'+id))}
			_ = channelRepo.AddUser(ctx, ch.Id, user)

			// Register session
			stream, _ := sessionManager.Register(ctx, user)

			// Get channel
			_, _ = channelRepo.Get(ctx, ch.Id)

			// Broadcast
			_ = broadcaster.Broadcast(ctx, ch.Id, &chatv1.Message{Content: "test"})

			// Unregister session
			_ = sessionManager.Unregister(ctx, user.Id)

			// Drain stream to avoid leaks
			go func() {
				for range stream {
				}
			}()
		}(i)
	}

	wg.Wait()
}
