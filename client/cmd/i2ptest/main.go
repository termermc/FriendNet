package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"math/rand/v2"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"friendnet.org/common"
	"github.com/go-i2p/go-sam-go"
	"github.com/go-i2p/go-sam-go/datagram"
	"github.com/go-i2p/i2pkeys"
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

func newSession(client *sam3.SAM) (*sam3.DatagramSession, error) {
	// Generate keys (optionally specify signature type)
	keys, err := client.NewKeys() // Uses default EdDSA_SHA512_Ed25519
	// Or: keys, err := client.NewKeys(sam3.Sig_ECDSA_SHA256_P256)
	if err != nil {
		return nil, err
	}

	sessId := "myDatagrams" + strconv.FormatInt(rand.Int64(), 10)

	return datagram.NewDatagramSession(client.SAM, sessId, keys, sam3.Options_Default)
}

type DatagramPacketConn struct {
	*sam3.DatagramSession
}

func runServer(ctx context.Context, session *sam3.DatagramSession) error {
	// vvvv WORKING
	buf := make([]byte, 10)
	for {
		println("Reading...")
		n, _, err := session.ReadFrom(buf)
		if err != nil {
			return fmt.Errorf(`failed to read from session: %w`, err)
		}

		println(string(buf[:n]))
	}

	//protoListener, err := protocol.NewQuicProtoListenerFromConn(session, &tls.Config{
	//	MinVersion:   tls.VersionTLS13,
	//	Certificates: []tls.Certificate{genCert()},
	//	NextProtos:   []string{protocol.DirectAlpnProtoName},
	//})
	//if err != nil {
	//	return fmt.Errorf(`failed to create proto listener: %w`, err)
	//}
	//
	//println("Listening on " + session.Addr().String())
	//
	//for {
	//	conn, err := protoListener.Accept(ctx)
	//	if err != nil {
	//		return fmt.Errorf(`failed to accept proto conn: %w`, err)
	//	}
	//	println("Server got connection from " + conn.RemoteAddr().String())
	//
	//	go func() {
	//		defer func() {
	//			_ = conn.CloseWithReason("")
	//		}()
	//
	//		for {
	//			bidi, err := conn.WaitForBidi(ctx)
	//			if err != nil {
	//				_, _ = fmt.Fprintf(os.Stderr, "WaitForBidi failed: %v\n", err)
	//				break
	//			}
	//			println("Server got bidi from " + conn.RemoteAddr().String())
	//
	//			msg, err := bidi.ReadRaw()
	//			if err != nil {
	//				_, _ = fmt.Fprintf(os.Stderr, "ReadRaw failed: %v\n", err)
	//				break
	//			}
	//
	//			fmt.Printf("Received message type: %s\n", msg.Type.String())
	//
	//			if msg.Type == pbv1.MsgType_MSG_TYPE_PING {
	//				err = bidi.Write(pbv1.MsgType_MSG_TYPE_PONG, &pbv1.MsgPong{
	//					SentTs: time.Now().UnixMilli(),
	//				})
	//				if err != nil {
	//					_, _ = fmt.Fprintf(os.Stderr, "Write failed: %v\n", err)
	//					break
	//				}
	//			} else {
	//				err = bidi.WriteAck()
	//				if err != nil {
	//					_, _ = fmt.Fprintf(os.Stderr, "WriteAck failed: %v\n", err)
	//					break
	//				}
	//			}
	//
	//			_ = bidi.Close()
	//		}
	//	}()
	//}
}

func runClient(ctx context.Context, sess *sam3.DatagramSession, serverAddr i2pkeys.I2PAddr) error {
	//println("Dialing " + destId + "...")
	//i2pConn, err := sess.DialContext(ctx, destId)
	//if err != nil {
	//	return fmt.Errorf(`failed to dial %s: %w`, destId, err)
	//}
	//defer func() {
	//	_ = i2pConn.Close()
	//}()

	// vvvvvv WORKING
	ticker := time.NewTicker(time.Second)
	for range ticker.C {
		_, err := sess.WriteTo([]byte("ilike cats"), serverAddr)
		//err := sess.SendDatagram([]byte("ilike cats"), serverAddr)
		if err != nil {
			return fmt.Errorf(`failed to send datagram to %s: %w`, serverAddr.String(), err)
		}
	}

	//tlsCfg := &tls.Config{
	//	MinVersion:         tls.VersionTLS13,
	//	NextProtos:         []string{protocol.AlpnProtoName},
	//	ServerName:         serverAddr.String(),
	//	InsecureSkipVerify: true,
	//	VerifyPeerCertificate: func(_ [][]byte, _ [][]*x509.Certificate) error {
	//		return nil
	//	},
	//}

	//pc, err := sess.Dial(serverAddr.String())
	//if err != nil {
	//	return fmt.Errorf(`failed to dial %s: %w`, serverAddr.String(), err)
	//}
	//defer func() {
	//	_ = pc.Close()
	//}()
	//toWrite := []byte("ilike cats")
	//ticker := time.NewTicker(time.Second)
	//defer ticker.Stop()
	//for range ticker.C {
	//	println("Writing...")
	//	_, err := pc.WriteTo(toWrite, serverAddr)
	//	if err != nil {
	//		return fmt.Errorf(`failed to write to %s: %w`, serverAddr.String(), err)
	//	}
	//}

	//println("Creating QUIC connection to " + serverAddr.String() + " via " + sess.Addr().String() + "...")
	//qConn, err := quic.Dial(ctx, sess, serverAddr, tlsCfg, &quic.Config{
	//	KeepAlivePeriod:    protocol.DefaultKeepAlivePeriod,
	//	MaxIncomingStreams: protocol.DefaultMaxIncomingStreams,
	//})
	//if err != nil {
	//	return fmt.Errorf(`failed to dial QUIC %q: %w`, serverAddr.String(), err)
	//}
	//conn := protocol.ToProtoConn(qConn)
	//defer func() {
	//	_ = conn.CloseWithReason("")
	//}()
	//
	//println("Sending ping...")
	//reply, err := conn.SendAndReceive(pbv1.MsgType_MSG_TYPE_PING, &pbv1.MsgPing{
	//	SentTs: time.Now().UnixMilli(),
	//})
	//if err != nil {
	//	return fmt.Errorf(`failed to send ping: %w`, err)
	//}
	//
	//println("Ping reply:", reply.Type.String())

	return nil
}

func main() {
	// TODO Send datagrams directly to see if connectivity is even happening at all

	println("Creating SAM clients...")
	serverSam, err := newSamClient()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = serverSam.Close()
	}()
	clientSam, err := newSamClient()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = clientSam.Close()
	}()

	println("Creating server session...")
	serverSess, serverSessErr := newSession(serverSam)
	if serverSessErr != nil {
		panic(serverSessErr)
	}
	clientSess, clientSessErr := newSession(clientSam)
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
		if err := runClient(ctx, clientSess, serverSess.Addr()); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "runClient failed: %v\n", err)
			stop()
		}
	})
	wg.Wait()

	println("Goodbye")
}
