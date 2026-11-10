module friendnet.org/common

go 1.27.0

require (
	connectrpc.com/connect v1.20.0
	friendnet.org/protocol v0.0.0
	github.com/coder/websocket v1.8.15
	github.com/termermc/go-mcf-password v1.1.0
	github.com/termermc/http-over-websocket/hows-go v0.0.0-20260813014833-63a4accd83e6
	golang.org/x/net v0.60.0
)

require (
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace friendnet.org/protocol => ../protocol
