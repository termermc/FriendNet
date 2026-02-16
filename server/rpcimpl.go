package server

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"

	"connectrpc.com/connect"
	"friendnet.org/common"
	v1 "friendnet.org/protocol/pb/serverrpc/v1"
	"friendnet.org/protocol/pb/serverrpc/v1/serverrpcv1connect"
	"friendnet.org/server/room"
)

var errRoomNotFound = connect.NewError(connect.CodeNotFound, errors.New("room not found"))
var errUserNotOnline = connect.NewError(connect.CodeNotFound, errors.New("user not online"))
var errAccountNotFound = connect.NewError(connect.CodeNotFound, errors.New("account not found"))
var errRoomExists = connect.NewError(connect.CodeAlreadyExists, errors.New("room already exists"))
var errAccountExists = connect.NewError(connect.CodeAlreadyExists, errors.New("account already exists"))
var errInvalidRoomName = connect.NewError(connect.CodeInvalidArgument, errors.New("invalid room name"))
var errInvalidUsername = connect.NewError(connect.CodeInvalidArgument, errors.New("invalid username"))

type rpcServerImpl struct {
	s *RpcServer
}

var _ serverrpcv1connect.ServerRpcServiceHandler = (*rpcServerImpl)(nil)

func (s *rpcServerImpl) roomToInfo(r *room.Room) *v1.RoomInfo {
	if r == nil {
		return nil
	}
	return &v1.RoomInfo{
		Name:            r.Name.String(),
		OnlineUserCount: uint32(r.ClientCount()),
	}
}
func (s *rpcServerImpl) clientToInfo(c *room.Client) *v1.OnlineUserInfo {
	return &v1.OnlineUserInfo{
		Username: c.Username.String(),
	}
}

func (s *rpcServerImpl) getRoom(name string) (*room.Room, error) {
	roomName, ok := common.NormalizeRoomName(name)
	if !ok {
		return nil, errRoomNotFound
	}

	r, has := s.s.server.RoomManager.GetRoomByName(roomName)
	if !has {
		return nil, errRoomNotFound
	}

	return r, nil
}
func (s *rpcServerImpl) getClient(r *room.Room, username string) (*room.Client, error) {
	uName, ok := common.NormalizeUsername(username)
	if !ok {
		return nil, errUserNotOnline
	}

	client, has := r.GetClientByUsername(uName)
	if !has {
		return nil, errUserNotOnline
	}

	return client, nil
}
func (s *rpcServerImpl) getOrGenPass(pass string) (string, bool) {
	if pass == "" {
		var buf [12]byte
		_, _ = rand.Read(buf[:])
		return base64.RawURLEncoding.EncodeToString(buf[:]), true
	}
	return pass, false
}

func (s *rpcServerImpl) GetRooms(context.Context, *v1.GetRoomsRequest) (*v1.GetRoomsResponse, error) {
	rooms := s.s.server.RoomManager.GetAll()
	infos := make([]*v1.RoomInfo, len(rooms))
	for i, r := range rooms {
		infos[i] = s.roomToInfo(r)
	}

	return &v1.GetRoomsResponse{
		Rooms: infos,
	}, nil
}
func (s *rpcServerImpl) GetRoomInfo(_ context.Context, req *v1.GetRoomInfoRequest) (*v1.GetRoomInfoResponse, error) {
	r, err := s.getRoom(req.Name)
	if err != nil {
		return nil, err
	}

	return &v1.GetRoomInfoResponse{
		Room: s.roomToInfo(r),
	}, nil
}
func (s *rpcServerImpl) GetOnlineUsers(_ context.Context, req *v1.GetOnlineUsersRequest, stream *connect.ServerStream[v1.GetOnlineUsersResponse]) error {
	r, err := s.getRoom(req.Room)
	if err != nil {
		return err
	}

	clients := r.GetAllClients()
	infos := make([]*v1.OnlineUserInfo, len(clients))
	for i, c := range clients {
		infos[i] = s.clientToInfo(c)
	}

	const pageSize = 50

	// Send pages of statuses.
	sent := 0
	for sent < len(clients) {
		end := sent + pageSize
		if end > len(clients) {
			end = len(clients)
		}

		err = stream.Send(&v1.GetOnlineUsersResponse{
			Users: infos[sent:end],
		})
		if err != nil {
			return err
		}

		// We could have sent less than pageSize, but in that case it would break anyway, so we don't care about being accurate here.
		sent += pageSize
	}

	return nil
}
func (s *rpcServerImpl) GetOnlineUserInfo(_ context.Context, req *v1.GetOnlineUserInfoRequest) (*v1.GetOnlineUserInfoResponse, error) {
	r, err := s.getRoom(req.Room)
	if err != nil {
		return nil, err
	}

	client, err := s.getClient(r, req.Username)
	if err != nil {
		return nil, err
	}

	return &v1.GetOnlineUserInfoResponse{
		User: s.clientToInfo(client),
	}, nil
}
func (s *rpcServerImpl) CreateRoom(ctx context.Context, req *v1.CreateRoomRequest) (*v1.CreateRoomResponse, error) {
	name, ok := common.NormalizeRoomName(req.Name)
	if !ok {
		return nil, errInvalidRoomName
	}

	r, err := s.s.server.RoomManager.CreateRoom(ctx, name)
	if err != nil {
		if errors.Is(err, room.ErrRoomExists) {
			return nil, errRoomExists
		}

		return nil, err
	}

	return &v1.CreateRoomResponse{
		Room: s.roomToInfo(r),
	}, nil
}
func (s *rpcServerImpl) DeleteRoom(ctx context.Context, req *v1.DeleteRoomRequest) (*v1.DeleteRoomResponse, error) {
	r, err := s.getRoom(req.Name)
	if err != nil {
		return nil, err
	}

	err = s.s.server.RoomManager.DeleteRoomByName(ctx, r.Name)
	if err != nil {
		return nil, err
	}

	return &v1.DeleteRoomResponse{}, nil
}
func (s *rpcServerImpl) CreateAccount(ctx context.Context, req *v1.CreateAccountRequest) (*v1.CreateAccountResponse, error) {
	r, err := s.getRoom(req.Room)
	if err != nil {
		return nil, err
	}

	username, ok := common.NormalizeUsername(req.Username)
	if !ok {
		return nil, errInvalidUsername
	}

	pass, wasGen := s.getOrGenPass(req.Password)

	err = r.CreateAccount(ctx, username, pass)
	if err != nil {
		if errors.Is(err, room.ErrAccountExists) {
			return nil, errAccountExists
		}
		return nil, err
	}

	res := &v1.CreateAccountResponse{}
	if wasGen {
		res.GeneratedPassword = &pass
	}
	return res, nil
}
func (s *rpcServerImpl) DeleteAccount(ctx context.Context, req *v1.DeleteAccountRequest) (*v1.DeleteAccountResponse, error) {
	r, err := s.getRoom(req.Room)
	if err != nil {
		return nil, err
	}

	username, ok := common.NormalizeUsername(req.Username)
	if !ok {
		return nil, errAccountNotFound
	}

	err = r.DeleteAccount(ctx, username)
	if err != nil {
		if errors.Is(err, room.ErrNoSuchAccount) {
			return nil, errAccountNotFound
		}

		return nil, err
	}

	return nil, nil
}
func (s *rpcServerImpl) UpdateAccountPassword(ctx context.Context, req *v1.UpdateAccountPasswordRequest) (*v1.UpdateAccountPasswordResponse, error) {
	r, err := s.getRoom(req.Room)
	if err != nil {
		return nil, err
	}

	username, ok := common.NormalizeUsername(req.Username)
	if !ok {
		return nil, errAccountNotFound
	}

	err = r.UpdateAccountPassword(ctx, username, req.Password)
	if err != nil {
		if errors.Is(err, room.ErrNoSuchAccount) {
			return nil, errAccountNotFound
		}
		return nil, err
	}

	return nil, nil
}
