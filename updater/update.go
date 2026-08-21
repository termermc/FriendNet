package updater

// CurrentUpdate is the current update the program is running.
// If the current update fetched from an online source has a timestamp before this one, it must be ignored.
var CurrentUpdate = UpdateInfo{
	CreatedTs:   1786587091,
	Version:     "1.2.2",
	Description: "This release fixes a few bugs for the client and server. Updating is recommended.\n\nChanges:\n - Fixed direct connections not being removed when they disconnect\n - Fixed some annoying console errors related to [http-over-websocket](https://github.com/termermc/http-over-websocket)\n - Fixed server HTTP external auth rejecting correct `Content-Type` response header when it contained a charset field\n - Fixed server HTTP external auth doubling its request headers on every request\n - Fixed PDFs being previewed as raw text\n - Added a limit of 1MiB for previewing files as text",
	Url:         "https://github.com/termermc/FriendNet/releases/tag/v1.2.2",
}
