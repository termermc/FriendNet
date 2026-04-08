package updater

// CurrentUpdate is the current update the client is running.
// If the current update fetched from an online source has a timestamp before this one, it must be ignored.
var CurrentUpdate = UpdateInfo{
	CreatedTs:   1775580848,
	Version:     "1.1.2",
	Description: "This update improves the server management experience. If you do not run a server, you can skip this update.\n\nChanges:\n - Added built-in CLI to server (RPC client is no longer required to manage it)\n - Added bearer token support to RPC client",
	Url:         "https://github.com/termermc/FriendNet/releases/tag/v1.1.2",
}
