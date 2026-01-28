package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

type CLI struct {
	in           *bufio.Reader
	out          io.Writer
	configPath   string
	statePath    string
	config       *ClientConfig
	state        *ClientState
	certStore    *JSONCertStore
	shares       *ShareManager
	webdavServer *WebDAVServer

	mu            sync.Mutex
	client        *protocol.ProtoClient
	sessionCancel context.CancelFunc
}

func NewCLI(in io.Reader, out io.Writer, configPath string, statePath string, cfg *ClientConfig, state *ClientState, certStore *JSONCertStore) *CLI {
	return &CLI{
		in:         bufio.NewReader(in),
		out:        out,
		configPath: configPath,
		statePath:  statePath,
		config:     cfg,
		state:      state,
		certStore:  certStore,
		shares:     NewShareManager(cfg),
	}
}

func (c *CLI) Run(ctx context.Context) error {
	c.printHelp()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := fmt.Fprint(c.out, "> "); err != nil {
			return err
		}

		line, err := c.in.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if err := c.handleCommand(ctx, line); err != nil {
			fmt.Fprintf(c.out, "error: %v\n", err)
		}
	}
}

func (c *CLI) handleCommand(ctx context.Context, line string) error {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return nil
	}

	switch strings.ToLower(fields[0]) {
	case "help", "?":
		c.printHelp()
		return nil
	case "quit", "exit":
		return c.disconnect()
	case "config":
		c.printConfig()
		return nil
	case "set":
		return c.setConfig(fields[1:])
	case "connect":
		return c.connect(ctx, fields[1:])
	case "share":
		return c.handleShare(fields[1:])
	case "webdav":
		return c.handleWebDAV(ctx, fields[1:])
	case "disconnect":
		return c.disconnect()
	case "ping":
		return c.ping()
	case "get-dir":
		return c.getDir(fields[1:])
	case "get-meta":
		return c.getMeta(fields[1:])
	case "get-file":
		return c.getFile(fields[1:])
	case "users":
		return c.getOnlineUsers()
	default:
		return fmt.Errorf("unknown command %q (try help)", fields[0])
	}
}

func (c *CLI) printHelp() {
	fmt.Fprintln(c.out, "Commands:")
	fmt.Fprintln(c.out, "  help                         Show this help")
	fmt.Fprintln(c.out, "  config                       Show current config")
	fmt.Fprintln(c.out, "  set <key> <value>             Set config key (server_addr, room, username, password)")
	fmt.Fprintln(c.out, "  connect [addr room user pass] Connect using args or config")
	fmt.Fprintln(c.out, "  share list                    List configured shares")
	fmt.Fprintln(c.out, "  share add <name> <path>        Add a share (name may be '-')")
	fmt.Fprintln(c.out, "  share remove <name>            Remove a share")
	fmt.Fprintln(c.out, "  webdav start                   Start WebDAV server")
	fmt.Fprintln(c.out, "  webdav stop                    Stop WebDAV server")
	fmt.Fprintln(c.out, "  disconnect                   Close the connection")
	fmt.Fprintln(c.out, "  ping                         Send a ping")
	fmt.Fprintln(c.out, "  get-dir <user> <path>         List files in a directory")
	fmt.Fprintln(c.out, "  get-meta <user> <path>        Fetch file metadata")
	fmt.Fprintln(c.out, "  get-file <user> <path> <out> [offset] [limit]  Download a file")
	fmt.Fprintln(c.out, "  users                        List online users in the room")
	fmt.Fprintln(c.out, "  quit                          Exit")
}

func (c *CLI) printConfig() {
	fmt.Fprintf(c.out, "server_addr=%s\n", c.config.ServerAddr)
	fmt.Fprintf(c.out, "room=%s\n", c.config.Room)
	fmt.Fprintf(c.out, "username=%s\n", c.config.Username)
	if c.config.Password != "" {
		fmt.Fprintln(c.out, "password=(set)")
	} else {
		fmt.Fprintln(c.out, "password=(empty)")
	}
}

func (c *CLI) setConfig(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: set <key> <value>")
	}

	key := strings.ToLower(args[0])
	value := strings.Join(args[1:], " ")

	switch key {
	case "server_addr":
		c.config.ServerAddr = value
	case "room":
		c.config.Room = value
	case "username":
		c.config.Username = value
	case "password":
		c.config.Password = value
	case "webdav_port":
		port, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid webdav_port: %w", err)
		}
		c.config.WebDAVPort = port
	default:
		return fmt.Errorf("unknown config key %q", key)
	}

	if err := SaveClientConfig(c.configPath, *c.config); err != nil {
		return err
	}
	c.shares = NewShareManager(c.config)
	return nil
}

func (c *CLI) handleShare(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: share <list|add|remove> ...")
	}
	switch strings.ToLower(args[0]) {
	case "list":
		if len(c.config.Shares) == 0 {
			fmt.Fprintln(c.out, "no shares configured")
			return nil
		}
		for _, share := range c.config.Shares {
			name := share.Name
			if name == "" {
				name = filepath.Base(share.Path)
			}
			fmt.Fprintf(c.out, "%s -> %s\n", name, share.Path)
		}
		return nil
	case "add":
		if len(args) != 3 {
			return fmt.Errorf("usage: share add <name> <path>")
		}
		name := args[1]
		if name == "-" {
			name = ""
		}
		c.config.Shares = append(c.config.Shares, Share{
			Name: name,
			Path: args[2],
		})
		if err := SaveClientConfig(c.configPath, *c.config); err != nil {
			return err
		}
		c.shares = NewShareManager(c.config)
		return nil
	case "remove":
		if len(args) != 2 {
			return fmt.Errorf("usage: share remove <name>")
		}
		name := strings.ToLower(args[1])
		updated := make([]Share, 0, len(c.config.Shares))
		for _, share := range c.config.Shares {
			shareName := share.Name
			if shareName == "" {
				shareName = filepath.Base(share.Path)
			}
			if strings.ToLower(shareName) == name {
				continue
			}
			updated = append(updated, share)
		}
		c.config.Shares = updated
		if err := SaveClientConfig(c.configPath, *c.config); err != nil {
			return err
		}
		c.shares = NewShareManager(c.config)
		return nil
	default:
		return fmt.Errorf("unknown share subcommand %q", args[0])
	}
}

func (c *CLI) handleWebDAV(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: webdav <start|stop>")
	}

	switch strings.ToLower(args[0]) {
	case "start":
		if c.webdavServer != nil {
			return fmt.Errorf("webdav already running")
		}
		port := c.config.WebDAVPort
		if port == 0 {
			return fmt.Errorf("webdav_port is not configured")
		}
		server := NewWebDAVServer(port, func() webdavClient {
			return c.getClient()
		})
		if err := server.Start(ctx); err != nil {
			return err
		}
		c.webdavServer = server
		fmt.Fprintf(c.out, "webdav listening on http://127.0.0.1:%d/\n", port)
		return nil
	case "stop":
		if c.webdavServer == nil {
			return fmt.Errorf("webdav is not running")
		}
		if err := c.webdavServer.Stop(ctx); err != nil {
			return err
		}
		c.webdavServer = nil
		fmt.Fprintln(c.out, "webdav stopped")
		return nil
	default:
		return fmt.Errorf("unknown webdav subcommand %q", args[0])
	}
}

func (c *CLI) connect(ctx context.Context, args []string) error {
	addr, room, username, password := c.config.ServerAddr, c.config.Room, c.config.Username, c.config.Password
	if len(args) > 0 {
		if len(args) != 4 {
			return fmt.Errorf("usage: connect [addr room user pass]")
		}
		addr, room, username, password = args[0], args[1], args[2], args[3]
	}

	if addr == "" || room == "" || username == "" {
		return fmt.Errorf("missing connection settings; use set or provide args")
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.client != nil {
		_ = c.disconnectLocked()
	}

	client, err := protocol.NewClient(addr, protocol.ClientCredentials{
		Room:     room,
		Username: username,
		Password: password,
	}, c.certStore)
	if err != nil {
		return err
	}

	client.OnPing = func(_ context.Context, _ *protocol.ProtoClient, bidi protocol.ProtoBidi, _ *pb.MsgPing) error {
		return bidi.Write(pb.MsgType_MSG_TYPE_PONG, &pb.MsgPong{
			SentTs: time.Now().UnixMilli(),
		})
	}
	client.OnGetDirFiles = c.handleDirFiles
	client.OnGetFileMeta = c.handleFileMeta
	client.OnGetFile = c.handleFile

	c.client = client
	sessionCtx, cancel := context.WithCancel(ctx)
	c.sessionCancel = cancel
	go func() {
		_ = client.Listen(sessionCtx, func(err error) {
			fmt.Fprintf(c.out, "listener error: %v\n", err)
		})
	}()
	go c.pingLoop(sessionCtx, client)

	c.config.ServerAddr = addr
	c.config.Room = room
	c.config.Username = username
	c.config.Password = password
	if err := SaveClientConfig(c.configPath, *c.config); err != nil {
		return err
	}
	c.shares = NewShareManager(c.config)

	c.state.LastAddr = addr
	c.state.LastRoom = room
	c.state.LastUsername = username
	c.state.LastSeenAt = time.Now().UTC().Format(time.RFC3339)
	if err := SaveClientState(c.statePath, *c.state); err != nil {
		return err
	}

	fmt.Fprintf(c.out, "connected to %s\n", addr)
	return nil
}

func (c *CLI) handleDirFiles(_ context.Context, _ *protocol.ProtoClient, bidi protocol.ProtoBidi, msg *pb.MsgGetDirFiles) error {
	dirFiles, err := c.shares.ListDir(msg.Path)
	if err != nil {
		return writeClientError(bidi, pb.ErrType_ERR_TYPE_FILE_NOT_EXIST, err)
	}
	return bidi.Write(pb.MsgType_MSG_TYPE_DIR_FILES, dirFiles)
}

func (c *CLI) handleFileMeta(_ context.Context, _ *protocol.ProtoClient, bidi protocol.ProtoBidi, msg *pb.MsgGetFileMeta) error {
	meta, file, err := c.shares.FileMeta(msg.Path)
	if err != nil {
		return writeClientError(bidi, pb.ErrType_ERR_TYPE_FILE_NOT_EXIST, err)
	}
	if file != nil {
		defer func() {
			_ = file.Close()
		}()
	}
	return bidi.Write(pb.MsgType_MSG_TYPE_FILE_META, meta)
}

func (c *CLI) handleFile(_ context.Context, _ *protocol.ProtoClient, bidi protocol.ProtoBidi, msg *pb.MsgGetFile) error {
	meta, file, err := c.shares.FileMeta(msg.Path)
	if err != nil {
		return writeClientError(bidi, pb.ErrType_ERR_TYPE_FILE_NOT_EXIST, err)
	}
	if meta.IsDir || file == nil {
		if file != nil {
			_ = file.Close()
		}
		return writeClientError(bidi, pb.ErrType_ERR_TYPE_INVALID_FIELDS, errors.New("path is a directory"))
	}
	defer func() {
		_ = file.Close()
	}()

	if err := bidi.Write(pb.MsgType_MSG_TYPE_FILE_META, meta); err != nil {
		return err
	}

	if msg.Offset > 0 {
		if _, err := file.Seek(int64(msg.Offset), io.SeekStart); err != nil {
			return err
		}
	}

	if msg.Limit > 0 {
		if _, err := io.CopyN(bidi.Stream, file, int64(msg.Limit)); err != nil && !errors.Is(err, io.EOF) {
			return err
		}
	} else {
		if _, err := io.Copy(bidi.Stream, file); err != nil {
			return err
		}
	}

	return nil
}

func (c *CLI) disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.disconnectLocked()
}

func (c *CLI) disconnectLocked() error {
	if c.webdavServer != nil {
		_ = c.webdavServer.Stop(context.Background())
		c.webdavServer = nil
	}
	if c.sessionCancel != nil {
		c.sessionCancel()
		c.sessionCancel = nil
	}
	if c.client != nil {
		err := c.client.Close()
		c.client = nil
		return err
	}
	return nil
}

func (c *CLI) ping() error {
	client := c.getClient()
	if client == nil {
		return fmt.Errorf("not connected")
	}
	pong, err := client.Ping()
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "pong at %d\n", pong.SentTs)
	return nil
}

func (c *CLI) getDir(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: get-dir <user> <path>")
	}
	client := c.getClient()
	if client == nil {
		return fmt.Errorf("not connected")
	}
	files, err := client.GetDirFiles(args[0], args[1])
	if err != nil {
		return err
	}
	basePath := strings.TrimSuffix(args[1], "/")
	if basePath == "" {
		basePath = "/"
	}
	for _, name := range files {
		fullPath := basePath
		if fullPath != "/" {
			fullPath += "/"
		}
		fullPath += name

		meta, err := client.GetFileMeta(args[0], fullPath)
		if err != nil {
			fmt.Fprintln(c.out, name)
			continue
		}
		if meta.IsDir {
			fmt.Fprintf(c.out, "%s/\n", name)
		} else {
			fmt.Fprintln(c.out, name)
		}
	}
	return nil
}

func (c *CLI) getMeta(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: get-meta <user> <path>")
	}
	client := c.getClient()
	if client == nil {
		return fmt.Errorf("not connected")
	}
	meta, err := client.GetFileMeta(args[0], args[1])
	if err != nil {
		return err
	}
	fmt.Fprintf(c.out, "size=%d\n", meta.Size)
	return nil
}

func (c *CLI) getFile(args []string) error {
	if len(args) < 3 || len(args) > 5 {
		return fmt.Errorf("usage: get-file <user> <path> <out> [offset] [limit]")
	}
	user, path, outPath := args[0], args[1], args[2]
	offset := uint64(0)
	limit := uint64(0)
	if len(args) >= 4 {
		value, err := strconv.ParseUint(args[3], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid offset: %w", err)
		}
		offset = value
	}
	if len(args) == 5 {
		value, err := strconv.ParseUint(args[4], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid limit: %w", err)
		}
		limit = value
	}

	client := c.getClient()
	if client == nil {
		return fmt.Errorf("not connected")
	}

	meta, stream, err := client.GetFile(user, path, offset, limit)
	if err != nil {
		return err
	}
	defer func() {
		_ = stream.Close()
	}()

	outFile, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = outFile.Close()
	}()

	var copied int64
	if limit > 0 {
		copied, err = io.CopyN(outFile, stream, int64(limit))
		if err != nil && err != io.EOF {
			return err
		}
	} else {
		copied, err = io.Copy(outFile, stream)
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(c.out, "wrote %d bytes (size=%d)\n", copied, meta.Size)
	return nil
}

func (c *CLI) getOnlineUsers() error {
	client := c.getClient()
	if client == nil {
		return fmt.Errorf("not connected")
	}
	users, err := client.GetOnlineUsers()
	if err != nil {
		return err
	}
	for _, user := range users {
		fmt.Fprintln(c.out, user)
	}
	return nil
}

func (c *CLI) getClient() *protocol.ProtoClient {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}

func (c *CLI) pingLoop(ctx context.Context, client *protocol.ProtoClient) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := client.Ping(); err != nil {
				fmt.Fprintf(c.out, "ping error: %v\n", err)
				return
			}
		}
	}
}

func writeClientError(bidi protocol.ProtoBidi, errType pb.ErrType, err error) error {
	message := err.Error()
	return bidi.Write(pb.MsgType_MSG_TYPE_ERROR, &pb.MsgError{
		Type:    errType,
		Message: &message,
	})
}
