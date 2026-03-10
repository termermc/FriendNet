module friendnet.org/common

go 1.26.0

require (
	connectrpc.com/connect v1.19.1
	friendnet.org/protocol v0.0.0
	github.com/termermc/go-mcf-password v1.0.0
	golang.org/x/net v0.50.0
)

require (
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace friendnet.org/protocol => ../protocol
