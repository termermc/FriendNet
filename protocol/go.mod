module friendnet.org/protocol

go 1.26.0

require (
	connectrpc.com/connect v1.19.1
	friendnet.org/common v0.0.0
	github.com/quic-go/quic-go v0.59.0
	google.golang.org/protobuf v1.36.11
)

require (
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
)

replace friendnet.org/common => ../common
