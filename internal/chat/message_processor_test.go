package chat

import (
	"context"
	"strings"
	"testing"

	chatv1 "Hermes/gen/chat/v1"
)

func TestMessageProcessor_Process(t *testing.T) {
	ctx := context.Background()
	processor := NewMessageProcessor(nil)

	user := &chatv1.User{
		Id:       "user-1",
		Username: "TestUser",
	}

	msg := &chatv1.Message{
		ChannelId: "channel-1",
		UserId:    "user-1",
		Content:   "Hello, world!",
	}

	err := processor.Process(ctx, msg, user)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Check that ID was generated
	if msg.Id == "" {
		t.Error("Expected ID to be generated")
	}

	// Check that timestamps were set
	if msg.Timestamp == 0 {
		t.Error("Expected Timestamp to be set")
	}
	if msg.ServerTime == 0 {
		t.Error("Expected ServerTime to be set")
	}

	// Check that username was set
	if msg.Username != "TestUser" {
		t.Errorf("Expected Username to be 'TestUser', got '%s'", msg.Username)
	}

	// Check that default type was set
	if msg.Type != chatv1.MessageType_MESSAGE_TYPE_CHAT {
		t.Errorf("Expected Type to be CHAT, got %v", msg.Type)
	}
}

func TestMessageProcessor_Validate_EmptyContent(t *testing.T) {
	processor := NewMessageProcessor(nil)

	msg := &chatv1.Message{
		ChannelId: "channel-1",
		UserId:    "user-1",
		Content:   "",
	}

	err := processor.Validate(msg)
	if err != ErrMessageEmpty {
		t.Errorf("Expected ErrMessageEmpty, got %v", err)
	}
}

func TestMessageProcessor_Validate_TooLong(t *testing.T) {
	config := &MessageConfig{
		MaxLength: 100,
		MinLength: 1,
	}
	processor := NewMessageProcessor(config)

	msg := &chatv1.Message{
		ChannelId: "channel-1",
		UserId:    "user-1",
		Content:   strings.Repeat("a", 101),
	}

	err := processor.Validate(msg)
	if err != ErrMessageTooLong {
		t.Errorf("Expected ErrMessageTooLong, got %v", err)
	}
}

func TestMessageProcessor_Validate_InvalidUTF8(t *testing.T) {
	processor := NewMessageProcessor(nil)

	msg := &chatv1.Message{
		ChannelId: "channel-1",
		UserId:    "user-1",
		Content:   string([]byte{0xff, 0xfe, 0xfd}),
	}

	err := processor.Validate(msg)
	if err != ErrMessageInvalidUTF8 {
		t.Errorf("Expected ErrMessageInvalidUTF8, got %v", err)
	}
}

func TestMessageProcessor_Validate_MissingChannelID(t *testing.T) {
	processor := NewMessageProcessor(nil)

	msg := &chatv1.Message{
		UserId:  "user-1",
		Content: "Hello",
	}

	err := processor.Validate(msg)
	if err == nil {
		t.Error("Expected error for missing ChannelId")
	}
}

func TestMessageProcessor_Validate_MissingUserID(t *testing.T) {
	processor := NewMessageProcessor(nil)

	msg := &chatv1.Message{
		ChannelId: "channel-1",
		Content:   "Hello",
	}

	err := processor.Validate(msg)
	if err == nil {
		t.Error("Expected error for missing UserId")
	}
}

func TestMessageProcessor_WhitespaceStripping(t *testing.T) {
	config := &MessageConfig{
		MaxLength:       2000,
		MinLength:       1,
		StripWhitespace: true,
	}
	processor := NewMessageProcessor(config)

	msg := &chatv1.Message{
		ChannelId: "channel-1",
		UserId:    "user-1",
		Content:   "  Hello, world!  ",
	}

	err := processor.Validate(msg)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}

	if msg.Content != "Hello, world!" {
		t.Errorf("Expected content to be trimmed, got '%s'", msg.Content)
	}
}

func TestMessageProcessor_WithFilter(t *testing.T) {
	ctx := context.Background()
	processor := NewMessageProcessor(nil)

	// Add a profanity filter
	filter := NewProfanityFilter(FilterModeMask)
	filter.AddWords("badword")
	processor.AddFilter(filter)

	user := &chatv1.User{
		Id:       "user-1",
		Username: "TestUser",
	}

	msg := &chatv1.Message{
		ChannelId: "channel-1",
		UserId:    "user-1",
		Content:   "This is a badword test",
	}

	err := processor.Process(ctx, msg, user)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	if strings.Contains(msg.Content, "badword") {
		t.Error("Expected 'badword' to be masked")
	}
	if !strings.Contains(msg.Content, "*******") {
		t.Error("Expected masked content")
	}
}

func TestProfanityFilter_Block(t *testing.T) {
	ctx := context.Background()
	filter := NewProfanityFilter(FilterModeBlock)
	filter.AddWords("blocked")

	_, blocked, _ := filter.Filter(ctx, "This is blocked content", "user-1")
	if !blocked {
		t.Error("Expected message to be blocked")
	}
}

func TestProfanityFilter_Mask(t *testing.T) {
	ctx := context.Background()
	filter := NewProfanityFilter(FilterModeMask)
	filter.AddWords("test")

	filtered, blocked, _ := filter.Filter(ctx, "This is a test message", "user-1")
	if blocked {
		t.Error("Expected message not to be blocked")
	}
	if strings.Contains(filtered, "test") {
		t.Errorf("Expected 'test' to be masked, got '%s'", filtered)
	}
}

func TestProfanityFilter_Replace(t *testing.T) {
	ctx := context.Background()
	filter := NewProfanityFilter(FilterModeReplace)
	filter.AddWords("bad")
	filter.SetReplacement("[removed]")

	filtered, blocked, _ := filter.Filter(ctx, "This is bad", "user-1")
	if blocked {
		t.Error("Expected message not to be blocked")
	}
	if !strings.Contains(filtered, "[removed]") {
		t.Errorf("Expected '[removed]' in output, got '%s'", filtered)
	}
}

func TestProfanityFilter_Pattern(t *testing.T) {
	ctx := context.Background()
	filter := NewProfanityFilter(FilterModeBlock)
	err := filter.AddPattern(`b[a4]d`) // Matches "bad" or "b4d"
	if err != nil {
		t.Fatalf("AddPattern failed: %v", err)
	}

	_, blocked, _ := filter.Filter(ctx, "This is b4d", "user-1")
	if !blocked {
		t.Error("Expected message to be blocked by pattern")
	}
}

func TestSpamFilter_Caps(t *testing.T) {
	ctx := context.Background()
	filter := NewSpamFilter()

	filtered, _, _ := filter.Filter(ctx, "THIS IS ALL CAPS MESSAGE TEST", "user-1")
	if filtered == "THIS IS ALL CAPS MESSAGE TEST" {
		t.Error("Expected caps to be converted to lowercase")
	}
}

func TestSpamFilter_RepeatedChars(t *testing.T) {
	ctx := context.Background()
	filter := NewSpamFilter()

	filtered, _, _ := filter.Filter(ctx, "Hellooooooooo", "user-1")
	// Should reduce repeated 'o' characters
	if strings.Count(filtered, "o") > 5 {
		t.Errorf("Expected repeated chars to be reduced, got '%s'", filtered)
	}
}

func TestSpamFilter_TooManyUrls(t *testing.T) {
	ctx := context.Background()
	filter := NewSpamFilter()

	content := "Check http://a.com http://b.com http://c.com http://d.com"
	_, blocked, _ := filter.Filter(ctx, content, "user-1")
	if !blocked {
		t.Error("Expected message with too many URLs to be blocked")
	}
}

func TestChainedFilter(t *testing.T) {
	ctx := context.Background()

	profanity := NewProfanityFilter(FilterModeMask)
	profanity.AddWords("bad")

	spam := NewSpamFilter()

	chain := NewChainedFilter(profanity, spam)

	// Test that both filters are applied
	filtered, _, _ := chain.Filter(ctx, "bad WORD IN CAPS", "user-1")

	// Should have masked "bad" and lowercased caps
	if strings.Contains(filtered, "bad") {
		t.Error("Expected 'bad' to be masked")
	}
}

func TestExtractMentions(t *testing.T) {
	tests := []struct {
		content  string
		expected []string
	}{
		{"Hello @user1", []string{"user1"}},
		{"@user1 @user2 check this", []string{"user1", "user2"}},
		{"No mentions here", []string{}},
		{"Email test@example.com not a mention", []string{}}, // @ in email shouldn't match
	}

	for _, tt := range tests {
		mentions := ExtractMentions(tt.content)
		if len(mentions) != len(tt.expected) {
			t.Errorf("ExtractMentions(%q): expected %d mentions, got %d", tt.content, len(tt.expected), len(mentions))
			continue
		}
		for i, m := range mentions {
			if m != tt.expected[i] {
				t.Errorf("ExtractMentions(%q): expected %s, got %s", tt.content, tt.expected[i], m)
			}
		}
	}
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		content   string
		command   string
		args      string
		isCommand bool
	}{
		{"/me waves", "me", "waves", true},
		{"/whisper user1 hello", "whisper", "user1 hello", true},
		{"/help", "help", "", true},
		{"Hello world", "", "", false},
		{"/SHOUT loud", "shout", "loud", true}, // Commands should be lowercase
	}

	for _, tt := range tests {
		cmd, args, isCmd := ParseCommand(tt.content)
		if isCmd != tt.isCommand {
			t.Errorf("ParseCommand(%q): expected isCommand=%v, got %v", tt.content, tt.isCommand, isCmd)
			continue
		}
		if isCmd {
			if cmd != tt.command {
				t.Errorf("ParseCommand(%q): expected command=%s, got %s", tt.content, tt.command, cmd)
			}
			if args != tt.args {
				t.Errorf("ParseCommand(%q): expected args=%s, got %s", tt.content, tt.args, args)
			}
		}
	}
}

func TestFormatEmote(t *testing.T) {
	result := FormatEmote("Player1", "waves hello")
	expected := "* Player1 waves hello"
	if result != expected {
		t.Errorf("FormatEmote: expected %q, got %q", expected, result)
	}
}

func TestUUIDv7Generation(t *testing.T) {
	// Generate multiple UUIDs and ensure they're unique
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateUUIDv7()
		if ids[id] {
			t.Errorf("Duplicate UUID generated: %s", id)
		}
		ids[id] = true

		// Basic UUID format validation
		if len(id) != 36 {
			t.Errorf("Invalid UUID length: %s", id)
		}
	}
}
