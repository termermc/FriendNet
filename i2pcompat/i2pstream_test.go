package i2pcompat

import (
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-i2p/go-sam-go"
	"github.com/go-i2p/go-sam-go/stream"
	"github.com/go-i2p/i2pkeys"
)

const wireMsg = "Hello from client!"

func newSamClient() (*sam3.SAM, error) {
	return sam3.NewSAM("127.0.0.1:7656")
}

func newSession(client *sam3.SAM) (*stream.StreamSession, error) {
	// Generate keys (optionally specify signature type)
	keys, err := client.NewKeys() // Uses default EdDSA_SHA512_Ed25519
	// Or: keys, err := client.NewKeys(sam3.Sig_ECDSA_SHA256_P256)
	if err != nil {
		return nil, err
	}

	sessId := "streams" + strconv.FormatInt(rand.Int64(), 10)

	return stream.NewStreamSession(client.SAM, sessId, keys, sam3.Options_Default)
}

func runServer(session *sam3.StreamSession) error {
	listener, err := session.Listen()
	if err != nil {
		return fmt.Errorf(`failed to listen: %w`, err)
	}
	defer func() {
		_ = listener.Close()
	}()

	// vvvv WORKING
	buf := make([]byte, 10)
	for {
		println("Awaiting connection...")
		conn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf(`failed to accept conn: %w`, err)
		}

		var sb strings.Builder
		for {
			n, err := conn.Read(buf)
			if err != nil {
				_ = conn.Close()

				if errors.Is(err, io.EOF) {
					break
				}

				return fmt.Errorf(`failed to read from conn: %w`, err)
			}
			if n == 0 {
				_ = conn.Close()
				return nil
			}

			sb.Write(buf[:n])
		}
		if sb.String() != wireMsg {
			_ = conn.Close()
			return fmt.Errorf(`unexpected message: %q`, sb.String())
		}

		fmt.Printf("Got valid message: %q\n", sb.String())

		_ = conn.Close()
	}
}

func runClient(sess *sam3.StreamSession, serverAddr i2pkeys.I2PAddr) error {
	ticker := time.NewTicker(time.Second)
	i := 0
	const iters = 10
	for range ticker.C {
		if i >= iters {
			return nil
		}
		i++

		conn, err := sess.DialI2P(serverAddr)
		if err != nil {
			return fmt.Errorf(`failed to open conn to %s: %w`, serverAddr.String(), err)
		}

		_, err = conn.Write([]byte(wireMsg))
		if err != nil {
			_ = conn.Close()
			return fmt.Errorf(`failed to write to %s: %w`, serverAddr.String(), err)
		}

		_ = conn.Close()
	}

	return nil
}

func TestBasicCommunication(t *testing.T) {
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

	finalErr := make(chan error)

	go func() {
		if err := runServer(serverSess); err != nil {
			finalErr <- fmt.Errorf("runServer failed: %w", err)
			return
		}
	}()
	go func() {
		if err := runClient(clientSess, serverSess.Addr()); err != nil {
			finalErr <- fmt.Errorf("runClient failed: %w", err)
			return
		}

		finalErr <- nil
	}()

	if err := <-finalErr; err != nil {
		t.Fatal(err)
	}
}
