# Configuring STUN

STUN (Session Traversal Utilities for NAT) is a protocol used to discover a computer's public IP address and port. It is
used for NAT hole punching, a technique supported by FriendNet to establish direct connections between peers, even when
they have firewalls and are behind NATs.

FriendNet servers (version 1.1.4 and later) have a built-in STUN server that runs on the same addresses that the server
is configured to listen on. By default, it will try to guess its own public listening IP and port to send to clients
who query the server for STUN servers to use. However, this guess can sometimes be incorrect, leaving clients without a
working STUN server to use for NAT hole punching.

It is recommended to manually configure the server's public listening IP and port as the STUN server address to send to
clients.

To configure the STUN server addresses to send to clients, edit `server.json` and add a `stun_servers` value:

```json
{
    "listen": [
        "0.0.0.0:20038"
    ],
    "stun_servers": [
        "my.friendnet.server:20038"
    ]
}
```

For example, if your FriendNet server has the public IP `1.2.3.4` and you were listening on `0.0.0.0:20038` (wildcard
address, port 20038), then your `server.json` should look something like this:

```json
{
    "listen": [
        "0.0.0.0:20038"
    ],
    "stun_servers": [
        "1.2.3.4:20038"
    ]
}
```

If you'd like to provide different STUN servers (like if your server only has an IPv6 address, but you want to enable
IPv4 STUN), you can provide 3rd party STUN servers in the `stun_servers` value:

```json
{
	"stun_servers": [
        "stun.l.google.com:19302",
        "stun.cloudflare.com:3478"
	]
}
```

If multiple STUN servers are provided, the client will try all of them simultaneously and use the response from the
first one that responds.
