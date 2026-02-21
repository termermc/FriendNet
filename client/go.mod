module friendnet.org/client

go 1.25.7

require (
	connectrpc.com/connect v1.19.1
	friendnet.org/common v0.0.0
	friendnet.org/protocol v0.0.0
	friendnet.org/webui v0.0.0
	github.com/google/uuid v1.6.0
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/puzpuzpuz/xsync/v4 v4.4.0
	github.com/quic-go/quic-go v0.59.0
	golang.org/x/net v0.50.0
	google.golang.org/protobuf v1.36.11
	modernc.org/sqlite v1.45.0
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/exp v0.0.0-20251023183803-a4bb9ffd2546 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	modernc.org/libc v1.67.6 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace (
	friendnet.org/common => ../common
	friendnet.org/protocol => ../protocol
	friendnet.org/webui => ../webui
)
