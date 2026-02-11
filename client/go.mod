module friendnet.org/client

go 1.25.6

require (
	friendnet.org/protocol v0.0.0
	github.com/puzpuzpuz/xsync/v4 v4.4.0
	golang.org/x/net v0.49.0
)

require (
	github.com/quic-go/quic-go v0.59.0 // indirect
	golang.org/x/crypto v0.47.0 // indirect
	golang.org/x/sys v0.40.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace friendnet.org/protocol => ../protocol
