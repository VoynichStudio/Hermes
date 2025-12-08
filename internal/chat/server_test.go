package chat

import (
	"sync"
	"testing"
	"time"

	chatv1 "Hermes/gen/chat/v1"
)

func TestNewServer(t *testing.T) {
	server := NewServer()

	if server == nil {
		t.Fatal("NewServer() returned nil")
	}

	// Should have default "general" channel
	ch, exists := server.GetChannel("general")
	if !exists {
		t.Error("NewServer() should create default 'general' channel")
	}
	if ch.Id != "general" {
		t.Errorf("general channel Id = %q, want %q", ch.Id, "general")
	}
	if ch.Label != "General" {
		t.Errorf("general channel Label = %q, want %q", ch.Label, "General")
	}
}

func TestServer_GetChannel(t *testing.T) {
	server := NewServer()

	tests := []struct {
		name       string
		channelID  string
		wantExists bool
	}{
		{
			name:       "existing channel",
			channelID:  "general",
			wantExists: true,
		},
		{
			name:       "non-existing channel",
			channelID:  "random",
			wantExists: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, exists := server.GetChannel(tt.channelID)
			if exists != tt.wantExists {
				t.Errorf("GetChannel(%q) exists = %v, want %v", tt.channelID, exists, tt.wantExists)
			}
		})
	}
}

func TestServer_GetOrCreateChannel(t *testing.T) {
	server := NewServer()

	// Test creating a new channel
	newChannel := &chatv1.Channel{
		Id:    "test-channel",
		Label: "Test Channel",
	}

	result := server.GetOrCreateChannel(newChannel)
	if result.Id != "test-channel" {
		t.Errorf("GetOrCreateChannel() Id = %q, want %q", result.Id, "test-channel")
	}

	// Test getting existing channel
	existingChannel := &chatv1.Channel{
		Id:    "test-channel",
		Label: "Different Label", // Should not update
	}

	result = server.GetOrCreateChannel(existingChannel)
	if result.Label != "Test Channel" {
		t.Errorf("GetOrCreateChannel() should return existing channel, Label = %q, want %q", result.Label, "Test Channel")
	}
}

func TestServer_AddUserToChannel(t *testing.T) {
	server := NewServer()

	user := &chatv1.User{
		Id:       "user-123",
		Username: "testuser",
	}

	server.AddUserToChannel("general", user)

	ch, _ := server.GetChannel("general")
	if len(ch.Users) != 1 {
		t.Errorf("AddUserToChannel() user count = %d, want 1", len(ch.Users))
	}
	if ch.Users[0].Id != "user-123" {
		t.Errorf("AddUserToChannel() user Id = %q, want %q", ch.Users[0].Id, "user-123")
	}
}

func TestServer_AddUserToChannel_NonExistentChannel(t *testing.T) {
	server := NewServer()

	user := &chatv1.User{
		Id:       "user-123",
		Username: "testuser",
	}

	// Should not panic when channel doesn't exist
	server.AddUserToChannel("non-existent", user)
}

func TestServer_RegisterUserStream(t *testing.T) {
	server := NewServer()

	stream := server.RegisterUserStream("user-123")
	if stream == nil {
		t.Fatal("RegisterUserStream() returned nil")
	}

	// Stream should be buffered
	select {
	case stream <- &chatv1.Message{Content: "test"}:
		// Success - channel is buffered
	default:
		t.Error("RegisterUserStream() stream should be buffered")
	}
}

func TestServer_UnregisterUserStream(t *testing.T) {
	server := NewServer()

	server.RegisterUserStream("user-123")
	server.UnregisterUserStream("user-123")

	// Re-registering should work
	stream := server.RegisterUserStream("user-123")
	if stream == nil {
		t.Error("RegisterUserStream() after UnregisterUserStream() returned nil")
	}
}

func TestServer_BroadcastToChannel(t *testing.T) {
	server := NewServer()

	// Add users to channel
	user1 := &chatv1.User{Id: "user-1", Username: "user1"}
	user2 := &chatv1.User{Id: "user-2", Username: "user2"}

	server.AddUserToChannel("general", user1)
	server.AddUserToChannel("general", user2)

	// Register streams
	stream1 := server.RegisterUserStream("user-1")
	stream2 := server.RegisterUserStream("user-2")

	// Broadcast message
	msg := &chatv1.Message{
		Id:        "msg-1",
		ChannelId: "general",
		Content:   "Hello everyone!",
	}

	err := server.BroadcastToChannel("general", msg)
	if err != nil {
		t.Fatalf("BroadcastToChannel() error = %v", err)
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

func TestServer_BroadcastToChannel_ChannelNotFound(t *testing.T) {
	server := NewServer()

	msg := &chatv1.Message{Content: "test"}
	err := server.BroadcastToChannel("non-existent", msg)

	if err != ErrChannelNotFound {
		t.Errorf("BroadcastToChannel() error = %v, want %v", err, ErrChannelNotFound)
	}
}

func TestServer_BroadcastToChannel_FullBuffer(t *testing.T) {
	server := NewServer()

	user := &chatv1.User{Id: "user-1", Username: "user1"}
	server.AddUserToChannel("general", user)
	stream := server.RegisterUserStream("user-1")

	// Fill the buffer (capacity is 10)
	for i := 0; i < 10; i++ {
		stream <- &chatv1.Message{Content: "filler"}
	}

	// This broadcast should not block even with full buffer
	msg := &chatv1.Message{Content: "overflow"}
	done := make(chan struct{})
	go func() {
		server.BroadcastToChannel("general", msg)
		close(done)
	}()

	select {
	case <-done:
		// Success - did not block
	case <-time.After(100 * time.Millisecond):
		t.Error("BroadcastToChannel() blocked on full buffer")
	}
}

func TestServer_ConcurrentAccess(t *testing.T) {
	server := NewServer()

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
			}
			server.GetOrCreateChannel(ch)

			// Add user
			user := &chatv1.User{Id: "user-" + string(rune('0'+id))}
			server.AddUserToChannel(ch.Id, user)

			// Register stream
			stream := server.RegisterUserStream(user.Id)

			// Get channel
			server.GetChannel(ch.Id)

			// Broadcast
			server.BroadcastToChannel(ch.Id, &chatv1.Message{Content: "test"})

			// Unregister stream
			server.UnregisterUserStream(user.Id)

			// Drain stream
			close(stream)
		}(i)
	}

	wg.Wait()
}
