package chat

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"github.com/google/uuid"

	chatv1 "Hermes/gen/chat/v1"
	"Hermes/pkg/auth"
)

var (
	// ErrNotAuthorized is returned when a user doesn't have permission
	ErrNotAuthorized = errors.New("not authorized")

	// ErrChannelFull is returned when a channel has reached max users
	ErrChannelFull = errors.New("channel is full")
)

// CreateChannel creates a new channel
func (s *Server) CreateChannel(
	ctx context.Context,
	req *connect.Request[chatv1.CreateChannelRequest],
) (*connect.Response[chatv1.CreateChannelResponse], error) {
	// Get user from context (set by auth middleware)
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, auth.ErrNoToken)
	}

	// Generate channel ID
	channelID := uuid.New().String()

	// Create the channel
	channel := &chatv1.Channel{
		Id:          channelID,
		Name:        req.Msg.Name,
		Label:       req.Msg.Label,
		Type:        req.Msg.Type,
		Description: req.Msg.Description,
		CreatorId:   user.Id,
		Users:       []*chatv1.User{},
		Permissions: []*chatv1.ChannelPermission{
			{
				UserId: user.Id,
				Permissions: []chatv1.Permission{
					chatv1.Permission_PERMISSION_READ,
					chatv1.Permission_PERMISSION_WRITE,
					chatv1.Permission_PERMISSION_MANAGE,
					chatv1.Permission_PERMISSION_ADMIN,
				},
			},
		},
		MaxUsers:  req.Msg.MaxUsers,
		IsPrivate: req.Msg.IsPrivate,
	}

	// Store the channel
	if err := s.channelRepo.Create(ctx, channel); err != nil {
		if err == ErrChannelExists {
			return nil, connect.NewError(connect.CodeAlreadyExists, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&chatv1.CreateChannelResponse{
		Channel: channel,
	}), nil
}

// DeleteChannel deletes a channel
func (s *Server) DeleteChannel(
	ctx context.Context,
	req *connect.Request[chatv1.DeleteChannelRequest],
) (*connect.Response[chatv1.DeleteChannelResponse], error) {
	// Get user from context
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, auth.ErrNoToken)
	}

	// Get the channel
	channel, err := s.channelRepo.Get(ctx, req.Msg.ChannelId)
	if err != nil {
		if err == ErrChannelNotFound {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Check if user has admin permission
	if !hasPermission(channel, user.Id, chatv1.Permission_PERMISSION_ADMIN) {
		return nil, connect.NewError(connect.CodePermissionDenied, ErrNotAuthorized)
	}

	// Delete the channel
	if err := s.channelRepo.Delete(ctx, req.Msg.ChannelId); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&chatv1.DeleteChannelResponse{Ok: true}), nil
}

// GetChannel retrieves channel details
func (s *Server) GetChannel(
	ctx context.Context,
	req *connect.Request[chatv1.GetChannelRequest],
) (*connect.Response[chatv1.GetChannelResponse], error) {
	// Get user from context
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, auth.ErrNoToken)
	}

	// Get the channel
	channel, err := s.channelRepo.Get(ctx, req.Msg.ChannelId)
	if err != nil {
		if err == ErrChannelNotFound {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Check if user has read permission for private channels
	if channel.IsPrivate && !hasPermission(channel, user.Id, chatv1.Permission_PERMISSION_READ) {
		return nil, connect.NewError(connect.CodePermissionDenied, ErrNotAuthorized)
	}

	return connect.NewResponse(&chatv1.GetChannelResponse{
		Channel: channel,
	}), nil
}

// ListChannels returns a list of available channels
func (s *Server) ListChannels(
	ctx context.Context,
	req *connect.Request[chatv1.ListChannelsRequest],
) (*connect.Response[chatv1.ListChannelsResponse], error) {
	// Get user from context
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, auth.ErrNoToken)
	}

	// Get all channels
	allChannels, err := s.channelRepo.List(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Filter channels
	var channels []*chatv1.Channel
	for _, ch := range allChannels {
		// Filter by type if specified
		if req.Msg.Type != chatv1.ChannelType_CHANNEL_TYPE_UNSPECIFIED && ch.Type != req.Msg.Type {
			continue
		}

		// Skip private channels the user doesn't have access to
		if ch.IsPrivate && !hasPermission(ch, user.Id, chatv1.Permission_PERMISSION_READ) {
			continue
		}

		channels = append(channels, ch)
	}

	// Apply pagination (simple implementation)
	pageSize := int(req.Msg.PageSize)
	if pageSize <= 0 {
		pageSize = 50 // Default page size
	}
	if pageSize > 100 {
		pageSize = 100 // Max page size
	}

	startIdx := 0
	if req.Msg.PageToken != "" {
		// Simple token: just the index
		for i, ch := range channels {
			if ch.Id == req.Msg.PageToken {
				startIdx = i + 1
				break
			}
		}
	}

	endIdx := startIdx + pageSize
	if endIdx > len(channels) {
		endIdx = len(channels)
	}

	var nextPageToken string
	if endIdx < len(channels) {
		nextPageToken = channels[endIdx-1].Id
	}

	return connect.NewResponse(&chatv1.ListChannelsResponse{
		Channels:      channels[startIdx:endIdx],
		NextPageToken: nextPageToken,
	}), nil
}

// LeaveChannel removes the current user from a channel
func (s *Server) LeaveChannel(
	ctx context.Context,
	req *connect.Request[chatv1.LeaveChannelRequest],
) (*connect.Response[chatv1.LeaveChannelResponse], error) {
	// Get user from context
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, auth.ErrNoToken)
	}

	// Remove user from channel
	if err := s.channelRepo.RemoveUser(ctx, req.Msg.ChannelId, user.Id); err != nil {
		if err == ErrChannelNotFound {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		// User not in channel is not an error for leave
		if err != ErrUserNotInChannel {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	// Also remove from session tracking if available
	_ = s.sessionManager.RemoveChannel(ctx, user.Id, req.Msg.ChannelId)

	return connect.NewResponse(&chatv1.LeaveChannelResponse{Ok: true}), nil
}

// UpdateChannelPermissions updates a user's permissions in a channel
func (s *Server) UpdateChannelPermissions(
	ctx context.Context,
	req *connect.Request[chatv1.UpdateChannelPermissionsRequest],
) (*connect.Response[chatv1.UpdateChannelPermissionsResponse], error) {
	// Get user from context
	currentUser, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, auth.ErrNoToken)
	}

	// Get the channel
	channel, err := s.channelRepo.Get(ctx, req.Msg.ChannelId)
	if err != nil {
		if err == ErrChannelNotFound {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Check if current user has manage permission
	if !hasPermission(channel, currentUser.Id, chatv1.Permission_PERMISSION_MANAGE) {
		return nil, connect.NewError(connect.CodePermissionDenied, ErrNotAuthorized)
	}

	// Update permissions
	found := false
	for _, perm := range channel.Permissions {
		if perm.UserId == req.Msg.UserId {
			perm.Permissions = req.Msg.Permissions
			found = true
			break
		}
	}

	if !found {
		channel.Permissions = append(channel.Permissions, &chatv1.ChannelPermission{
			UserId:      req.Msg.UserId,
			Permissions: req.Msg.Permissions,
		})
	}

	// Note: In a real implementation, we'd persist this change
	// For now with in-memory, the channel object is already updated

	return connect.NewResponse(&chatv1.UpdateChannelPermissionsResponse{Ok: true}), nil
}

// hasPermission checks if a user has a specific permission in a channel
func hasPermission(channel *chatv1.Channel, userID string, permission chatv1.Permission) bool {
	// Channel creator always has all permissions
	if channel.CreatorId == userID {
		return true
	}

	// Check explicit permissions
	for _, perm := range channel.Permissions {
		if perm.UserId == userID {
			for _, p := range perm.Permissions {
				if p == permission || p == chatv1.Permission_PERMISSION_ADMIN {
					return true
				}
			}
			return false
		}
	}

	// For public channels, everyone has read/write by default
	if !channel.IsPrivate {
		return permission == chatv1.Permission_PERMISSION_READ ||
			permission == chatv1.Permission_PERMISSION_WRITE
	}

	return false
}

// HasReadPermission checks if a user can read from a channel
func HasReadPermission(channel *chatv1.Channel, userID string) bool {
	return hasPermission(channel, userID, chatv1.Permission_PERMISSION_READ)
}

// HasWritePermission checks if a user can write to a channel
func HasWritePermission(channel *chatv1.Channel, userID string) bool {
	return hasPermission(channel, userID, chatv1.Permission_PERMISSION_WRITE)
}

// HasManagePermission checks if a user can manage a channel
func HasManagePermission(channel *chatv1.Channel, userID string) bool {
	return hasPermission(channel, userID, chatv1.Permission_PERMISSION_MANAGE)
}

// HasAdminPermission checks if a user has admin access to a channel
func HasAdminPermission(channel *chatv1.Channel, userID string) bool {
	return hasPermission(channel, userID, chatv1.Permission_PERMISSION_ADMIN)
}
