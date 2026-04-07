package tcpstyle

import (
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"os"
	"testing"
	"time"

	"friendnet.org/protocol"
	pb "friendnet.org/protocol/pb/v1"
)

var logger = slog.New(
	slog.NewJSONHandler(
		os.Stderr,
		&slog.HandlerOptions{
			Level: slog.LevelDebug,
		},
	),
)

func tcpDialer(addr string) (net.Conn, error) {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			return nil, nil
		}

		return nil, err
	}

	return conn, nil
}

func mkConnManager(tcpAddr string, connInactivityTimeout time.Duration) *ConnManager {
	listener, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		panic(err)
	}

	return NewConnManager(logger, listener, tcpDialer, listener.Accept, connInactivityTimeout)
}

type twoSides struct {
	mgrA  *ConnManager
	mgrB  *ConnManager
	addrA string
	addrB string
}

func (s *twoSides) Close() error {
	_ = s.mgrA.Close()
	_ = s.mgrB.Close()
	return nil
}

func mkSides() twoSides {
	portA := rand.IntN(65535-1024) + 1024
	portB := rand.IntN(65535-1024) + 1024

	addrA := fmt.Sprintf("127.0.0.1:%d", portA)
	addrB := fmt.Sprintf("127.0.0.1:%d", portB)

	return twoSides{
		mgrA:  mkConnManager(addrA, 10*time.Second),
		mgrB:  mkConnManager(addrB, 10*time.Second),
		addrA: addrA,
		addrB: addrB,
	}
}

func TestConnManager_Accept(t *testing.T) {
	sides := mkSides()
	defer func() {
		_ = sides.Close()
	}()

	resChan := make(chan error, 1)

	go func() {
		toB, err := sides.mgrA.Dial(sides.addrB)
		if err != nil {
			resChan <- fmt.Errorf(`A failed to dial B: %w`, err)
			return
		}

		defer func() {
			_ = toB.CloseWithReason("")
		}()

		_, err = protocol.SendAndReceiveExpect[*pb.MsgPong](toB, pb.MsgType_MSG_TYPE_PING, &pb.MsgPing{
			SentTs: time.Now().UnixNano(),
		}, pb.MsgType_MSG_TYPE_PONG)
		if err != nil {
			resChan <- fmt.Errorf(`A failed to receive pong from B: %w`, err)
			return
		}

		resChan <- nil
	}()
	go func() {
		fromA, err := sides.mgrB.Accept(t.Context())
		if err != nil {
			resChan <- fmt.Errorf(`B failed to accept conn from A: %w`, err)
			return
		}

		bidi, err := fromA.WaitForBidi(t.Context())
		if err != nil {
			resChan <- fmt.Errorf(`B failed to wait for bidi from A: %w`, err)
			return
		}
		defer func() {
			_ = bidi.Close()
		}()

		_, err = protocol.ReadExpect[*pb.MsgPing](bidi.ProtoStreamReader, pb.MsgType_MSG_TYPE_PING)
		if err != nil {
			resChan <- fmt.Errorf(`B failed to read ping from A: %w`, err)
			return
		}

		err = bidi.Write(pb.MsgType_MSG_TYPE_PONG, &pb.MsgPong{
			SentTs: time.Now().UnixMilli(),
		})
		if err != nil {
			resChan <- fmt.Errorf(`B failed to send pong to A: %w`, err)
			return
		}

		resChan <- nil
	}()

	if err := <-resChan; err != nil {
		t.Fatal(err)
	}
	if err := <-resChan; err != nil {
		t.Fatal(err)
	}
}
