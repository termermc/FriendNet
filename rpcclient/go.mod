module friendnet.org/rpcclient

go 1.27.0

require (
	connectrpc.com/connect v1.21.1
	friendnet.org/common v0.0.0
	friendnet.org/protocol v0.0.0
	github.com/chzyer/readline v1.5.1
)

require (
	github.com/coder/websocket v1.8.15 // indirect
	github.com/termermc/http-over-websocket/hows-go v0.0.0-20260813014833-63a4accd83e6 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace (
	friendnet.org/common => ../common
	friendnet.org/protocol => ../protocol
)
