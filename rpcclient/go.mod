module friendnet.org/rpcclient

go 1.25.7

require (
	connectrpc.com/connect v1.19.1
	friendnet.org/protocol v0.0.0
	github.com/chzyer/readline v1.5.1
)

require (
	golang.org/x/sys v0.41.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace friendnet.org/protocol => ../protocol
