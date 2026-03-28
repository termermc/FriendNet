package appinfo

import "friendnet.org/common/updater"

// CurrentUpdate is the current update the client is running.
// If the current update fetched from an online source has a timestamp before this one, it must be ignored.
var CurrentUpdate = updater.UpdateInfo{
	CreatedTs:   1774729978,
	Version:     "1.1.0",
	Description: "This release brings a complete download manager for resumable and bulk file/folder downloads.\n\nChanges:\n - Added a fully-featured download manager, supporting bulk file and folder downloads, resumable downloads, and concurrent downloads.\n - Fixed opening 127.0.0.1 instead of localhost in the browser, which caused certificate warnings\n - Documented client WebDAV integration (go check out the docs!)",
	Url:         "https://github.com/termermc/FriendNet/releases/tag/v1.1.0",
}
