module friendnet.org/client

go 1.27.0

require (
	connectrpc.com/connect v1.21.1
	friendnet.org/browser v0.0.0
	friendnet.org/common v0.0.0
	friendnet.org/mkcert v0.0.0
	friendnet.org/protocol v0.0.0
	friendnet.org/stun v0.0.0
	friendnet.org/updater v0.0.0
	friendnet.org/upnp v0.0.0
	friendnet.org/webui v0.0.0
	github.com/quic-go/quic-go v0.61.0
	golang.org/x/net v0.60.0
	golang.org/x/sys v0.50.0
	google.golang.org/protobuf v1.36.12
	modernc.org/sqlite v1.57.0
)

require (
	buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go v1.36.12-20260825204119-511051f7f437.1 // indirect
	buf.build/go/protovalidate v1.4.0 // indirect
	cel.dev/cel-go v0.32.0 // indirect
	cel.dev/expr v0.25.3 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/termermc/http-over-websocket/hows-go v0.0.0-20260813014833-63a4accd83e6 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/exp v0.0.0-20260820142414-ca536658362e // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260819154853-08b0e4226688 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260819154853-08b0e4226688 // indirect
	modernc.org/libc v1.75.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

replace (
	friendnet.org/browser => ../browser
	friendnet.org/common => ../common
	friendnet.org/mkcert => ../mkcert
	friendnet.org/protocol => ../protocol
	friendnet.org/stun => ../stun
	friendnet.org/updater => ../updater
	friendnet.org/upnp => ../upnp
	friendnet.org/webui => ../webui
)
