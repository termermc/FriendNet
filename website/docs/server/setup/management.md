# Management

If your server is not yet running, start it.

Once it is started, enter the directory the server and RPC client binary are in, and run `friendnet-rpcclient`.

> If you cannot enter the directory, or you configured your RPC interface to listen on another socket, you can use the
> `-addr` flag to specify the address of the RPC interface (`unix:///path/to/socket`, `http://127.0.0.1:8080`, etc.).

Once the CLI is open, you can type `help` to see a list of commands.

Some useful commands to set up a new server:

 - `createroom` Create a new room
 - `createaccount` Create an account for a room

The usage for each command is documented in the CLI.
