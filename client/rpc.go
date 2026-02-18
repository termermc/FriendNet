package client

import (
	"context"
	"errors"
	"io"

	"connectrpc.com/connect"
	"friendnet.org/client/room"
	"friendnet.org/common"
	"friendnet.org/protocol"
	v1 "friendnet.org/protocol/pb/clientrpc/v1"
	"friendnet.org/protocol/pb/clientrpc/v1/clientrpcv1connect"
	pb "friendnet.org/protocol/pb/v1"
)

var errServerNotFound = connect.NewError(connect.CodeNotFound, errors.New("server not found"))
var errInvalidUsername = connect.NewError(connect.CodeInvalidArgument, errors.New("invalid username"))
var errInvalidRoomName = connect.NewError(connect.CodeInvalidArgument, errors.New("invalid room name"))
var errPathNotDir = connect.NewError(connect.CodeInvalidArgument, errors.New("path is not a directory"))

type RpcServer struct {
	client *MultiClient
}

func (s *RpcServer) serverToInfo(srv Server) *v1.ServerInfo {
	return &v1.ServerInfo{
		Uuid:      srv.Uuid,
		Name:      srv.Name,
		Address:   srv.Address(),
		Room:      srv.Room().String(),
		Username:  srv.Username().String(),
		CreatedTs: srv.CreatedTs.Unix(),
	}
}
func (s *RpcServer) metaToInfo(meta *pb.MsgFileMeta) *v1.FileMeta {
	return &v1.FileMeta{
		Name:  meta.Name,
		IsDir: meta.IsDir,
		Size:  meta.Size,
	}
}

func (s *RpcServer) Stop(ctx context.Context, request *v1.StopRequest) (*v1.StopResponse, error) {
	if err := s.client.Close(); err != nil {
		return nil, err
	}

	return &v1.StopResponse{}, nil
}

func (s *RpcServer) GetServers(ctx context.Context, request *v1.GetServersRequest) (*v1.GetServersResponse, error) {
	servers := s.client.GetAll()

	infos := make([]*v1.ServerInfo, len(servers))
	for i, srv := range servers {
		infos[i] = s.serverToInfo(srv)
	}

	return &v1.GetServersResponse{
		Servers: infos,
	}, nil
}

func (s *RpcServer) CreateServer(ctx context.Context, request *v1.CreateServerRequest) (*v1.CreateServerResponse, error) {
	room, roomOk := common.NormalizeRoomName(request.Room)
	if !roomOk {
		return nil, errInvalidRoomName
	}
	username, usernameOk := common.NormalizeUsername(request.Username)
	if !usernameOk {
		return nil, errInvalidUsername
	}

	srv, err := s.client.Create(
		ctx,
		request.Name,
		request.Address,
		room,
		username,
		request.Password,
	)
	if err != nil {
		return nil, err
	}

	return &v1.CreateServerResponse{
		Server: s.serverToInfo(srv),
	}, nil
}

func (s *RpcServer) DeleteServer(ctx context.Context, request *v1.DeleteServerRequest) (*v1.DeleteServerResponse, error) {
	_, has := s.client.GetByUuid(request.Uuid)
	if !has {
		return nil, errServerNotFound
	}

	if err := s.client.DeleteByUuid(ctx, request.Uuid); err != nil {
		return nil, err
	}

	return &v1.DeleteServerResponse{}, nil
}

func (s *RpcServer) ConnectServer(ctx context.Context, request *v1.ConnectServerRequest) (*v1.ConnectServerResponse, error) {
	srv, has := s.client.GetByUuid(request.Uuid)
	if !has {
		return nil, errServerNotFound
	}

	srv.Connect()

	return &v1.ConnectServerResponse{}, nil
}

func (s *RpcServer) DisconnectServer(ctx context.Context, request *v1.DisconnectServerRequest) (*v1.DisconnectServerResponse, error) {
	srv, has := s.client.GetByUuid(request.Uuid)
	if !has {
		return nil, errServerNotFound
	}

	srv.Disconnect()

	return &v1.DisconnectServerResponse{}, nil
}

func (s *RpcServer) UpdateServer(ctx context.Context, request *v1.UpdateServerRequest) (*v1.UpdateServerResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s *RpcServer) GetShares(ctx context.Context, request *v1.GetSharesRequest) (*v1.GetSharesResponse, error) {

	//TODO implement me
	panic("implement me")
}

func (s *RpcServer) CreateShare(ctx context.Context, request *v1.CreateShareRequest) (*v1.CreateShareResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s *RpcServer) DeleteShare(ctx context.Context, request *v1.DeleteShareRequest) (*v1.DeleteShareResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s *RpcServer) GetDirFiles(ctx context.Context, request *v1.GetDirFilesRequest, res *connect.ServerStream[v1.GetDirFilesResponse]) error {
	username, usernameOk := common.NormalizeUsername(request.Username)
	if !usernameOk {
		return errInvalidUsername
	}

	srv, has := s.client.GetByUuid(request.ServerUuid)
	if !has {
		return errServerNotFound
	}

	return srv.Do(ctx, func(ctx context.Context, c *room.Conn) error {
		peer := c.GetVirtualC2cConn(username)
		stream, err := peer.GetDirFiles(request.Path)
		if err != nil {
			return err
		}

		for {
			var msg *pb.MsgDirFiles
			msg, err = stream.ReadNext()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}

				var protoMsgErr protocol.ProtoMsgError
				if errors.As(err, &protoMsgErr) {
					if protoMsgErr.Msg.Type == pb.ErrType_ERR_TYPE_PATH_NOT_DIRECTORY {
						return errPathNotDir
					}
				}

				return err
			}

			// I'd preallocate the content slice, but I'm not sure if Send holds a reference to the message.
			content := make([]*v1.FileMeta, len(msg.Files))
			for i, file := range msg.Files {
				content[i] = s.metaToInfo(file)
			}
			err = res.Send(&v1.GetDirFilesResponse{
				Content: content,
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *RpcServer) GetFileMeta(ctx context.Context, request *v1.GetFileMetaRequest) (*v1.GetFileMetaResponse, error) {
	username, usernameOk := common.NormalizeUsername(request.Username)
	if !usernameOk {
		return nil, errInvalidUsername
	}

	srv, has := s.client.GetByUuid(request.ServerUuid)
	if !has {
		return nil, errServerNotFound
	}

	return DoValue(srv.ConnNanny, ctx, func(ctx context.Context, c *room.Conn) (*v1.GetFileMetaResponse, error) {
		peer := c.GetVirtualC2cConn(username)
		meta, err := peer.GetFileMeta(request.Path)
		if err != nil {
			return nil, err
		}

		return &v1.GetFileMetaResponse{
			Meta: s.metaToInfo(meta),
		}, nil
	})
}

func NewRpcServer(client *MultiClient) *RpcServer {
	return &RpcServer{
		client: client,
	}
}

func (s *RpcServer) Close() error {
	return nil
}

var _ clientrpcv1connect.ClientRpcServiceHandler = (*RpcServer)(nil)
