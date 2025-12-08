package chat

import (
	"context"

	"connectrpc.com/connect"

	chatv1 "Hermes/gen/chat/v1"
	"Hermes/pkg/auth"
)

// JoinChannel subscribes a user to a channel and streams messages to them
func (s *Server) JoinChannel(
	ctx context.Context,
	req *connect.Request[chatv1.JoinChannelRequest],
	stream *connect.ServerStream[chatv1.JoinChannelResponse],
) error {
	// Get user from context (set by auth middleware)
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return connect.NewError(connect.CodeUnauthenticated, auth.ErrNoToken)
	}

	// Get or create the channel
	channel, err := s.channelRepo.GetOrCreate(ctx, req.Msg.ChannelToJoin)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	// Add user to the channel
	if err := s.channelRepo.AddUser(ctx, channel.Id, user); err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	// Register the user's session and get message stream
	userStream, err := s.sessionManager.Register(ctx, user)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	// Track channel subscription in session
	if err := s.sessionManager.AddChannel(ctx, user.Id, channel.Id); err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}

	// Cleanup on disconnect
	defer func() {
		_ = s.sessionManager.RemoveChannel(ctx, user.Id, channel.Id)
		_ = s.sessionManager.Unregister(ctx, user.Id)
		_ = s.channelRepo.RemoveUser(ctx, channel.Id, user.Id)
	}()

	// Stream messages to the user
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg, ok := <-userStream:
			if !ok {
				// Channel closed
				return nil
			}
			if err := stream.Send(&chatv1.JoinChannelResponse{MessageStream: msg}); err != nil {
				return err
			}
		}
	}
}

// SendMessage broadcasts a message to all subscribers of a channel
func (s *Server) SendMessage(
	ctx context.Context,
	req *connect.Request[chatv1.SendMessageRequest],
) (*connect.Response[chatv1.SendMessageResponse], error) {
	// Get user from context (set by auth middleware)
	user, ok := auth.UserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, auth.ErrNoToken)
	}

	msg := req.Msg.SentMessage

	// Ensure the message UserId matches the authenticated user
	msg.UserId = user.Id

	// Process the message (generates ID, timestamps, validates, filters)
	if err := s.messageProcessor.Process(ctx, msg, user); err != nil {
		switch err {
		case ErrMessageEmpty:
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		case ErrMessageTooLong:
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		case ErrMessageInvalidUTF8:
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		case ErrMessageFiltered:
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		default:
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	// Broadcast using the broadcaster interface
	if err := s.broadcaster.Broadcast(ctx, msg.ChannelId, msg); err != nil {
		if err == ErrChannelNotFound {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&chatv1.SendMessageResponse{Ok: true}), nil
}
