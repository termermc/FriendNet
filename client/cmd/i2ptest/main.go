package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"friendnet.org/common"
	"friendnet.org/protocol"
	pbv1 "friendnet.org/protocol/pb/v1"
	"github.com/go-i2p/go-sam-go"
	"github.com/go-i2p/go-sam-go/raw"
	"github.com/quic-go/quic-go"
)

func genCert() tls.Certificate {
	certRaw, err := common.GenSelfSignedPem("i2p")
	if err != nil {
		panic(err)
	}
	cert, err := tls.X509KeyPair(certRaw, certRaw)
	if err != nil {
		panic(err)
	}
	return cert
}

func newSamClient() (*sam3.SAM, error) {
	return sam3.NewSAM("127.0.0.1:7656")
}

func newSession(client *sam3.SAM) (*sam3.RawSession, error) {
	// Generate keys (optionally specify signature type)
	keys, err := client.NewKeys() // Uses default EdDSA_SHA512_Ed25519
	// Or: keys, err := client.NewKeys(sam3.Sig_ECDSA_SHA256_P256)
	if err != nil {
		return nil, err
	}

	sessId := "myDatagrams" + strconv.FormatInt(rand.Int64(), 10)

	return raw.NewRawSession(client.SAM, sessId, keys, sam3.Options_Small)
	//return client.NewRawSession(sessId, keys, sam3.Options_Small, 0)
}

func runServer(ctx context.Context, session *sam3.RawSession) error {
	protoListener, err := protocol.NewQuicProtoListenerFromConn(session.PacketConn(), &tls.Config{
		MinVersion:   tls.VersionTLS13,
		Certificates: []tls.Certificate{genCert()},
		NextProtos:   []string{protocol.DirectAlpnProtoName},
	})
	if err != nil {
		return fmt.Errorf(`failed to create proto listener: %w`, err)
	}

	println("Listening on " + session.Addr().String())

	for {
		conn, err := protoListener.Accept(ctx)
		if err != nil {
			return fmt.Errorf(`failed to accept proto conn: %w`, err)
		}
		println("Server got connection from " + conn.RemoteAddr().String())

		go func() {
			defer func() {
				_ = conn.CloseWithReason("")
			}()

			for {
				bidi, err := conn.WaitForBidi(ctx)
				if err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "WaitForBidi failed: %v\n", err)
					break
				}
				println("Server got bidi from " + conn.RemoteAddr().String())

				msg, err := bidi.ReadRaw()
				if err != nil {
					_, _ = fmt.Fprintf(os.Stderr, "ReadRaw failed: %v\n", err)
					break
				}

				fmt.Printf("Received message type: %s\n", msg.Type.String())

				if msg.Type == pbv1.MsgType_MSG_TYPE_PING {
					err = bidi.Write(pbv1.MsgType_MSG_TYPE_PONG, &pbv1.MsgPong{
						SentTs: time.Now().UnixMilli(),
					})
					if err != nil {
						_, _ = fmt.Fprintf(os.Stderr, "Write failed: %v\n", err)
						break
					}
				} else {
					err = bidi.WriteAck()
					if err != nil {
						_, _ = fmt.Fprintf(os.Stderr, "WriteAck failed: %v\n", err)
						break
					}
				}

				_ = bidi.Close()
			}
		}()
	}
}

type i2pAddr struct {
	id string
}

func (a i2pAddr) String() string {
	return a.id
}
func (a i2pAddr) Network() string {
	return "I2P"
}

func runClient(ctx context.Context, sess *sam3.RawSession, destId string) error {
	//println("Dialing " + destId + "...")
	//i2pConn, err := sess.DialContext(ctx, destId)
	//if err != nil {
	//	return fmt.Errorf(`failed to dial %s: %w`, destId, err)
	//}
	//defer func() {
	//	_ = i2pConn.Close()
	//}()

	tlsCfg := &tls.Config{
		MinVersion:         tls.VersionTLS13,
		NextProtos:         []string{protocol.AlpnProtoName},
		ServerName:         destId,
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(_ [][]byte, _ [][]*x509.Certificate) error {
			return nil
		},
	}

	println("Creating QUIC connection to " + destId + " via " + sess.Addr().String() + "...")
	qConn, err := quic.Dial(ctx, sess.PacketConn(), i2pAddr{destId}, tlsCfg, &quic.Config{
		KeepAlivePeriod:    protocol.DefaultKeepAlivePeriod,
		MaxIncomingStreams: protocol.DefaultMaxIncomingStreams,
	})
	if err != nil {
		return fmt.Errorf(`failed to dial QUIC %q: %w`, destId, err)
	}
	conn := protocol.ToProtoConn(qConn)
	defer func() {
		_ = conn.CloseWithReason("")
	}()

	println("Sending ping...")
	reply, err := conn.SendAndReceive(pbv1.MsgType_MSG_TYPE_PING, &pbv1.MsgPing{
		SentTs: time.Now().UnixMilli(),
	})
	if err != nil {
		return fmt.Errorf(`failed to send ping: %w`, err)
	}

	println("Ping reply:", reply.Type.String())

	return nil
}

func main() {
	// TODO Send datagrams directly to see if connectivity is even happening at all

	println("Creating SAM client...")
	client, err := newSamClient()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = client.Close()
	}()

	var serverSess *sam3.RawSession
	var serverSessErr error
	var clientSess *sam3.RawSession
	var clientSessErr error
	var sessWg sync.WaitGroup
	sessWg.Go(func() {
		println("Creating server session...")
		serverSess, serverSessErr = newSession(client)
	})
	sessWg.Go(func() {
		println("Creating client session...")
		clientSess, clientSessErr = newSession(client)
	})
	sessWg.Wait()
	if serverSessErr != nil {
		panic(serverSessErr)
	}
	if clientSessErr != nil {
		panic(clientSessErr)
	}
	defer func() {
		_ = serverSess.Close()
		_ = clientSess.Close()
	}()

	println("Server address: " + serverSess.Addr().String())
	println("Client address: " + clientSess.Addr().String())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	var wg sync.WaitGroup
	wg.Go(func() {
		if err := runServer(ctx, serverSess); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "runServer failed: %v\n", err)
			stop()
		}
	})
	wg.Go(func() {
		if err := runClient(ctx, clientSess, serverSess.Addr().String()); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "runClient failed: %v\n", err)
			stop()
		}
	})
	wg.Wait()

	println("Goodbye")
}
