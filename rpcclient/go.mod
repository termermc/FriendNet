module friendnet.org/rpcclient

go 1.27.0

require (
	connectrpc.com/connect v1.20.0
	friendnet.org/common v0.0.0
	friendnet.org/protocol v0.0.0
	github.com/chzyer/readline v1.5.1
)

require (
	github.com/coder/websocket v1.8.15 // indirect
	github.com/termermc/http-over-websocket/hows-go v0.0.0-20260808144918-d30ba17155d3 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

replace (
	friendnet.org/common => ../common
	friendnet.org/protocol => ../protocol
)
