package main

import (
	"context"
	"fmt"
	"strings"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

type AuthStore struct {
	rooms map[string]map[string]string
}

func NewAuthStore(cfg ServerConfig) (*AuthStore, error) {
	rooms := make(map[string]map[string]string)
	for _, room := range cfg.Rooms {
		if room.Name == "" {
			return nil, fmt.Errorf("room name is required")
		}
		roomKey := strings.ToLower(room.Name)
		users := make(map[string]string)
		for _, user := range room.Users {
			if user.Username == "" {
				return nil, fmt.Errorf("username required in room %q", room.Name)
			}
			users[strings.ToLower(user.Username)] = user.Password
		}
		rooms[roomKey] = users
	}

	return &AuthStore{rooms: rooms}, nil
}

func (a *AuthStore) Handler(_ context.Context, _ *protocol.ProtoServerClient, msg *pb.MsgAuthenticate) (*pb.MsgAuthAccepted, *pb.MsgAuthRejected, error) {
	if msg == nil {
		message := "missing credentials"
		return nil, &pb.MsgAuthRejected{
			Reason:  pb.AuthRejectionReason_AUTH_REJECTION_REASON_INVALID_CREDENTIALS,
			Message: &message,
		}, nil
	}

	roomUsers, ok := a.rooms[strings.ToLower(msg.Room)]
	if !ok {
		message := "unknown room"
		return nil, &pb.MsgAuthRejected{
			Reason:  pb.AuthRejectionReason_AUTH_REJECTION_REASON_INVALID_CREDENTIALS,
			Message: &message,
		}, nil
	}

	expected, ok := roomUsers[strings.ToLower(msg.Username)]
	if !ok || expected != msg.Password {
		message := "invalid credentials"
		return nil, &pb.MsgAuthRejected{
			Reason:  pb.AuthRejectionReason_AUTH_REJECTION_REASON_INVALID_CREDENTIALS,
			Message: &message,
		}, nil
	}

	return &pb.MsgAuthAccepted{}, nil, nil
}

func (a *AuthStore) HandlerWithRegistry(registry *ClientRegistry) protocol.ServerAuthHandler {
	return func(ctx context.Context, client *protocol.ProtoServerClient, msg *pb.MsgAuthenticate) (*pb.MsgAuthAccepted, *pb.MsgAuthRejected, error) {
		accepted, rejected, err := a.Handler(ctx, client, msg)
		if err != nil || rejected != nil || accepted == nil {
			return accepted, rejected, err
		}
		if registry != nil {
			if err := registry.Register(client, msg.Room, msg.Username); err != nil {
				message := err.Error()
				return nil, &pb.MsgAuthRejected{
					Reason:  pb.AuthRejectionReason_AUTH_REJECTION_REASON_INVALID_CREDENTIALS,
					Message: &message,
				}, nil
			}
		}
		return accepted, nil, nil
	}
}
