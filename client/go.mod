module friendnet.org/client

go 1.26.0

require (
	connectrpc.com/connect v1.19.1
	friendnet.org/common v0.0.0
	friendnet.org/mkcert v0.0.0
	friendnet.org/protocol v0.0.0
	friendnet.org/upnp v0.0.0
	friendnet.org/webui v0.0.0
	github.com/go-i2p/onramp v0.33.93-0.20260203041150-e8b1b0661efc
	github.com/google/uuid v1.6.0
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c
	github.com/quic-go/quic-go v0.59.0
	golang.org/x/net v0.50.0
	golang.org/x/sys v0.41.0
	google.golang.org/protobuf v1.36.11
	modernc.org/sqlite v1.46.1
)

require (
	filippo.io/edwards25519 v1.1.0 // indirect
	github.com/armon/circbuf v0.0.0-20190214190532-5111143e8da2 // indirect
	github.com/beevik/ntp v1.5.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cretz/bine v0.2.0 // indirect
	github.com/dchest/siphash v1.2.3 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/eyedeekay/go-unzip v0.0.0-20240201194209-560d8225b50e // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-i2p/common v0.1.0 // indirect
	github.com/go-i2p/crypto v0.1.1-0.20251212210701-124dadb97cb7 // indirect
	github.com/go-i2p/elgamal v0.0.2 // indirect
	github.com/go-i2p/go-datagrams v0.0.0-20260127191042-ac9c62c3262d // indirect
	github.com/go-i2p/go-i2cp v0.1.1-0.20260124020217-dc6f6649a1df // indirect
	github.com/go-i2p/go-i2p v0.1.1-0.20251217014914-4558f6b6e173 // indirect
	github.com/go-i2p/go-noise v0.1.0 // indirect
	github.com/go-i2p/go-sam-bridge v0.0.0-20260127191723-8e14472fcc26 // indirect
	github.com/go-i2p/go-sam-go v0.33.0 // indirect
	github.com/go-i2p/go-streaming v0.0.0-20260127190938-144f6f599abb // indirect
	github.com/go-i2p/i2pkeys v0.33.92 // indirect
	github.com/go-i2p/logger v0.1.0 // indirect
	github.com/go-i2p/noise v0.0.0-20251212204422-ded862d8cdf9 // indirect
	github.com/go-i2p/su3 v0.0.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/oklog/ulid/v2 v2.1.1 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/sagikazarmark/locafero v0.12.0 // indirect
	github.com/samber/lo v1.52.0 // indirect
	github.com/samber/oops v1.21.0 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/spf13/viper v1.21.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	go.opentelemetry.io/otel v1.39.0 // indirect
	go.opentelemetry.io/otel/trace v1.39.0 // indirect
	go.step.sm/crypto v0.76.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.48.0 // indirect
	golang.org/x/exp v0.0.0-20260218203240-3dfff04db8fa // indirect
	golang.org/x/text v0.34.0 // indirect
	howett.net/plist v1.0.1 // indirect
	modernc.org/libc v1.68.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)

replace (
	friendnet.org/common => ../common
	friendnet.org/mkcert => ../mkcert
	friendnet.org/protocol => ../protocol
	friendnet.org/upnp => ../upnp
	friendnet.org/webui => ../webui
)
