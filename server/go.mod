module friendnet.org/server

go 1.25.6

require (
	friendnet.org/protocol v0.0.0
	github.com/quic-go/quic-go v0.59.0
)

replace friendnet.org/protocol => ../protocol

require (
	golang.org/x/crypto v0.41.0 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
)
