package direct

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"
	"unsafe"

	"friendnet.org/common"
	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"friendnet.org/upnp"
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

	cfg          *Config
	cfgAddrPorts map[netip.AddrPort]struct{}
	defaultPort  uint16

	// All currently listening servers.
	servers map[netip.AddrPort]*Server

	// All active partitions.
	// Closing a partition removes it from this map.
	partitions map[string]*Partition
}

func NewManager(
	logger *slog.Logger,
	cfg *Config,
) (*Manager, error) {
	addrPorts, err := cfg.Validate()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	defaultPort := cfg.DefaultPort
	if defaultPort == 0 {
		const minPort = 1024
		defaultPort = uint16(rand.IntN(65535-minPort) + minPort)
	}

	m := &Manager{
		logger: logger,

		ctx:       ctx,
		ctxCancel: cancel,

		cfg:          cfg,
		cfgAddrPorts: addrPorts,
		defaultPort:  defaultPort,

		servers:    make(map[netip.AddrPort]*Server),
		partitions: make(map[string]*Partition),
	}

	if !cfg.Disable {
		// Start servers in the background.
		// We don't want UPnP and slow listening operations to stall startup.
		go m.startServers()
	}

	return m, nil
}

func (m *Manager) lockAndRemoveServer(addrPort netip.AddrPort) {
	m.mu.Lock()
	server, has := m.servers[addrPort]
	if has {
		delete(m.servers, addrPort)
		for _, part := range m.partitions {
			go part.notifyServerClose(server)
		}
	}
	m.mu.Unlock()
}

func (m *Manager) startServers() {
	addrPorts := m.cfgAddrPorts

	defaultPort := m.defaultPort

	var publicIp netip.Addr

	if !m.cfg.DisableUPnP {
		upnp.SetLogger(m.logger)

		timeout := m.cfg.UpnpTimeout
		if timeout == 0 {
			timeout = 10 * time.Second
		}

		timeoutCtx, cancel := context.WithTimeout(m.ctx, timeout)

		const ipForwardDuration = 2 * time.Hour

		forwardIps := func(ctx context.Context, defaultPort uint16) upnp.ForwardResult {
			return upnp.ForwardUDPForPublicIPs(ctx, defaultPort, "FriendNet Client Direct", ipForwardDuration, 3*time.Second)
		}

		forwardOk := func() bool {
			result := forwardIps(timeoutCtx, defaultPort)
			errs := make([]error, len(result.Failures))
			for i, failure := range result.Failures {
				errs[i] = failure.Err
			}
			if len(result.Forwarded) == 0 {
				m.logger.Warn("UPnP public IP discovery and forwarding failed",
					"service", "direct.Manager",
					"err", errors.Join(errs...),
				)
				return false
			}

			// Prefer IPv4.
			for _, forwarded := range result.Forwarded {
				if forwarded.IP.To4() == nil {
					// Address is IPv6, ignore for now.
					continue
				}

				var ok bool
				publicIp, ok = netip.AddrFromSlice(forwarded.IP)
				if !ok {
					// Can't convert from net.IP to netip.Addr for some reason.
					continue
				}
			}
			// If there was no IPv4, use the first available address.
			if !publicIp.IsValid() {
				for _, forwarded := range result.Forwarded {
					publicIp, _ = netip.AddrFromSlice(forwarded.IP)
					if publicIp.IsValid() {
						break
					}
				}
			}

			if !publicIp.IsValid() {
				m.logger.Error("UPnP public IP discovery returned success, but no public IP could be found",
					"service", "direct.Manager",
				)
				return false
			}

			return true
		}()

		if forwardOk {
			// Periodically re-forward IPs with UPnP.
			go func() {
				ticker := time.NewTicker(ipForwardDuration - (10 * time.Minute))
				defer ticker.Stop()

				for {
					select {
					case <-m.ctx.Done():
						return
					case <-ticker.C:
						forwardIps(context.Background(), defaultPort)
					}
				}
			}()
		}

		cancel()
	}

	var probedIps []netip.Addr
	if !m.cfg.DisableProbeIpsToAdvertise {
		probedIps = common.GetUnicastIpsFromInterfaces(false, true)
	}

	// Collect addresses to listen on.
	listenAddrPorts := make([]netip.AddrPort, 0, 1+len(addrPorts)+len(probedIps))
	if publicIp.IsValid() {
		listenAddrPorts = append(listenAddrPorts, netip.AddrPortFrom(publicIp, defaultPort))
	}
	for addrPort := range addrPorts {
		if addrPort.Port() == 0 {
			listenAddrPorts = append(listenAddrPorts, netip.AddrPortFrom(addrPort.Addr(), defaultPort))
		} else {
			listenAddrPorts = append(listenAddrPorts, addrPort)
		}
	}
	for _, addr := range probedIps {
		addrPort := netip.AddrPortFrom(addr, defaultPort)
		listenAddrPorts = append(listenAddrPorts, addrPort)
	}

	// Create servers for each address.
	servers := make([]*Server, 0, len(listenAddrPorts))
	for _, addrPort := range listenAddrPorts {
		server, err := NewServer(m.logger, m.ctx, m, addrPort, m.cfg.Cert)
		if err != nil {
			m.logger.Error("failed to create direct server",
				"service", "direct.Manager",
				"addr", addrPort.String(),
				"err", err,
			)
			continue
		}

		servers = append(servers, server)
	}

	// Add them to map.
	m.mu.Lock()
	for _, server := range servers {
		m.servers[server.AddrPort] = server
		for _, part := range m.partitions {
			go part.notifyServerOpen(server)
		}
	}
	m.mu.Unlock()
}

// Close closes the manager and all the servers it manages.
func (m *Manager) Close() error {
	m.mu.Lock()
	if m.isClosed {
		m.mu.Unlock()
		return nil
	}
	m.isClosed = true
	m.mu.Unlock()

	// Canceling will automatically close servers and partitions.
	m.ctxCancel()

	return nil
}

// IsDisabled returns true if the manager is disabled.
// Disabled does not mean closed, although a disabled manager
func (m *Manager) IsDisabled() bool {
	return m.cfg.Disable
}

// IsPublicIpDiscoveryDisabled is whether clients should disable public IP discovery via the server.
// By default, clients will try to discover the machine's public IP by asking the server for it.
func (m *Manager) IsPublicIpDiscoveryDisabled() bool {
	return m.cfg.DisablePublicIpDiscovery
}

// AdvertisePrivateIps returns whether clients should advertise their private IPs to the server.
func (m *Manager) AdvertisePrivateIps() bool {
	return m.cfg.AdvertisePrivateIps
}

// NotifyIpAvailable notifies the Manager that an IP address is available for use.
// If there is not already a direct server running on that IP with the default port,
// a new one will be started for it in the background.
//
// This method can be used by connections after finding out their public IP from a server.
func (m *Manager) NotifyIpAvailable(ip netip.Addr) {
	if m.cfg.Disable {
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.isClosed {
		return
	}

	_, has := m.servers[netip.AddrPortFrom(ip, m.defaultPort)]
	if has {
		return
	}

	go func() {
		addrPort := netip.AddrPortFrom(ip, m.defaultPort)
		server, err := NewServer(m.logger, m.ctx, m, addrPort, m.cfg.Cert)
		if err != nil {
			m.logger.Error("failed to start direct server after IP notification",
				"service", "direct.Manager",
				"ip", ip.String(),
				"addr", addrPort.String(),
				"err", err,
			)
			return
		}

		m.mu.Lock()
		// Check again, just in case there are concurrent calls.
		_, has = m.servers[addrPort]
		if has {
			m.mu.Unlock()
			_ = server.Close()
			return
		}
		m.servers[server.AddrPort] = server
		for _, part := range m.partitions {
			go part.notifyServerOpen(server)
		}
		m.mu.Unlock()
	}()
}

// GetServers returns all currently running direct servers.
// If the manager is closed or disabled, returns empty.
// Note that this method creates a new slice each time it is called.
func (m *Manager) GetServers() []*Server {
	if m.cfg.Disable {
		return nil
	}

	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.isClosed {
		return nil
	}

	res := make([]*Server, 0, len(m.servers))
	for _, server := range m.servers {
		res = append(res, server)
	}
	return res
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

// CreatePartition creates a new partition, using a hash of name as the partition ID.
// If a partition with the same ID already exists, returns ErrPartitionExists.
func (m *Manager) CreatePartition(name string) (*Partition, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create ID from hash of IDs.
	hasher := fnv.New64a()
	_, _ = hasher.Write(unsafe.Slice(unsafe.StringData(name), len(name)))
	hash := base64.RawURLEncoding.EncodeToString(hasher.Sum(nil))

	_, has := m.partitions[hash]
	if has {
		return nil, ErrPartitionExists
	}

	ctx, ctxCancel := context.WithCancel(m.ctx)
	partition := &Partition{
		ctx:       ctx,
		ctxCancel: ctxCancel,

		id: hash,
		m:  m,

		connChan:        make(chan *IncomingDirectConn),
		serverOpenChan:  make(chan *Server),
		serverCloseChan: make(chan *Server),
	}
	m.partitions[hash] = partition
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

		go func() {
			// Give some time for the result message to be received.
			time.Sleep(500 * time.Millisecond)
			_ = i.conn.CloseWithReason(closeMsg)
		}()
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

	id string
	m  *Manager

	connChan        chan *IncomingDirectConn
	serverOpenChan  chan *Server
	serverCloseChan chan *Server
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

	// Cancel before closing channels to unblock waiting channel sends without a panic.
	p.ctxCancel()
	close(p.connChan)
	close(p.serverOpenChan)
	close(p.serverCloseChan)

	p.m.mu.Lock()
	delete(p.m.partitions, p.id)
	p.m.mu.Unlock()

	// If there is a conn that was not accepted, close it now.
	select {
	case conn := <-p.connChan:
		if conn == nil {
			break
		}
		_ = conn.InternalError()
	default:
		break
	}

	return nil
}

// CreateMethodId returns a direct connection method ID using the specified ID string that also encodes the partition into it.
// Creating method IDs with this function is required for incoming connections to be routed to the correct partition.
func (p *Partition) CreateMethodId(id string) string {
	return p.id + ":" + id
}

func (p *Partition) notifyServerOpen(server *Server) {
	select {
	case <-p.ctx.Done():
		return
	case p.serverOpenChan <- server:
	}
}
func (p *Partition) notifyServerClose(server *Server) {
	select {
	case <-p.ctx.Done():
		return
	case p.serverCloseChan <- server:
	}
}

// sendConn sends a new incoming direct connection to the partition.
// This method will block until the connection is received or the context is done.
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
// The returned *IncomingDirectConn is never nil.
func (p *Partition) AcceptConn() (*IncomingDirectConn, error) {
	select {
	case <-p.ctx.Done():
		return nil, ErrPartitionClosed
	case conn := <-p.connChan:
		return conn, nil
	}
}

// WaitServerOpen waits for a server to be open, and then returns it.
// If the partition is closed, it returns ErrPartitionClosed.
// The returned *Server is never nil.
func (p *Partition) WaitServerOpen() (*Server, error) {
	select {
	case <-p.ctx.Done():
		return nil, ErrPartitionClosed
	case server := <-p.serverOpenChan:
		return server, nil
	}
}

// WaitServerClose waits for a server to be closed, and then returns it.
// The returned server must not have any methods called on it.
// If the partition is closed, it returns ErrPartitionClosed.
// The returned *Server is never nil.
func (p *Partition) WaitServerClose() (*Server, error) {
	select {
	case <-p.ctx.Done():
		return nil, ErrPartitionClosed
	case server := <-p.serverCloseChan:
		return server, nil
	}
}
