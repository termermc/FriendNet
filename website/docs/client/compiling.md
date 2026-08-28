# Compiling

To compile the FriendNet client, you need the following prerequisites:

- The latest [Go compiler](https://go.dev)
- [Node.js](https://nodejs.org/en) 24.0 or higher (unless you skip the web UI)
- [Git](https://git-scm.com/)
- [make](https://www.gnu.org/software/make/) (If you are on Linux, you probably have it already)

On Windows, substitute `./build` with `build.bat`.

First, clone the repository:

```shell
git clone https://github.com/termermc/FriendNet.git
```

Then, compile the client:

```shell
./build client
```

The compiled client will be in the `client` directory, named something like `friendnet-client` or
`friendnet-client.exe`.

If you do not intend to use the web UI (such as for an embedded client with a different frontend), you can run:

```shell
./build client-noui
```

This will build the client only without building the web UI.
