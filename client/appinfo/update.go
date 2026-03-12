package appinfo

import "friendnet.org/common/updater"

// CurrentUpdate is the current update the client is running.
// If the current update fetched from an online source has a timestamp before this one, it must be ignored.
var CurrentUpdate = updater.UpdateInfo{
	CreatedTs:   1773298066,
	Version:     "1.0.2",
	Description: "This release simplifies the client and makes some small improvements with how RPC works.\n\nChanges:\n - Client web UI, RPC and file proxy all run under the same HTTP server now\n - Server RPC now supports HTTPS\n - File proxy URLs include an authentication token in the URL, making it safer for remote access setups\n - Fixed root CA install for some Firefox-based browsers under Linux",
	Url:         "https://github.com/termermc/FriendNet/releases/tag/v1.0.2",
}
