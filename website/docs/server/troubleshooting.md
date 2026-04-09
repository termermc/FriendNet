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

### You're running the server in Docker without host networking

Docker can have issues with UDP forwarding which prevents the server from sending or receiving UDP traffic.
You should run the server with the host network driver. See the [Docker setup guide](./setup/docker.md) for more
information.

## I can't figure out how to add users/rooms after the server has started

If the server is running inside a systemd service or somewhere else that prevents you from using its built-in CLI, you
can use the RPC client to connect to it remotely. See the [management](./setup/management.md) guide for more
information.
