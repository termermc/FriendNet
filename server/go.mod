module friendnet.org/server

go 1.27.0

require (
	connectrpc.com/connect v1.20.0
	friendnet.org/adminui v0.0.0
	friendnet.org/ahocorasick v0.0.0
	friendnet.org/common v0.0.0
	friendnet.org/protocol v0.0.0
	friendnet.org/rpcclient v0.0.0
	friendnet.org/stun v0.0.0
	friendnet.org/updater v0.0.0
	github.com/anyascii/go v0.3.3
	github.com/quic-go/quic-go v0.61.0
	github.com/termermc/go-mcf-password v1.0.0
	golang.org/x/term v0.45.0
	google.golang.org/protobuf v1.36.12
	modernc.org/sqlite v1.57.0
)

require (
	github.com/chzyer/readline v1.5.1 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/termermc/http-over-websocket/hows-go v0.0.0-20260813014833-63a4accd83e6 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace (
	friendnet.org/adminui => ../adminui
	friendnet.org/ahocorasick => ../ahocorasick
	friendnet.org/common => ../common
	friendnet.org/protocol => ../protocol
	friendnet.org/rpcclient => ../rpcclient
	friendnet.org/stun => ../stun
	friendnet.org/updater => ../updater
)
