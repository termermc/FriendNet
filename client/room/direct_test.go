package room

import (
	"testing"
	"time"

	"friendnet.org/protocol"
)

type dummyConn struct {
	protocol.ProtoConn
}

func TestSortByRtt(t *testing.T) {
	conn1 := &dummyConn{}
	conn2 := &dummyConn{}
	conn3 := &dummyConn{}

	entries := []*DirectConnEntry{
		&DirectConnEntry{
			Conn: conn1,
			stats: PingStats{
				LastPing: time.Now().Add(-(ServerPingInterval + time.Second)),
				Rtt:      0,
			},
		},
		&DirectConnEntry{
			Conn: conn2,
			stats: PingStats{
				LastPing: time.Now(),
				Rtt:      0,
			},
		},
		&DirectConnEntry{
			Conn: conn3,
			stats: PingStats{
				LastPing: time.Now(),
				Rtt:      100 * time.Microsecond,
			},
		},
	}

	SortDirectConnEntriesByRtt(entries)

	if entries[0].Conn != conn2 {
		t.Fatalf("conn2 should have been first")
	}
	if entries[1].Conn != conn3 {
		t.Fatalf("conn3 should have been second")
	}
	if entries[2].Conn != conn1 {
		t.Fatalf("conn1 should have been third")
	}
}
