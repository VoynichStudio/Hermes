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
	user, err := auth.Authenticate(req.Header())
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}

	channelID := req.Msg.ChannelToJoin.Id

	// Get or create the channel
	channel := s.GetOrCreateChannel(req.Msg.ChannelToJoin)

	// Add user to the channel
	s.AddUserToChannel(channel.Id, user)

	// Register the user's message stream
	userStream := s.RegisterUserStream(user.Id)
	defer s.UnregisterUserStream(user.Id)

	// Stream messages to the user
	for {
		select {
		case <-ctx.Done():
			return nil
		case msg := <-userStream:
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
	_, err := auth.Authenticate(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}

	msg := req.Msg.SentMessage

	if err := s.BroadcastToChannel(msg.ChannelId, msg); err != nil {
		if err == ErrChannelNotFound {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&chatv1.SendMessageResponse{Ok: true}), nil
}
