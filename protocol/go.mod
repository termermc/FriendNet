module friendnet.org/protocol

go 1.26.5

require (
	connectrpc.com/connect v1.19.1
	friendnet.org/common v0.0.0
	github.com/quic-go/quic-go v0.61.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/coder/websocket v1.8.15 // indirect
	github.com/termermc/http-over-websocket/hows-go v0.0.0-20260808144918-d30ba17155d3 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
)

replace friendnet.org/common => ../common
