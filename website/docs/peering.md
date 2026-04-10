# Peering

FriendNet operates as a peer-to-peer network, coordinated by a server, similar to how Bittorrent relies on trackers.

When a client wants to download a file from a peer, it first asks the server for the peer's addresses.
When the client gets the list, it tries to use them to connect directly. If any of the addresses work, a direct
connection is established. If none of the addresses work, the client will try and ask the peer to connect to it,
and the peer will go through the same process trying to connect to the client.

If neither peer can reach each other, they will fall back to relaying provided by the server.
This is how FriendNet continues to work when neither side of a download has a working connection method.

Clients use different techniques to make themselves available for connections, including UPnP. Currently, NAT traversal
is not implemented, but it is being worked on.
