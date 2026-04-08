# Configuration

If you ran your server at least once, you should see the following files created in the current working directory:

```
server.json
server.pem
server.db
```

The `server.json` file contains the server configuration that you can edit.

The default server configuration can be used without modification if you want to run on the default port `20038` and
not customize anything.

We will go over `server.json` in more detail later.

The `server.pem` file is the TLS certificate used to encrypt traffic to and from your server.
This is a self-signed certificate generated automatically by the server.

Clients use a Trust On First Use (TOFU) policy to verify the server's certificate, which means that they will trust the
certificate when they first connect and associate it with the server's hostname/IP address. You need to be careful to
keep the certificate safe, because if you remove or replace it, clients that previously connected to the server will
be unable to connect.

You do not need to use LetsEncrypt or any other certificate authority to generate a certificate.

The `server.db` file is the SQLite database used by the server to store its data. It stores rooms, accounts and other
important data for the server. If the file is removed or replaced, existing rooms and accounts will be lost.

## `server.json`

The `server.json` file contains the server configuration that you can edit. It specifies the host+ports to listen on,
the paths to the certificate and database files, and RPC settings.

It will look something like this:

```json
{
  "listen": [
    "0.0.0.0:20038",
    "[::]:20038"
  ],
  "db_path": "server.db",
  "pem_path": "server.pem",
  "rpc": {
    "interfaces": [
      {
        "address": "unix://friendnet-server.sock",
        "allowed_methods": [
          "*"
        ],
        "cors_allow_all_origins": false
      },
      {
        "address": "http://127.0.0.1:8080",
        "allowed_methods": [
          "GetRooms",
          "GetRoomInfo",
          "GetOnlineUsers",
          "GetOnlineUserInfo"
        ],
        "cors_allow_all_origins": true
      }
    ]
  }
}
```

By default, the server will listen on all interfaces on port `20038`, for both IPv4 and IPv6. In most cases, you do not
need to change this.

Please note that if you are testing the server locally on your machine, you should connect to the address
`127.0.0.1:20038` instead of `0.0.0.0:20038` because the latter is a wildcard address, not a real address that you can
connect to directly. In the case of IPv6, you should use `[::1]:20038` instead of `[::]:20038` for the same reason.

The `rpc` property specifies which interfaces to expose the RPC interface on, and which RPC methods are allowed on those
interfaces.

By default, you will want to keep the first RPC entry because that will be used by default for the RPC client.

Other RPC entries can be used for exposing public RPC endpoints for things like widgets or querying information about
the server.

To require an authorization token to access an endpoint, add a `bearer_token` property to it, like so:

```json
{
	"address": "http://127.0.0.1:8080",
	"allowed_methods": [
		"*"
	],
	"bearer_token": "some-secure-random-token"
}
```

---

Next: [Management](management.md)
