package updater

// CurrentUpdate is the current update the program is running.
// If the current update fetched from an online source has a timestamp before this one, it must be ignored.
var CurrentUpdate = UpdateInfo{
	CreatedTs:   1775719750,
	Version:     "1.1.3",
	Description: "This release adds an admin UI to the server and fixes a client bug.\n\nChanges:\n - Added an optional admin web UI to manage servers\n - Fixed online user list not loading correctly when reconnecting to a server",
	Url:         "https://github.com/termermc/FriendNet/releases/tag/v1.1.3",
}
