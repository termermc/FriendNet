# copyparty

[copyparty](https://github.com/9001/copyparty) is an extremely feature-rich file upload and sharing server.
Like FriendNet, it has multi-user support and allows downloading files. Unlike FriendNet, users upload and download
files to and from the central server rather than sharing files peer-to-peer.

While copyparty is like a feature-packed and compatible Google Drive, FriendNet is a peer-to-peer sharing protocol with
servers that only coordinate peer discovery and relay.

| *                           | FriendNet    | copyparty               |
|-----------------------------|--------------|-------------------------|
| Who Stores Files?           | Users        | Server                  |
| Sharing Model               | Peer-to-peer | Centralized             |
| Self-Hosted?                | Yes          | Yes                     |
| Users Need to Port Forward? | No           | No                      |
| Supported Platforms         | Desktop OSes | Anything with a browser |

Should you use copyparty over FriendNet? Depends on your use-case.

If you only want to share your own files, or you want to allow people to upload to
a server directly, copyparty is a better choice.

If you and your friends want to share large collections of files with each other, FriendNet is a better choice. You and
your friends do not need to upload any files to share them, as they are downloaded directly from each other.
