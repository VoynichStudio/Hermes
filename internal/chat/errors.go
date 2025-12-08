package chat

import "errors"

var (
	// ErrChannelNotFound is returned when a channel does not exist
	ErrChannelNotFound = errors.New("channel not found")
)
