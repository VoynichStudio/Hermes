// main package for setting up
package main

import (
	"context"
	"net/http"
	"sync"
	"errors"
	"connectrpc.com/connect"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"

	"Hermes/lib/auth"        // generated by protoc-gen-go
	chatv1 "Hermes/gen/chat/v1"        // generated by protoc-gen-go
	"Hermes/gen/chat/v1/chatv1connect" // generated by protoc-gen-connect-go
)

//ChatServer something
type ChatServer struct {
	chatv1connect.UnimplementedChatServiceHandler
	chs map[string]*chatv1.Channel
	userStreams map[string]chan *chatv1.Message
	rwmu sync.RWMutex
}

// NewChatServer returns an instance of the chat server
func NewChatServer() *ChatServer {
	defaultChannels := make(map[string]*chatv1.Channel)
	defaultChannels["general"] = &chatv1.Channel{
		Id: "general",
		Label: "general",
		Users: []*chatv1.User{},
	}
	
	return &ChatServer{
		chs: defaultChannels,
		userStreams: make(map[string]chan *chatv1.Message),
	}
}

// JoinChannel gets a user and sets it as a subscriber of a channel to receive its messages 
func (s *ChatServer) JoinChannel(
	ctx context.Context,
	req *connect.Request[chatv1.JoinChannelRequest],
	stream *connect.ServerStream[chatv1.JoinChannelResponse],
) error {
	user, err := auth.Authenticate(req.Header())
	if err != nil {
		return connect.NewError(connect.CodeUnauthenticated, err)
	}
	channelID := req.Msg.ChannelToJoin.Id

	s.rwmu.Lock()
	channel, exists := s.chs[channelID]
	if !exists {
		channel = req.Msg.ChannelToJoin
		s.chs[req.Msg.ChannelToJoin.Id] = req.Msg.ChannelToJoin
	}

	channel.Users = append(channel.Users, user)
	userStream := make(chan *chatv1.Message, 10)
	s.userStreams[user.Id] = userStream
	s.rwmu.Unlock()
	defer func() {
		s.rwmu.Lock()
		delete(s.userStreams, user.Id)
		s.rwmu.Unlock()
	}()

	for {
		select {
		case <- ctx.Done():
			return nil
		case msg := <-userStream:
			if err := stream.Send(&chatv1.JoinChannelResponse{ MessageStream: msg }); err != nil {
				return err
			}
		}
	}
}

// SendMessage gets a message from a user and broadcasts it to all the subscribers of the channel
func (s *ChatServer) SendMessage(
	_ context.Context,
	req *connect.Request[chatv1.SendMessageRequest],
) (*connect.Response[chatv1.SendMessageResponse], error)  {
	_, err := auth.Authenticate(req.Header())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, err)
	}
	msg := req.Msg.SentMessage

	s.rwmu.RLock()
	ch, exists := s.chs[req.Msg.SentMessage.ChannelId]
  s.rwmu.RUnlock()
	
	if !exists {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("Channel not found"))
	}
	for _, u := range ch.Users {
		if ch, ok := s.userStreams[u.Id]; ok {
			ch <- msg
		}
	}
	s.rwmu.RUnlock()
	
	return connect.NewResponse(&chatv1.SendMessageResponse{Ok: true}), nil
}

func main() {
	cs := NewChatServer()
	mux := http.NewServeMux()

	path, handler := chatv1connect.NewChatServiceHandler(cs)
	mux.Handle(path, handler)
	http.ListenAndServe(
		"localhost:8080",
		// Use h2c so we can serve HTTP/2 without TLS.
		h2c.NewHandler(mux, &http2.Server{}),
	)
	
	
}


