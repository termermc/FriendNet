package direct

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"sync"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

const invalidTokenReason = "invalid token"

// ErrManagerClosed is returned by Manager methods when the Manager is closed.
var ErrManagerClosed = errors.New("direct connection manager is closed")

// ErrPartitionExists is returned when trying to create a new partition with an ID that already exists.
var ErrPartitionExists = errors.New("partition with same ID exists")

// ErrPartitionClosed is returned when trying to interact with a closed partition.
var ErrPartitionClosed = errors.New("partition closed")

// Manager manages direct connection servers, discovery of IP addresses, and port forwarding.
type Manager struct {
	logger *slog.Logger

	mu       sync.RWMutex
	isClosed bool

	ctx       context.Context
	ctxCancel context.CancelFunc

	cfg          Config
	cfgAddrPorts map[netip.AddrPort]struct{}

	// All currently listening servers.
	servers map[netip.AddrPort]*Server

	// All active partitions.
	// Closing a partition removes it from this map.
	partitions map[string]*Partition
}

func NewManager(
	logger *slog.Logger,
	cfg Config,
) (*Manager, error) {
	addrPorts, err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		logger: logger,

		ctx:       ctx,
		ctxCancel: cancel,

		cfg:          cfg,
		cfgAddrPorts: addrPorts,
	}, nil
}

// Close closes the manager and all the servers it manages.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.isClosed {
		defer m.mu.Unlock()
		return nil
	}
	m.isClosed = true

	// TODO Collect servers

	defer m.mu.Unlock()

	// TODO Close listeners

	return nil
}

// IsDisabled returns true if the manager is disabled.
// Disabled does not mean closed, although a disabled manager
func (m *Manager) IsDisabled() bool {
	return m.cfg.Disable
}

func (m *Manager) getPartByMethodId(methodId string) (part *Partition, has bool) {
	colonIdx := strings.IndexRune(methodId, ':')
	if colonIdx == -1 {
		return nil, false
	}

	partId := methodId[:colonIdx]

	m.mu.RLock()
	part, has = m.partitions[partId]
	m.mu.RUnlock()

	return part, has
}

// CreatePartition creates a new partition with the specified ID.
// If a partition with the same ID already exists, returns ErrPartitionExists.
func (m *Manager) CreatePartition(id string) (*Partition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, has := m.partitions[id]
	if has {
		return nil, ErrPartitionExists
	}

	ctx, ctxCancel := context.WithCancel(m.ctx)
	partition := &Partition{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		id:       id,
		m:        m,
		connChan: make(chan *IncomingDirectConn),
	}
	m.partitions[id] = partition
	return partition, nil
}

// IncomingDirectConn is an incoming direct connection.
// Struct instances must not be used after calling any of the methods except for RemoteAddr.
// The method ID field of the handshake can be ignored because it was already used to determine the correct partition.
type IncomingDirectConn struct {
	conn protocol.ProtoConn

	// The handshake message received from the incoming connection.
	Handshake *pb.MsgDirectConnHandshake

	// The bidi where the handshake message was sent.
	// This should be closed after accepting or rejecting the connection.
	Bidi protocol.ProtoBidi
}

// RemoteAddr returns the remote address of the incoming connection.
func (i *IncomingDirectConn) RemoteAddr() net.Addr {
	return i.conn.RemoteAddr()
}

// SendResultAndClose sends the result of the handshake and closes the bidi and connection.
// Regardless of whether the method returns an error, the underlying connection will be closed.
func (i *IncomingDirectConn) SendResultAndClose(result pb.DirectConnHandshakeResult, closeMsg string) error {
	defer func() {
		_ = i.Bidi.Close()
		_ = i.conn.CloseWithReason(closeMsg)
	}()

	err := i.Bidi.Write(pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE_RESULT, &pb.MsgDirectConnHandshakeResult{
		Result: result,
	})
	if err != nil {
		return err
	}

	return nil
}

// InvalidToken rejects the incoming connection because the token was invalid.
// After sending the result, it closes the bidi and connection.
// Regardless of whether the method returns an error, the underlying connection will be closed.
func (i *IncomingDirectConn) InvalidToken() error {
	return i.SendResultAndClose(pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_TOKEN_INVALID, invalidTokenReason)
}

// InternalError rejects the incoming connection because of an internal error.
// After sending the result, it closes the bidi and connection.
// Regardless of whether the method returns an error, the underlying connection will be closed.
func (i *IncomingDirectConn) InternalError() error {
	return i.SendResultAndClose(pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_INTERNAL_ERROR, "internal error")
}

// KThxBye tells the incoming connection that the handshake was successful, but the connection will be closed anyway.
// After sending the result, it closes the bidi and connection.
// Regardless of whether the method returns an error, the underlying connection will be closed.
func (i *IncomingDirectConn) KThxBye() error {
	return i.SendResultAndClose(pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_KTHXBYE, "handshake succeeded, kthxbye")
}

// Approve approves the incoming connection and closes the bidi.
// It returns the connection to the caller, now in a fully authenticated state.
func (i *IncomingDirectConn) Approve() (conn protocol.ProtoConn, err error) {
	err = i.Bidi.Write(pb.MsgType_MSG_TYPE_DIRECT_CONN_HANDSHAKE_RESULT, &pb.MsgDirectConnHandshakeResult{
		Result: pb.DirectConnHandshakeResult_DIRECT_CONN_HANDSHAKE_RESULT_OK,
	})
	if err != nil {
		return nil, fmt.Errorf(`failed to send handshake OK result: %w`, err)
	}

	_ = i.Bidi.Close()

	return i.conn, nil
}

// Partition is an identifier for a listener on a shared direct connection server.
// It is used to differentiate which underlying client should handle incoming connections.
// A Partition receives incoming connections and is responsible for authenticating them and
// finishing their handshakes.
type Partition struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	id       string
	m        *Manager
	connChan chan *IncomingDirectConn
}

// Close closes the partition and stops listening for incoming connections.
// It should be called when the connection that owns the partition is closed.
func (p *Partition) Close() error {
	select {
	case <-p.ctx.Done():
		// Already closed, either by a previous call to Close or the Manager being closed.
		return nil
	default:
	}

	close(p.connChan)
	p.ctxCancel()

	p.m.mu.Lock()
	delete(p.m.partitions, p.id)
	p.m.mu.Unlock()

	return nil
}

// CreateMethodId returns a direct connection method ID using the specified ID string that also encodes the partition into it.
// Creating method IDs with this function is required for incoming connections to be routed to the correct partition.
func (p *Partition) CreateMethodId(id string) string {
	return p.id + ":" + id
}

func (p *Partition) sendConn(conn *IncomingDirectConn) {
	select {
	case <-p.ctx.Done():
		// Partition or manager is closed.
		_ = conn.InvalidToken()
		return
	case p.connChan <- conn:
	}
}

// AcceptConn waits for an incoming connection and returns it.
// Once a connection is received, it is no longer owned by the partition.
//
// It returns ErrPartitionClosed if the partition is closed.
func (p *Partition) AcceptConn() (*IncomingDirectConn, error) {
	select {
	case <-p.ctx.Done():
		return nil, ErrPartitionClosed
	case conn := <-p.connChan:
		return conn, nil
	}
}
