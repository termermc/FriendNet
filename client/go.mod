module friendnet.org/client

go 1.26.5

require (
	connectrpc.com/connect v1.19.1
	friendnet.org/common v0.0.0
	friendnet.org/protocol v0.0.0
	friendnet.org/stun v0.0.0
	friendnet.org/updater v0.0.0
	friendnet.org/upnp v0.0.0
	friendnet.org/webui v0.0.0
	github.com/crgimenes/glaze v0.0.31
	github.com/google/uuid v1.6.0
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/quic-go/quic-go v0.61.0
	golang.org/x/net v0.56.0
	golang.org/x/sys v0.47.0
	google.golang.org/protobuf v1.36.11
	modernc.org/sqlite v1.46.1
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/ebitengine/purego v0.10.1 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/exp v0.0.0-20260218203240-3dfff04db8fa // indirect
	golang.org/x/text v0.40.0 // indirect
	modernc.org/libc v1.68.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace (
	friendnet.org/common => ../common
	friendnet.org/protocol => ../protocol
	friendnet.org/stun => ../stun
	friendnet.org/updater => ../updater
	friendnet.org/upnp => ../upnp
	friendnet.org/webui => ../webui
)
