package appinfo

import "friendnet.org/common/updater"

// CurrentUpdate is the current update the client is running.
// If the current update fetched from an online source has a timestamp before this one, it must be ignored.
var CurrentUpdate = updater.UpdateInfo{
	CreatedTs:   1773027199,
	Version:     "1.0.0",
	Description: "Initial release of FriendNet.",
	Url:         "https://github.com/termermc/FriendNet/releases/tag/v1.0.0",
}
