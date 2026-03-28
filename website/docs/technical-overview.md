# Technical Overview

FriendNet is a self-hostable peer-to-peer file sharing system, similar to [Soulseek](https://www.slsknet.org/) or [Direct Connect](https://en.wikipedia.org/wiki/Direct_Connect_(protocol)).

It uses a hub server to coordinate peer discovery and as a fallback relay if peers cannot connect to each other directly.
The relay functionality provided by servers allows users who cannot open ports to participate, which sets FriendNet apart
from other peer-to-peer file sharing systems.

The protocol is implemented with [QUIC](https://en.wikipedia.org/wiki/QUIC) and [Protocol Buffers](https://protobuf.dev/).
You can read the protocol documentation [on GitHub](https://github.com/termermc/FriendNet/blob/master/protocol).

Servers are made up of [rooms](server/rooms.md), which function as logical hubs. Each room has its own accounts, forming
the basis of FriendNet's system.

The FriendNet client and server are written in Go, and each expose a [gRPC](https://grpc.io/) interface for controlling them.

The server's RPC interface can be accessed with a CLI or through any custom interface that understands its gRPC schema.

The client is implemented as a backend application, where a frontend controls it through its gRPC interface.
The default frontend is a web UI. The decision to use a web UI instead of using a native toolkit like [GTK](https://www.gtk.org/) was made for pragmatic reasons.
Many file sharing clients, such as [Transmission](https://transmissionbt.com/) provide a web UI to control a headless client
running on a home lab server or a seedbox, but others require separate clients for this functionality.

The file sharing system FriendNet was largely based on, Soulseek, has several clients, but most of them do not provide a web UI.
To run a Soulseek client headless on a server, you need a separate client, [slskd](https://github.com/slskd/slskd).
FriendNet uses a web UI by default, removing the need for a separate client.

While a native UI does not currently exist, creating one is possible using the client gRPC interface, without the need to build
a completely new implementation.
