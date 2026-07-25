package updater

// CurrentUpdate is the current update the program is running.
// If the current update fetched from an online source has a timestamp before this one, it must be ignored.
var CurrentUpdate = UpdateInfo{
	CreatedTs:   1784967355,
	Version:     "1.2.0",
	Description: "This release adds NAT hole punching for clients and improves the file browse view.\n\nBoth servers and clients need to update to take advantage of NAT hole punching. To learn more about it, visit https://friendnet.org/news/nat-hole-punching/.\nAs usual, servers and clients are backwards- and forwards-compatible, so all existing features will continue to work regardless of whether servers or clients update.\n\nChanges:\n - Added NAT hole punching for peer direct connections.\n - Added file size column to files table\n - Added pagination for large folders",
	Url:         "https://github.com/termermc/FriendNet/releases/tag/v1.2.0",
}
