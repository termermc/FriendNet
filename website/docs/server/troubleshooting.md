# Troubleshooting

Some solutions and guidance for common problems.

## Users Can't Connect to My Server

This could be for a number of reasons.

### 1. Your port is not open for **UDP** traffic

First, check that `20038` (or the port your server uses) is open for **UDP** traffic in your firewall.
Opening `20038` for TCP will not work.

### 2. The user is already connected on another client/device

Servers only allow one client connected per-user, so have the user check their client's log viewer.
If they see something like `username already connected`, they are already connected on another client, or they have
multiple server entries on their client for the same server.

### 3. The user has the wrong credentials

Have the user check their client's log viewer. If they have the wrong credentials, they will see a message like
`invalid credentials`.

### 4. The server's certificate was changed

Have the user check their client's log viewer. If they see something like `server certificate mismatch`, then the
server's certificate has changed.

If the server was recreated or its `server.pem` file was deleted or replaced, the server will have a different
certificate from the last time the client connected. Clients will refuse to connect to a server if its certificate is
different from the last time the client connected.

To make the client forget the old certificate, have the user run `friendnet-client -rmcerthost <hostname>`. The
`<hostname>` must be the server's host without the port, so `127.0.0.1`, `example.com`, etc. For example, if the client
is trying to connect to `127.0.0.1:20038`, then they must type `friendnet-client -rmcerthost 127.0.0.1`. After doing,
the client will be able to connect to the server again.

Every client that previously connected to the server will need to perform these steps to be able to connect again.

### You're running the server in Docker without host networking

Docker can have issues with UDP forwarding which prevents the server from sending or receiving UDP traffic.
You should run the server with the host network driver. See the [Docker setup guide](./setup/docker.md) for more
information.

## I can't figure out how to add users/rooms after the server has started

If the server is running inside a systemd service or somewhere else that prevents you from using its built-in CLI, you
can use the RPC client to connect to it remotely. See the [management](./setup/management.md) guide for more
information.
