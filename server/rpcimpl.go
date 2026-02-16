package server

import (
	"context"

	"connectrpc.com/connect"
	v1 "friendnet.org/protocol/pb/serverrpc/v1"
	"friendnet.org/protocol/pb/serverrpc/v1/serverrpcv1connect"
)

type rpcServerImpl struct {
	s *RpcServer
}

var _ serverrpcv1connect.ServerRpcServiceHandler = (*rpcServerImpl)(nil)

func (s *rpcServerImpl) GetRooms(context.Context, *v1.GetRoomsRequest) (*v1.GetRoomsResponse, error) {
	return nil, nil
}
func (s *rpcServerImpl) GetRoomInfo(context.Context, *v1.GetRoomInfoRequest) (*v1.GetRoomInfoResponse, error) {
	return nil, nil
}
func (s *rpcServerImpl) GetOnlineUsers(context.Context, *v1.GetOnlineUsersRequest, *connect.ServerStream[v1.GetOnlineUsersResponse]) error {
	return nil
}
func (s *rpcServerImpl) GetOnlineUserInfo(context.Context, *v1.GetOnlineUserInfoRequest) (*v1.GetOnlineUserInfoResponse, error) {
	return nil, nil
}
func (s *rpcServerImpl) CreateRoom(context.Context, *v1.CreateRoomRequest) (*v1.CreateRoomResponse, error) {
	return nil, nil
}
func (s *rpcServerImpl) DeleteRoom(context.Context, *v1.DeleteRoomRequest) (*v1.DeleteRoomResponse, error) {
	return nil, nil
}
func (s *rpcServerImpl) CreateAccount(context.Context, *v1.CreateAccountRequest) (*v1.CreateAccountResponse, error) {
	return nil, nil
}
func (s *rpcServerImpl) DeleteAccount(context.Context, *v1.DeleteAccountRequest) (*v1.DeleteAccountResponse, error) {
	return nil, nil
}
func (s *rpcServerImpl) UpdateAccountPassword(context.Context, *v1.UpdateAccountPasswordRequest) (*v1.UpdateAccountPasswordResponse, error) {
	return nil, nil
}
