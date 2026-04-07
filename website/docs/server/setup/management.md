# Management

If your server is not yet running, start it.

The server can be managed while it is running using either its built-in CLI, or the separate RPC client.

Once the CLI is open, you can type `help` to see a list of commands.

Some useful commands to set up a new server:

- `createroom` Create a new room
- `createaccount` Create an account for a room

The usage for each command is documented in the CLI.

## RPC Client

To manage the server remotely or without having to touch its CLI, you can use the RPC client. It works the same way as
the CLI, but can be used to manage the server remotely.

The RPC client binary is packaged with the server and named `rpcclient`. To use it, you can call it in the same
directory where the server's management socket is located (usually the server's current working directory), or specify
it manually using the `-addr` flag.

The `-addr` flag can be used to specify the address of the RPC interface
(`unix:///path/to/socket`, `http://127.0.0.1:8080`, etc.). It can be a UNIX socket path, or an HTTP or HTTPS URL.

If the interface requires a bearer token, use the `-token` flag to specify it.

RPC interfaces can be configured in the server's `server.json` file.
