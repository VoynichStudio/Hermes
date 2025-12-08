package chat

import "errors"

var (
	// ErrChannelNotFound is returned when a channel does not exist
	ErrChannelNotFound = errors.New("channel not found")

	// ErrChannelExists is returned when trying to create a channel that already exists
	ErrChannelExists = errors.New("channel already exists")

	// ErrUserNotInChannel is returned when a user is not a member of a channel
	ErrUserNotInChannel = errors.New("user not in channel")

	// ErrUserNotFound is returned when a user session does not exist
	ErrUserNotFound = errors.New("user not found")

	// ErrMessageNotFound is returned when a message does not exist
	ErrMessageNotFound = errors.New("message not found")

	// ErrSessionExists is returned when trying to create a duplicate session
	ErrSessionExists = errors.New("session already exists")
)
