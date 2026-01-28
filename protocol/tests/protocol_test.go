package tests

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
	"github.com/quic-go/quic-go"
)

type memCertStore struct {
	mu    sync.Mutex
	certs map[string][]byte
}

func (s *memCertStore) Get(host string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return append([]byte(nil), s.certs[host]...), nil
}

func (s *memCertStore) Put(host string, certDER []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.certs[host] = append([]byte(nil), certDER...)
	return nil
}

func newTLSConfig(t *testing.T) *tls.Config {
	t.Helper()

	pemBytes, err := genSelfSignedPem("friendnet-test")
	if err != nil {
		t.Fatalf("failed to generate self-signed cert: %v", err)
	}

	keyPair, err := tls.X509KeyPair(pemBytes, pemBytes)
	if err != nil {
		t.Fatalf("failed to parse key pair: %v", err)
	}

	return &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{keyPair},
		NextProtos:   []string{protocol.AlpnProtoName},
	}
}

func genSelfSignedPem(commonName string) ([]byte, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	notBefore := time.Now().Add(-1 * time.Minute)
	notAfter := notBefore.Add(365 * 24 * time.Hour)

	tpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: commonName,
		},
		NotBefore: notBefore,
		NotAfter:  notAfter,
		KeyUsage:  x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	derCert, err := x509.CreateCertificate(rand.Reader, tpl, tpl, priv.Public(), priv)
	if err != nil {
		return nil, err
	}

	pkcs8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, err
	}

	pemCert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derCert})
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})

	return append(pemCert, pemKey...), nil
}

func newTestListener(t *testing.T) (*quic.Listener, *net.UDPConn, string) {
	t.Helper()

	udpConn, err := net.ListenUDP("udp4", &net.UDPAddr{
		IP:   net.IPv4(127, 0, 0, 1),
		Port: 0,
	})
	if err != nil {
		t.Skipf("skipping: failed to listen udp: %v", err)
	}

	transport := quic.Transport{Conn: udpConn}
	listener, err := transport.Listen(newTLSConfig(t), &quic.Config{})
	if err != nil {
		_ = udpConn.Close()
		t.Fatalf("failed to listen quic: %v", err)
	}

	return listener, udpConn, listener.Addr().String()
}

func newAcceptedPair(t *testing.T) (*protocol.ProtoServerClient, *protocol.ProtoClient, func()) {
	t.Helper()

	listener, udpConn, addr := newTestListener(t)
	server := protocol.NewProtoServer(listener)
	server.VersionHandler = func(_ context.Context, _ *protocol.ProtoServerClient, _ *pb.ProtoVersion) (*pb.ProtoVersion, *pb.MsgVersionRejected, error) {
		return protocol.CurrentProtocolVersion, nil, nil
	}
	server.AuthHandler = func(_ context.Context, _ *protocol.ProtoServerClient, _ *pb.MsgAuthenticate) (*pb.MsgAuthAccepted, *pb.MsgAuthRejected, error) {
		return &pb.MsgAuthAccepted{}, nil, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	serverClientCh := make(chan *protocol.ProtoServerClient, 1)
	serverErrCh := make(chan error, 1)

	go func() {
		client, err := server.Accept(ctx)
		if err != nil {
			serverErrCh <- err
			return
		}
		serverClientCh <- client
	}()

	client, err := protocol.NewClient(addr, protocol.ClientCredentials{
		Room:     "room",
		Username: "user",
		Password: "pass",
	}, &memCertStore{certs: map[string][]byte{}})
	if err != nil {
		serverErr := ""
		select {
		case acceptErr := <-serverErrCh:
			serverErr = acceptErr.Error()
		case <-time.After(1 * time.Second):
		}
		cancel()
		_ = listener.Close()
		_ = udpConn.Close()
		if serverErr != "" {
			t.Fatalf("failed to create client: %v (server error: %s)", err, serverErr)
		}
		t.Fatalf("failed to create client: %v", err)
	}

	var serverClient *protocol.ProtoServerClient
	select {
	case serverClient = <-serverClientCh:
	case err := <-serverErrCh:
		cancel()
		_ = client.Close()
		_ = listener.Close()
		_ = udpConn.Close()
		t.Fatalf("failed to accept client: %v", err)
	case <-ctx.Done():
		cancel()
		_ = client.Close()
		_ = listener.Close()
		_ = udpConn.Close()
		t.Fatalf("timeout waiting for accept")
	}

	cleanup := func() {
		cancel()
		_ = client.Close()
		_ = serverClient.Close()
		_ = listener.Close()
		_ = udpConn.Close()
	}

	return serverClient, client, cleanup
}

func TestClientRequestsServer(t *testing.T) {
	serverClient, client, cleanup := newAcceptedPair(t)
	defer cleanup()

	content := []byte("file-bytes")
	serverClient.OnPing = func(_ context.Context, _ *protocol.ProtoServerClient, bidi protocol.ProtoBidi, msg *pb.MsgPing) error {
		return bidi.Write(pb.MsgType_MSG_TYPE_PONG, &pb.MsgPong{SentTs: msg.SentTs})
	}
	serverClient.OnGetDirFiles = func(_ context.Context, _ *protocol.ProtoServerClient, bidi protocol.ProtoBidi, msg *pb.MsgGetDirFiles) error {
		if msg.Path != "/dir" {
			return errors.New("unexpected dir path")
		}
		return bidi.Write(pb.MsgType_MSG_TYPE_DIR_FILES, &pb.MsgDirFiles{
			Filenames: []string{"a.txt", "b.txt"},
		})
	}
	serverClient.OnGetFileMeta = func(_ context.Context, _ *protocol.ProtoServerClient, bidi protocol.ProtoBidi, msg *pb.MsgGetFileMeta) error {
		if msg.Path != "/file.bin" {
			return errors.New("unexpected file path")
		}
		return bidi.Write(pb.MsgType_MSG_TYPE_FILE_META, &pb.MsgFileMeta{
			Size: uint64(len(content)),
		})
	}
	serverClient.OnGetFile = func(_ context.Context, _ *protocol.ProtoServerClient, bidi protocol.ProtoBidi, msg *pb.MsgGetFile) error {
		if msg.Path != "/file.bin" {
			return errors.New("unexpected file path")
		}
		if err := bidi.Write(pb.MsgType_MSG_TYPE_FILE_META, &pb.MsgFileMeta{
			Size: uint64(len(content)),
		}); err != nil {
			return err
		}
		_, err := bidi.Stream.Write(content)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	listenErrCh := make(chan error, 1)
	handlerErrCh := make(chan error, 1)
	go func() {
		listenErrCh <- serverClient.Listen(ctx, func(err error) {
			select {
			case handlerErrCh <- err:
			default:
			}
		})
	}()

	if _, err := client.Ping(); err != nil {
		t.Fatalf("ping failed: %v", err)
	}

	filenames, err := client.GetDirFiles("server", "/dir")
	if err != nil {
		t.Fatalf("get dir files failed: %v", err)
	}
	if len(filenames) != 2 || filenames[0] != "a.txt" || filenames[1] != "b.txt" {
		t.Fatalf("unexpected filenames: %v", filenames)
	}

	meta, err := client.GetFileMeta("server", "/file.bin")
	if err != nil {
		t.Fatalf("get file meta failed: %v", err)
	}
	if meta.Size != uint64(len(content)) {
		t.Fatalf("unexpected file size: %d", meta.Size)
	}

	fileMeta, reader, err := client.GetFile("server", "/file.bin", 0, 0)
	if err != nil {
		t.Fatalf("get file failed: %v", err)
	}
	if fileMeta.Size != uint64(len(content)) {
		t.Fatalf("unexpected file size: %d", fileMeta.Size)
	}

	readBytes, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close file stream failed: %v", err)
	}
	if string(readBytes) != string(content) {
		t.Fatalf("unexpected file content: %q", string(readBytes))
	}

	select {
	case err := <-handlerErrCh:
		t.Fatalf("handler error: %v", err)
	default:
	}

	cancel()
	select {
	case err := <-listenErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("listen error: %v", err)
		}
	default:
	}
}

func TestServerRequestsClient(t *testing.T) {
	serverClient, client, cleanup := newAcceptedPair(t)
	defer cleanup()

	content := []byte("client-file")
	client.OnPing = func(_ context.Context, _ *protocol.ProtoClient, bidi protocol.ProtoBidi, msg *pb.MsgPing) error {
		return bidi.Write(pb.MsgType_MSG_TYPE_PONG, &pb.MsgPong{SentTs: msg.SentTs})
	}
	client.OnGetDirFiles = func(_ context.Context, _ *protocol.ProtoClient, bidi protocol.ProtoBidi, msg *pb.MsgGetDirFiles) error {
		if msg.User != "client" || msg.Path != "/dir" {
			return errors.New("unexpected dir request")
		}
		return bidi.Write(pb.MsgType_MSG_TYPE_DIR_FILES, &pb.MsgDirFiles{
			Filenames: []string{"c.txt"},
		})
	}
	client.OnGetFileMeta = func(_ context.Context, _ *protocol.ProtoClient, bidi protocol.ProtoBidi, msg *pb.MsgGetFileMeta) error {
		if msg.User != "client" || msg.Path != "/file.bin" {
			return errors.New("unexpected file meta request")
		}
		return bidi.Write(pb.MsgType_MSG_TYPE_FILE_META, &pb.MsgFileMeta{
			Size: uint64(len(content)),
		})
	}
	client.OnGetFile = func(_ context.Context, _ *protocol.ProtoClient, bidi protocol.ProtoBidi, msg *pb.MsgGetFile) error {
		if msg.User != "client" || msg.Path != "/file.bin" {
			return errors.New("unexpected file request")
		}
		if err := bidi.Write(pb.MsgType_MSG_TYPE_FILE_META, &pb.MsgFileMeta{
			Size: uint64(len(content)),
		}); err != nil {
			return err
		}
		_, err := bidi.Stream.Write(content)
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	listenErrCh := make(chan error, 1)
	handlerErrCh := make(chan error, 1)
	go func() {
		listenErrCh <- client.Listen(ctx, func(err error) {
			select {
			case handlerErrCh <- err:
			default:
			}
		})
	}()

	if _, err := serverClient.Ping(); err != nil {
		t.Fatalf("ping failed: %v", err)
	}

	filenames, err := serverClient.GetDirFiles("client", "/dir")
	if err != nil {
		t.Fatalf("get dir files failed: %v", err)
	}
	if len(filenames) != 1 || filenames[0] != "c.txt" {
		t.Fatalf("unexpected filenames: %v", filenames)
	}

	meta, err := serverClient.GetFileMeta("client", "/file.bin")
	if err != nil {
		t.Fatalf("get file meta failed: %v", err)
	}
	if meta.Size != uint64(len(content)) {
		t.Fatalf("unexpected file size: %d", meta.Size)
	}

	fileMeta, reader, err := serverClient.GetFile("client", "/file.bin", 0, 0)
	if err != nil {
		t.Fatalf("get file failed: %v", err)
	}
	if fileMeta.Size != uint64(len(content)) {
		t.Fatalf("unexpected file size: %d", fileMeta.Size)
	}

	readBytes, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close file stream failed: %v", err)
	}
	if string(readBytes) != string(content) {
		t.Fatalf("unexpected file content: %q", string(readBytes))
	}

	select {
	case err := <-handlerErrCh:
		t.Fatalf("handler error: %v", err)
	default:
	}

	cancel()
	select {
	case err := <-listenErrCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("listen error: %v", err)
		}
	default:
	}
}
