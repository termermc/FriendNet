package appinfo

import "friendnet.org/common/updater"

// CurrentUpdate is the current update the client is running.
// If the current update fetched from an online source has a timestamp before this one, it must be ignored.
var CurrentUpdate = updater.UpdateInfo{
	CreatedTs:   1773198933,
	Version:     "1.0.1",
	Description: " - Improved client UI\n - Fixed issue with opening new browser windows for already-running clients\n - Added -rmcerthost client flag to remove hostnames from the certificate store (needed if a server changes certificate)\n - Added Linux ARM64 build",
	Url:         "https://github.com/termermc/FriendNet/releases/tag/v1.0.1",
}
