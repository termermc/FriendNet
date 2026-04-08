package i2pcompat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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

func mkI2pManager(connInactivityTimeout time.Duration) *I2pManager {
	mgr, err := NewI2pManager(logger, "127.0.0.1:7656", connInactivityTimeout)
	if err != nil {
		panic(err)
	}
	return mgr
}

type twoSides struct {
	a *I2pManager
	b *I2pManager
}

func (s *twoSides) Close() error {
	_ = s.a.Close()
	_ = s.b.Close()
	return nil
}

func mkSides(timeout time.Duration) twoSides {
	return twoSides{
		a: mkI2pManager(timeout),
		b: mkI2pManager(timeout),
	}
}

func TestPing(t *testing.T) {
	sides := mkSides(10 * time.Second)
	defer func() {
		_ = sides.Close()
	}()

	resChan := make(chan error, 1)

	go func() {
		toB, err := sides.a.Dial(context.Background(), sides.b.Addr().String())
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
		fromA, err := sides.b.Accept(t.Context())
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
	timeout := 10_000 * time.Millisecond
	sides := mkSides(timeout)
	defer func() {
		_ = sides.Close()
	}()

	resChan := make(chan error)

	go func() {
		toB, err := sides.a.Dial(context.Background(), sides.b.Addr().String())
		if err != nil {
			resChan <- fmt.Errorf(`A failed to dial B: %w`, err)
			return
		}

		defer func() {
			_ = toB.CloseWithReason("")
		}()

		time.Sleep(timeout / 2)

		_, err = protocol.SendAndReceiveExpect[*pb.MsgPong](toB, pb.MsgType_MSG_TYPE_PING, &pb.MsgPing{
			SentTs: time.Now().UnixNano(),
		}, pb.MsgType_MSG_TYPE_PONG)
		if err != nil {
			resChan <- fmt.Errorf(`A failed to receive pong from B: %w`, err)
			return
		}

		time.Sleep(time.Duration(float64(timeout) * 0.75))

		// Total elapsed time > timeout * 1.5
		// Since there was activity, the connection should not have been closed, even though it's more than the timeout.

		_, err = protocol.SendAndReceiveExpect[*pb.MsgPong](toB, pb.MsgType_MSG_TYPE_PING, &pb.MsgPing{
			SentTs: time.Now().UnixNano(),
		}, pb.MsgType_MSG_TYPE_PONG)
		if err != nil {
			resChan <- fmt.Errorf(`A failed to receive pong #2 from B: %w`, err)
			return
		}

		// Wait more than the timeout.
		time.Sleep(time.Duration(float64(timeout) * 1.5))

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
		fromA, err := sides.b.Accept(t.Context())
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
