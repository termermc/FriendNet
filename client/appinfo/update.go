package appinfo

import "friendnet.org/common/updater"

// CurrentUpdate is the current update the client is running.
// If the current update fetched from an online source has a timestamp before this one, it must be ignored.
var CurrentUpdate = updater.UpdateInfo{
	CreatedTs:   1775266955,
	Version:     "1.1.1",
	Description: "This is a bug fix release.\n\nChanges:\n - Fixed downloads not re-queuing when a peer goes offline mid-download\n - Fixed the transfers page failing to render when an empty file download was queued\n - Web UI render errors are now logged and displayed\n\nA news section was also added to the FriendNet website. You can subscribe to its RSS feed if you would like to keep up with FriendNet development news.",
	Url:         "https://github.com/termermc/FriendNet/releases/tag/v1.1.1",
}
