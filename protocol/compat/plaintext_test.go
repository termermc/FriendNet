package compat

import (
	"context"
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
	"github.com/quic-go/quic-go"
)

var logger = slog.New(
	slog.NewJSONHandler(
		os.Stderr,
		&slog.HandlerOptions{
			Level: slog.LevelDebug,
		},
	),
)

func tcpDialer(_ context.Context, addr string) (net.Conn, error) {
	return net.Dial("tcp", addr)
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

func mkSides(timeout time.Duration) twoSides {
	portA := rand.IntN(65535-1024) + 1024
	portB := rand.IntN(65535-1024) + 1024

	addrA := fmt.Sprintf("127.0.0.1:%d", portA)
	addrB := fmt.Sprintf("127.0.0.1:%d", portB)

	return twoSides{
		mgrA:  mkConnManager(addrA, timeout),
		mgrB:  mkConnManager(addrB, timeout),
		addrA: addrA,
		addrB: addrB,
	}
}

func TestPing(t *testing.T) {
	sides := mkSides(10 * time.Second)
	defer func() {
		_ = sides.Close()
	}()

	resChan := make(chan error, 1)

	go func() {
		toB, err := sides.mgrA.Dial(context.Background(), sides.addrB)
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

func TestTimeout(t *testing.T) {
	sides := mkSides(2 * time.Second)
	defer func() {
		_ = sides.Close()
	}()

	resChan := make(chan error)

	go func() {
		toB, err := sides.mgrA.Dial(context.Background(), sides.addrB)
		if err != nil {
			resChan <- fmt.Errorf(`A failed to dial B: %w`, err)
			return
		}

		defer func() {
			_ = toB.CloseWithReason("")
		}()

		time.Sleep(1 * time.Second)

		_, err = protocol.SendAndReceiveExpect[*pb.MsgPong](toB, pb.MsgType_MSG_TYPE_PING, &pb.MsgPing{
			SentTs: time.Now().UnixNano(),
		}, pb.MsgType_MSG_TYPE_PONG)
		if err != nil {
			resChan <- fmt.Errorf(`A failed to receive pong from B: %w`, err)
			return
		}

		time.Sleep(1500 * time.Millisecond)

		// Total elapsed time >2.5s
		// Since there was activity, the connection should not have been closed, even though it's more than the 2
		// second timeout.

		_, err = protocol.SendAndReceiveExpect[*pb.MsgPong](toB, pb.MsgType_MSG_TYPE_PING, &pb.MsgPing{
			SentTs: time.Now().UnixNano(),
		}, pb.MsgType_MSG_TYPE_PONG)
		if err != nil {
			resChan <- fmt.Errorf(`A failed to receive pong #2 from B: %w`, err)
			return
		}

		// Wait more than the timeout.
		time.Sleep(2500 * time.Millisecond)

		_, err = protocol.SendAndReceiveExpect[*pb.MsgPong](toB, pb.MsgType_MSG_TYPE_PING, &pb.MsgPing{
			SentTs: time.Now().UnixNano(),
		}, pb.MsgType_MSG_TYPE_PONG)
		if err != nil {
			if _, ok := errors.AsType[*quic.ApplicationError](err); !ok {
				resChan <- fmt.Errorf(`expected ApplicationError, got %T: %w`, err, err)
				return
			}

			resChan <- nil
			return
		}

		resChan <- errors.New("expected ApplicationError, got nil")
	}()
	go func() {
		fromA, err := sides.mgrB.Accept(t.Context())
		if err != nil {
			resChan <- fmt.Errorf(`B failed to accept conn from A: %w`, err)
			return
		}

		for {
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
		}
	}()

	if err := <-resChan; err != nil {
		t.Fatal(err)
	}
}
