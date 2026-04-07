module friendnet.org/i2pcompat

go 1.26.1

require (
	friendnet.org/protocol v0.0.0
	github.com/go-i2p/go-sam-go v0.33.0
	github.com/go-i2p/i2pkeys v0.33.92
	github.com/quic-go/quic-go v0.59.0
)

require (
	connectrpc.com/connect v1.19.1 // indirect
	friendnet.org/common v0.0.0 // indirect
	github.com/go-i2p/common v0.0.1 // indirect
	github.com/go-i2p/crypto v0.0.1 // indirect
	github.com/go-i2p/logger v0.0.1 // indirect
	github.com/oklog/ulid/v2 v2.1.1 // indirect
	github.com/samber/lo v1.52.0 // indirect
	github.com/samber/oops v1.19.3 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/net v0.50.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace (
	friendnet.org/common => ../common
	friendnet.org/protocol => ../protocol
	github.com/go-i2p/go-sam-go => ../../go-sam-go-fork
)
