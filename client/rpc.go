package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"time"

	"connectrpc.com/connect"
	"friendnet.org/client/clog"
	"friendnet.org/client/direct"
	"friendnet.org/client/event"
	"friendnet.org/client/room"
	"friendnet.org/client/share"
	"friendnet.org/client/storage"
	"friendnet.org/common"
	"friendnet.org/common/updater"
	"friendnet.org/protocol"
	v1 "friendnet.org/protocol/pb/clientrpc/v1"
	"friendnet.org/protocol/pb/clientrpc/v1/clientrpcv1connect"
	pb "friendnet.org/protocol/pb/v1"
)

var errServerNotFound = connect.NewError(connect.CodeNotFound, errors.New("server not found"))
var errInvalidUsername = connect.NewError(connect.CodeInvalidArgument, errors.New("invalid username"))
var errInvalidRoomName = connect.NewError(connect.CodeInvalidArgument, errors.New("invalid room name"))
var errPathNotDir = connect.NewError(connect.CodeInvalidArgument, errors.New("path is not a directory"))
var errShareNotFound = connect.NewError(connect.CodeNotFound, errors.New("share not found"))
var errFileNotFound = connect.NewError(connect.CodeNotFound, errors.New("file not found"))
var errIncorrectPassword = connect.NewError(connect.CodeInvalidArgument, errors.New("incorrect password"))
var errInvalidDefaultPort = connect.NewError(connect.CodeInvalidArgument, errors.New("default port must be between 1024 and 65535 (inclusive), or 0 for random"))
var errInvalidUpnpTimeout = connect.NewError(connect.CodeInvalidArgument, errors.New("UPnP timeout must be between 0 and 60000 (inclusive)"))
var errIndexingDisabled = connect.NewError(connect.CodeFailedPrecondition, errors.New("share has indexing disabled"))
var errEmptySearchQuery = connect.NewError(connect.CodeInvalidArgument, errors.New("search query cannot be empty"))
var errInvalidShareName = connect.NewError(connect.CodeInvalidArgument, share.ErrInvalidShareName)

type RpcServer struct {
	clogHandler     clog.Handler
	client          *MultiClient
	eventBus        *event.Bus
	updateChecker   *updater.UpdateChecker
	downloadManager *DownloadManager
	stopper         func()
}

func NewRpcServer(
	clogHandler clog.Handler,
	client *MultiClient,
	eventBus *event.Bus,
	updateChecker *updater.UpdateChecker,
	downloadManager *DownloadManager,
	stopper func(),
) *RpcServer {
	return &RpcServer{
		clogHandler:     clogHandler,
		client:          client,
		eventBus:        eventBus,
		updateChecker:   updateChecker,
		downloadManager: downloadManager,
		stopper:         stopper,
	}
}

func (s *RpcServer) Close() error {
	return nil
}

var _ clientrpcv1connect.ClientRpcServiceHandler = (*RpcServer)(nil)

func (s *RpcServer) serverToInfo(srv *Server) *v1.ServerInfo {
	return &v1.ServerInfo{
		State: &v1.ServerInfo_State{
			ConnState: srv.ConnNanny.State().ToRpcEnum(),
		},
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
func (s *RpcServer) shareRecToInfo(share storage.ShareRecord) *v1.ShareInfo {
	return &v1.ShareInfo{
		Uuid:        share.Uuid,
		ServerUuid:  share.Server,
		Name:        share.Name,
		Path:        share.Path.String(),
		CreatedTs:   share.CreatedTs.Unix(),
		FollowLinks: share.FollowLinks,
	}
}
func (s *RpcServer) writeLogMsgPtr(rec clog.MessageRecord, ptr *v1.LogMessage) {
	attrs := make([]*v1.LogMessageAttr, len(rec.Attrs))
	for i, attr := range rec.Attrs {
		attrs[i] = &v1.LogMessageAttr{
			Kind:  attr.Kind,
			Key:   attr.Key,
			Value: attr.Value,
		}
	}

	ptr.Uid = rec.Uuid
	ptr.CreatedTs = rec.CreatedTs.UnixMilli()
	ptr.Message = rec.Message
	ptr.Attrs = attrs
}

func (s *RpcServer) StreamLogs(ctx context.Context, request *v1.StreamLogsRequest, conn *connect.ServerStream[v1.StreamLogsResponse]) error {
	sendMany := func(recs []clog.MessageRecord) error {
		msgs := make([]v1.LogMessage, len(recs))
		ptrs := make([]*v1.LogMessage, len(recs))
		for i, rec := range recs {
			ptr := &msgs[i]
			s.writeLogMsgPtr(rec, ptr)
			ptrs[i] = ptr
		}

		return conn.Send(&v1.StreamLogsResponse{
			Logs: ptrs,
		})
	}
	sendOne := func(rec clog.MessageRecord) error {
		ptr := &v1.LogMessage{}
		s.writeLogMsgPtr(rec, ptr)

		return conn.Send(&v1.StreamLogsResponse{
			Logs: []*v1.LogMessage{ptr},
		})
	}

	pending := make(chan clog.MessageRecord, 100)

	sub := s.clogHandler.Subscribe(func(rec clog.MessageRecord) {
		pending <- rec
	})
	defer s.clogHandler.Unsubscribe(sub)

	// If old logs were requested, send them first.
	if request.SendLogsAfterTs != nil {
		ts := time.UnixMilli(*request.SendLogsAfterTs)
		recs, err := s.clogHandler.GetLogsAfter(ts, slog.LevelDebug)
		if err != nil {
			return err
		}

		if err = sendMany(recs); err != nil {
			return err
		}
	}

	// Send new logs from subscription.
	for {
		select {
		case <-ctx.Done():
			return nil
		case rec := <-pending:
			if err := sendOne(rec); err != nil {
				return err
			}
		}
	}
}

func (s *RpcServer) StreamEvents(ctx context.Context, _ *v1.StreamEventsRequest, conn *connect.ServerStream[v1.StreamEventsResponse]) error {
	pending := make(chan *v1.StreamEventsResponse, 100)

	sub := s.eventBus.Subscribe(func(evt *v1.Event, ctx *v1.EventContext) {
		pending <- &v1.StreamEventsResponse{
			Event:   evt,
			Context: ctx,
		}
	})
	defer s.eventBus.Unsubscribe(sub)

	// Stream new events as they come in.
	for {
		select {
		case <-ctx.Done():
			return nil
		case res := <-pending:
			err := conn.Send(res)
			if err != nil {
				return err
			}
		}
	}
}

func (s *RpcServer) Stop(_ context.Context, _ *v1.StopRequest) (*v1.StopResponse, error) {
	s.stopper()

	return &v1.StopResponse{}, nil
}

func (s *RpcServer) GetClientInfo(_ context.Context, _ *v1.GetClientInfoRequest) (*v1.GetClientInfoResponse, error) {
	return &v1.GetClientInfoResponse{}, nil
}

func (s *RpcServer) GetServers(_ context.Context, _ *v1.GetServersRequest) (*v1.GetServersResponse, error) {
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
	roomName, roomOk := common.NormalizeRoomName(request.Room)
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
		roomName,
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

func (s *RpcServer) ConnectServer(_ context.Context, request *v1.ConnectServerRequest) (*v1.ConnectServerResponse, error) {
	srv, has := s.client.GetByUuid(request.Uuid)
	if !has {
		return nil, errServerNotFound
	}

	srv.Connect()

	return &v1.ConnectServerResponse{}, nil
}

func (s *RpcServer) DisconnectServer(_ context.Context, request *v1.DisconnectServerRequest) (*v1.DisconnectServerResponse, error) {
	srv, has := s.client.GetByUuid(request.Uuid)
	if !has {
		return nil, errServerNotFound
	}

	srv.Disconnect()

	return &v1.DisconnectServerResponse{}, nil
}

func (s *RpcServer) UpdateServer(ctx context.Context, request *v1.UpdateServerRequest) (*v1.UpdateServerResponse, error) {
	var roomName *common.NormalizedRoomName
	if request.Room != nil {
		n, roomOk := common.NormalizeRoomName(*request.Room)
		if !roomOk {
			return nil, errInvalidRoomName
		}
		roomName = &n
	}
	var username *common.NormalizedUsername
	if request.Username != nil {
		u, usernameOk := common.NormalizeUsername(*request.Username)
		if !usernameOk {
			return nil, errInvalidUsername
		}
		username = &u
	}

	srv, has := s.client.GetByUuid(request.Uuid)
	if !has {
		return nil, errServerNotFound
	}

	err := s.client.Update(ctx,
		request.Uuid,
		storage.UpdateServerFields{
			Name:     request.Name,
			Address:  request.Address,
			Room:     roomName,
			Username: username,
			Password: request.Password,
		},
	)
	if err != nil {
		return nil, err
	}

	srv, has = s.client.GetByUuid(request.Uuid)
	if !has {
		return nil, errServerNotFound
	}

	return &v1.UpdateServerResponse{
		Server: s.serverToInfo(srv),
	}, nil
}

func (s *RpcServer) GetShares(ctx context.Context, request *v1.GetSharesRequest) (*v1.GetSharesResponse, error) {
	_, has := s.client.GetByUuid(request.ServerUuid)
	if !has {
		return nil, errServerNotFound
	}

	records, err := s.client.storage.GetSharesByServer(ctx, request.ServerUuid)
	if err != nil {
		return nil, err
	}

	infos := make([]*v1.ShareInfo, len(records))
	for i, record := range records {
		infos[i] = s.shareRecToInfo(record)
	}

	return &v1.GetSharesResponse{
		Shares: infos,
	}, nil
}

func (s *RpcServer) CreateShare(ctx context.Context, request *v1.CreateShareRequest) (*v1.CreateShareResponse, error) {
	srv, has := s.client.GetByUuid(request.ServerUuid)
	if !has {
		return nil, errServerNotFound
	}

	_, err := srv.ShareMgr.Add(ctx, request.Name, request.Path, request.FollowLinks)
	if err != nil {
		if errors.Is(err, share.ErrInvalidShareName) {
			return nil, errInvalidShareName
		}

		return nil, err
	}

	record, has, err := s.client.storage.GetShareByServerUuidAndName(ctx, request.ServerUuid, request.Name)
	if err != nil {
		return nil, err
	}
	if !has {
		return nil, fmt.Errorf(`failed to get newly created share record with name %q and server UUID %q`, request.Name, request.ServerUuid)
	}

	info := s.shareRecToInfo(record)
	return &v1.CreateShareResponse{
		Share: info,
	}, nil
}

func (s *RpcServer) DeleteShare(ctx context.Context, request *v1.DeleteShareRequest) (*v1.DeleteShareResponse, error) {
	srv, has := s.client.GetByUuid(request.ServerUuid)
	if !has {
		return nil, errServerNotFound
	}

	_, has = srv.ShareMgr.GetByName(request.Name)
	if !has {
		return nil, errShareNotFound
	}

	err := srv.ShareMgr.Delete(ctx, request.Name)
	if err != nil {
		return nil, err
	}

	return &v1.DeleteShareResponse{}, nil
}

func (s *RpcServer) GetDirFiles(ctx context.Context, request *v1.GetDirFilesRequest, res *connect.ServerStream[v1.GetDirFilesResponse]) error {
	username, usernameOk := common.NormalizeUsername(request.Username)
	if !usernameOk {
		return errInvalidUsername
	}

	path, pathErr := common.ValidatePath(request.Path)
	if pathErr != nil {
		return connect.NewError(connect.CodeInvalidArgument, pathErr)
	}

	srv, has := s.client.GetByUuid(request.ServerUuid)
	if !has {
		return errServerNotFound
	}

	return srv.Do(ctx, func(ctx context.Context, c *room.Conn) error {
		peer := c.GetVirtualC2cConn(username, false)
		stream, err := peer.GetDirFiles(path)
		if err != nil {
			return err
		}
		defer func() {
			_ = stream.Close()
		}()

		for {
			var msg *pb.MsgDirFiles
			msg, err = stream.ReadNext()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}

				if protoMsgErr, ok := errors.AsType[protocol.ProtoMsgError](err); ok {
					if protoMsgErr.Msg.Type == pb.ErrType_ERR_TYPE_PATH_NOT_DIRECTORY {
						return errPathNotDir
					}
					if protoMsgErr.Msg.Type == pb.ErrType_ERR_TYPE_FILE_NOT_EXIST {
						return errFileNotFound
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

	path, pathErr := common.ValidatePath(request.Path)
	if pathErr != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, pathErr)
	}

	srv, has := s.client.GetByUuid(request.ServerUuid)
	if !has {
		return nil, errServerNotFound
	}

	return DoValue(srv.ConnNanny, ctx, func(ctx context.Context, c *room.Conn) (*v1.GetFileMetaResponse, error) {
		peer := c.GetVirtualC2cConn(username, false)
		meta, err := peer.GetFileMeta(path)
		if err != nil {
			if protoMsgErr, ok := errors.AsType[protocol.ProtoMsgError](err); ok {
				if protoMsgErr.Msg.Type == pb.ErrType_ERR_TYPE_FILE_NOT_EXIST {
					return nil, errFileNotFound
				}
			}

			return nil, err
		}

		return &v1.GetFileMetaResponse{
			Meta: s.metaToInfo(meta),
		}, nil
	})
}

func (s *RpcServer) GetOnlineUsers(ctx context.Context, request *v1.GetOnlineUsersRequest, res *connect.ServerStream[v1.GetOnlineUsersResponse]) error {
	srv, has := s.client.GetByUuid(request.ServerUuid)
	if !has {
		return errServerNotFound
	}

	return srv.Do(ctx, func(ctx context.Context, c *room.Conn) error {
		stream, err := c.GetOnlineUsers()
		if err != nil {
			return err
		}
		defer func() {
			_ = stream.Close()
		}()

		for {
			var msg *pb.MsgOnlineUsers
			msg, err = stream.ReadNext()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}

				return err
			}

			users := make([]*v1.OnlineUserInfo, len(msg.Users))
			for i, user := range msg.Users {
				users[i] = &v1.OnlineUserInfo{
					Username: user.Username,
				}
			}
			err = res.Send(&v1.GetOnlineUsersResponse{
				Users: users,
			})
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *RpcServer) ChangeAccountPassword(ctx context.Context, request *v1.ChangeAccountPasswordRequest) (*v1.ChangeAccountPasswordResponse, error) {
	srv, has := s.client.GetByUuid(request.ServerUuid)
	if !has {
		return nil, errServerNotFound
	}

	// Send password change request to server.
	err := srv.Do(ctx, func(ctx context.Context, c *room.Conn) error {
		err := c.ChangeAccountPassword(
			request.CurrentPassword,
			request.NewPassword,
		)
		if err != nil {
			if protoErr, ok := errors.AsType[protocol.ProtoMsgError](err); ok {
				errType := protoErr.Msg.Type
				if errType == pb.ErrType_ERR_TYPE_PERMISSION_DENIED {
					return errIncorrectPassword
				}
				if errType == pb.ErrType_ERR_TYPE_INVALID_FIELDS {
					return connect.NewError(connect.CodeInvalidArgument, errors.New(common.StrPtrOr(protoErr.Msg.Message, "password does not meet requirements")))
				}
			}

			return err
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Update password in database and memory.
	err = s.client.Update(ctx, srv.Uuid, storage.UpdateServerFields{
		Password: new(request.NewPassword),
	})
	if err != nil {
		return nil, fmt.Errorf(`failed to update server password in database: %w`, err)
	}

	return &v1.ChangeAccountPasswordResponse{}, nil
}

func (s *RpcServer) ServerConnect(_ context.Context, request *v1.ServerConnectRequest) (*v1.ServerConnectResponse, error) {
	srv, has := s.client.GetByUuid(request.Uuid)
	if !has {
		return nil, errServerNotFound
	}

	srv.Connect()

	return &v1.ServerConnectResponse{}, nil
}

func (s *RpcServer) ServerDisconnect(_ context.Context, request *v1.ServerDisconnectRequest) (*v1.ServerDisconnectResponse, error) {
	srv, has := s.client.GetByUuid(request.Uuid)
	if !has {
		return nil, errServerNotFound
	}

	srv.Disconnect()

	return &v1.ServerDisconnectResponse{}, nil
}

func (s *RpcServer) GetDirectSettings(ctx context.Context, _ *v1.GetDirectSettingsRequest) (*v1.GetDirectSettingsResponse, error) {
	cfg, err := direct.ConfigFromSettings(ctx, s.client.storage)
	if err != nil {
		return nil, err
	}

	return &v1.GetDirectSettingsResponse{
		Settings: &v1.DirectSettings{
			Disable:                    cfg.Disable,
			Addresses:                  cfg.Addresses,
			DefaultPort:                uint32(cfg.DefaultPort),
			DisableProbeIpsToAdvertise: cfg.DisableProbeIpsToAdvertise,
			AdvertisePrivateIps:        cfg.AdvertisePrivateIps,
			DisablePublicIpDiscovery:   cfg.DisablePublicIpDiscovery,
			DisableUpnp:                cfg.DisableUPnP,
			UpnpTimeoutMs:              uint32(cfg.UpnpTimeout / time.Millisecond),
		},
	}, nil
}

func (s *RpcServer) UpdateDirectSettings(ctx context.Context, request *v1.UpdateDirectSettingsRequest) (*v1.UpdateDirectSettingsResponse, error) {
	store := s.client.storage
	cfg := request.Settings

	// Validate addresses.
	for _, addr := range cfg.Addresses {
		_, err := netip.ParseAddrPort(addr)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid IP:PORT address format: %s", addr))
		}
	}

	// Validate default port.
	if cfg.DefaultPort != 0 {
		if cfg.DefaultPort > 65535 || cfg.DefaultPort < 1024 {
			return nil, errInvalidDefaultPort
		}
	}

	// Validate UPnP timeout.
	if cfg.UpnpTimeoutMs > 60_000 {
		return nil, errInvalidUpnpTimeout
	}

	if err := store.PutSettingBool(ctx, direct.SettingDisable, cfg.Disable); err != nil {
		return nil, err
	}
	if cfg.Disable {
		return &v1.UpdateDirectSettingsResponse{}, nil
	}

	addrsJson, err := json.Marshal(cfg.Addresses)
	if err != nil {
		return nil, err
	}
	if err = store.PutSetting(ctx, direct.SettingAddrs, string(addrsJson)); err != nil {
		return nil, err
	}

	if err = store.PutSettingInt(ctx, direct.SettingDefaultPort, int64(cfg.DefaultPort)); err != nil {
		return nil, err
	}

	if err = store.PutSettingBool(ctx, direct.SettingDisableProbeIpsToAdvertise, cfg.DisableProbeIpsToAdvertise); err != nil {
		return nil, err
	}

	if err = store.PutSettingBool(ctx, direct.SettingAdvertisePrivateIps, cfg.AdvertisePrivateIps); err != nil {
		return nil, err
	}

	if err = store.PutSettingBool(ctx, direct.SettingDisablePublicIpDiscovery, cfg.DisablePublicIpDiscovery); err != nil {
		return nil, err
	}

	if err = store.PutSettingInt(ctx, direct.SettingUpnpTimeoutMs, int64(cfg.UpnpTimeoutMs)); err != nil {
		return nil, err
	}

	if err = store.PutSettingBool(ctx, direct.SettingDisableUPnP, cfg.DisableUpnp); err != nil {
		return nil, err
	}

	if err = store.PutSettingInt(ctx, direct.SettingUpnpTimeoutMs, int64(cfg.UpnpTimeoutMs)); err != nil {
		return nil, err
	}

	return &v1.UpdateDirectSettingsResponse{}, nil
}

func (s *RpcServer) IndexShare(_ context.Context, request *v1.IndexShareRequest) (*v1.IndexShareResponse, error) {
	srv, has := s.client.GetByUuid(request.ServerUuid)
	if !has {
		return nil, errServerNotFound
	}

	_, has = srv.ShareMgr.GetByName(request.Name)
	if !has {
		return nil, errShareNotFound
	}

	err := srv.ShareMgr.ScheduleShareIndex(request.Name)
	if err != nil {
		if errors.Is(err, share.ErrIndexingDisabled) {
			return nil, errIndexingDisabled
		}

		return nil, err
	}

	return &v1.IndexShareResponse{}, nil
}

func (s *RpcServer) StreamSearch(ctx context.Context, request *v1.StreamSearchRequest, conn *connect.ServerStream[v1.StreamSearchResponse]) error {
	if request.Query == "" {
		return errEmptySearchQuery
	}

	srv, has := s.client.GetByUuid(request.ServerUuid)
	if !has {
		return errServerNotFound
	}

	return srv.Do(ctx, func(ctx context.Context, c *room.Conn) error {
		if request.Username == nil {
			// Stream from server.
			stream, err := c.Search(request.Query)
			if err != nil {
				return err
			}
			defer func() {
				_ = stream.Close()
			}()

			for {
				next, nextErr := stream.ReadNext()
				if nextErr != nil {
					if protocol.IsErrorConnCloseOrCancel(nextErr) {
						return nil
					}
				}

				err = conn.Send(&v1.StreamSearchResponse{
					Username:      next.Username,
					DirectoryPath: next.Result.DirectoryPath,
					File:          s.metaToInfo(next.Result.File),
					Snippet:       next.Result.Snippet,
				})
				if err != nil {
					if protocol.IsErrorConnCloseOrCancel(err) {
						return nil
					}
					return err
				}
			}
		} else {
			// Stream from client.
			username, usernameOk := common.NormalizeUsername(*request.Username)
			if !usernameOk {
				return errInvalidUsername
			}

			peer := c.GetVirtualC2cConn(username, false)

			stream, err := peer.Search(request.Query)
			if err != nil {
				return err
			}
			defer func() {
				_ = stream.Close()
			}()

			for {
				next, nextErr := stream.ReadNext()
				if nextErr != nil {
					if protocol.IsErrorConnCloseOrCancel(nextErr) {
						return nil
					}
				}

				err = conn.Send(&v1.StreamSearchResponse{
					Username:      peer.Username.String(),
					DirectoryPath: next.DirectoryPath,
					File:          s.metaToInfo(next.File),
					Snippet:       next.Snippet,
				})
				if err != nil {
					if protocol.IsErrorConnCloseOrCancel(err) {
						return nil
					}
					return err
				}
			}
		}
	})
}

func (s *RpcServer) updateToInfo(update *updater.UpdateInfo, updateErr error) *v1.UpdateInfo {
	var info *v1.UpdateInfo
	if updateErr != nil {
		info = &v1.UpdateInfo{
			IsValid: false,
		}
	} else if update != nil {
		info = &v1.UpdateInfo{
			IsValid:     true,
			CreatedTs:   update.CreatedTs,
			Version:     update.Version,
			Description: update.Description,
			Url:         update.Url,
		}
	}

	return info
}

func (s *RpcServer) GetUpdateInfo(_ context.Context, _ *v1.GetUpdateInfoRequest) (*v1.GetUpdateInfoResponse, error) {
	return &v1.GetUpdateInfoResponse{
		CurrentInfo: s.updateToInfo(&s.updateChecker.CurrentUpdate, nil),
		NewInfo:     s.updateToInfo(s.updateChecker.GetNewUpdate()),
	}, nil
}

func (s *RpcServer) CheckForNewUpdate(_ context.Context, _ *v1.CheckForNewUpdateRequest) (*v1.CheckForNewUpdateResponse, error) {
	return &v1.CheckForNewUpdateResponse{
		NewInfo: s.updateToInfo(s.updateChecker.CheckNow()),
	}, nil
}

func (s *RpcServer) GetDownloadManagerItems(_ context.Context, _ *v1.GetDownloadManagerItemsRequest) (*v1.GetDownloadManagerItemsResponse, error) {
	return &v1.GetDownloadManagerItemsResponse{
		Items: s.downloadManager.SnapshotStates(),
	}, nil
}

func (s *RpcServer) QueueFileDownload(_ context.Context, request *v1.QueueFileDownloadRequest) (*v1.QueueFileDownloadResponse, error) {
	srv, has := s.client.GetByUuid(request.ServerUuid)
	if !has {
		return nil, errServerNotFound
	}
	username, usernameOk := common.NormalizeUsername(request.PeerUsername)
	if !usernameOk {
		return nil, errInvalidUsername
	}
	path, pathErr := common.ValidatePath(request.FilePath)
	if pathErr != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, pathErr)
	}

	err := s.downloadManager.Queue(
		srv,
		username,
		path,
	)
	if err != nil {
		return nil, err
	}

	return &v1.QueueFileDownloadResponse{}, nil
}

func (s *RpcServer) CancelFileDownload(ctx context.Context, request *v1.CancelFileDownloadRequest) (*v1.CancelFileDownloadResponse, error) {
	//TODO implement me
	panic("implement me")
}
