package chat

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	chatv1 "Hermes/gen/chat/v1"
)

var (
	// ErrMessageTooLong is returned when a message exceeds the maximum length
	ErrMessageTooLong = errors.New("message exceeds maximum length")

	// ErrMessageEmpty is returned when a message is empty
	ErrMessageEmpty = errors.New("message content is empty")

	// ErrMessageInvalidUTF8 is returned when a message contains invalid UTF-8
	ErrMessageInvalidUTF8 = errors.New("message contains invalid UTF-8")

	// ErrMessageFiltered is returned when a message is blocked by content filter
	ErrMessageFiltered = errors.New("message blocked by content filter")

	// ErrMessageRateLimited is returned when sending too many messages
	ErrMessageRateLimited = errors.New("message rate limited")

	// ErrInvalidMessageType is returned when the message type is not allowed
	ErrInvalidMessageType = errors.New("invalid message type for this context")
)

// MessageConfig holds configuration for message processing
type MessageConfig struct {
	MaxLength          int           // Maximum message length in characters
	MinLength          int           // Minimum message length (0 = no minimum)
	AllowEmptyMessages bool          // Whether to allow empty messages
	StripWhitespace    bool          // Whether to strip leading/trailing whitespace
	MaxMentions        int           // Maximum number of @mentions per message
	RateLimit          int           // Messages per minute (0 = no limit)
	RateLimitWindow    time.Duration // Rate limit window
}

// DefaultMessageConfig returns sensible defaults for message processing
func DefaultMessageConfig() *MessageConfig {
	return &MessageConfig{
		MaxLength:          2000, // Discord-like limit
		MinLength:          1,
		AllowEmptyMessages: false,
		StripWhitespace:    true,
		MaxMentions:        10,
		RateLimit:          30,           // 30 messages per minute
		RateLimitWindow:    time.Minute,
	}
}

// ContentFilter defines the interface for message content filtering
// This allows pluggable implementations (profanity, spam, etc.)
type ContentFilter interface {
	// Filter checks the message content and returns filtered content
	// If the message should be blocked, returns empty string and an error
	Filter(ctx context.Context, content string, userID string) (filtered string, blocked bool, err error)

	// Name returns the filter name for logging
	Name() string
}

// MessageProcessor handles message validation, ID generation, and filtering
type MessageProcessor struct {
	config  *MessageConfig
	filters []ContentFilter
}

// NewMessageProcessor creates a new message processor with the given config
func NewMessageProcessor(config *MessageConfig) *MessageProcessor {
	if config == nil {
		config = DefaultMessageConfig()
	}
	return &MessageProcessor{
		config:  config,
		filters: make([]ContentFilter, 0),
	}
}

// AddFilter adds a content filter to the processing pipeline
func (p *MessageProcessor) AddFilter(filter ContentFilter) {
	p.filters = append(p.filters, filter)
}

// Process validates and enriches a message with server-side data
// This should be called before broadcasting a message
func (p *MessageProcessor) Process(ctx context.Context, msg *chatv1.Message, user *chatv1.User) error {
	// Generate UUID v7 for time-ordered IDs
	if msg.Id == "" {
		msg.Id = generateUUIDv7()
	}

	// Set server-side timestamps
	now := time.Now().UnixMilli()
	msg.Timestamp = now
	msg.ServerTime = now

	// Set username from user if provided
	if user != nil && msg.Username == "" {
		msg.Username = user.Username
	}

	// Set default message type if not specified
	if msg.Type == chatv1.MessageType_MESSAGE_TYPE_UNSPECIFIED {
		msg.Type = chatv1.MessageType_MESSAGE_TYPE_CHAT
	}

	// Validate the message
	if err := p.Validate(msg); err != nil {
		return err
	}

	// Apply content filters
	if err := p.applyFilters(ctx, msg); err != nil {
		return err
	}

	return nil
}

// Validate checks if a message meets all requirements
func (p *MessageProcessor) Validate(msg *chatv1.Message) error {
	content := msg.Content

	// Strip whitespace if configured
	if p.config.StripWhitespace {
		content = strings.TrimSpace(content)
		msg.Content = content
	}

	// Check for empty content
	if !p.config.AllowEmptyMessages && len(content) == 0 {
		return ErrMessageEmpty
	}

	// Check UTF-8 validity
	if !utf8.ValidString(content) {
		return ErrMessageInvalidUTF8
	}

	// Check length (in runes for proper unicode handling)
	runeCount := utf8.RuneCountInString(content)
	if runeCount > p.config.MaxLength {
		return ErrMessageTooLong
	}

	if runeCount < p.config.MinLength {
		return ErrMessageEmpty
	}

	// Check mention count
	if p.config.MaxMentions > 0 {
		mentionCount := countMentions(content)
		if mentionCount > p.config.MaxMentions {
			return errors.New("too many mentions in message")
		}
	}

	// Validate channel ID
	if msg.ChannelId == "" {
		return errors.New("channel ID is required")
	}

	// Validate user ID
	if msg.UserId == "" {
		return errors.New("user ID is required")
	}

	return nil
}

// applyFilters runs all content filters on the message
func (p *MessageProcessor) applyFilters(ctx context.Context, msg *chatv1.Message) error {
	for _, filter := range p.filters {
		filtered, blocked, err := filter.Filter(ctx, msg.Content, msg.UserId)
		if err != nil {
			return err
		}
		if blocked {
			return ErrMessageFiltered
		}
		msg.Content = filtered
	}
	return nil
}

// generateUUIDv7 generates a UUID v7 (time-ordered) for message IDs
// UUID v7 embeds a Unix timestamp for natural time-ordering
func generateUUIDv7() string {
	// google/uuid v1.4+ supports UUID v7
	id, err := uuid.NewV7()
	if err != nil {
		// Fallback to v4 if v7 fails
		return uuid.New().String()
	}
	return id.String()
}

// countMentions counts the number of @mentions in a message
func countMentions(content string) int {
	// Match @username patterns at word boundaries (not in emails)
	re := regexp.MustCompile(`(?:^|[^a-zA-Z0-9_.+-])@(\w+)`)
	return len(re.FindAllString(content, -1))
}

// ExtractMentions extracts mentioned usernames from message content
// Excludes email addresses (user@domain.com won't match)
func ExtractMentions(content string) []string {
	// Match @username only when preceded by non-word chars or start of string
	re := regexp.MustCompile(`(?:^|[^a-zA-Z0-9_.+-])@(\w+)`)
	matches := re.FindAllStringSubmatch(content, -1)
	mentions := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			mentions = append(mentions, match[1])
		}
	}
	return mentions
}

// FormatEmote formats an emote message (/me action)
func FormatEmote(username, action string) string {
	return "* " + username + " " + action
}

// ParseCommand checks if a message is a command and returns the command name and args
func ParseCommand(content string) (command string, args string, isCommand bool) {
	if !strings.HasPrefix(content, "/") {
		return "", "", false
	}

	parts := strings.SplitN(content[1:], " ", 2)
	command = strings.ToLower(parts[0])
	if len(parts) > 1 {
		args = parts[1]
	}
	return command, args, true
}
