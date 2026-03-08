package appinfo

import (
	"time"

	"friendnet.org/common/updater"
)

// UpdatePubkeyEd25519 is the ed25519 public key used to verify update signatures.
var UpdatePubkeyEd25519 = "AAAAC3NzaC1lZDI1NTE5AAAAIHMM7QrqPS1wpH0T2w9XzsfiUCgTNqmgl0WfyFVmEr1S"

// CurrentUpdate is the current update the client is running.
// If the current update fetched from an online source has a timestamp before this one, it must be ignored.
var CurrentUpdate = updater.UpdateInfo{
	Ts:          time.Unix(1773011579, 0),
	Version:     "0.0.0",
	Description: "FriendNet alpha release.",
	Url:         "https://friendnet.org/download",
}
