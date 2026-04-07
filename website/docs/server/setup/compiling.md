# Compiling

To compile the FriendNet server, you need the following prerequisites:

- The latest [Go compiler](https://go.dev)
- [Git](https://git-scm.com/)
- [make](https://www.gnu.org/software/make/) (If you are on Linux, you probably have it already)

First, clone the repository:

```shell
git clone https://github.com/termermc/FriendNet.git
```

Then, compile the server:

```shell
make server
```

The compiled server will be in the `server` directory, named something like `friendnet-server` or
`friendnet-server.exe`.

You will also need to compile the RPC client if you want to remotely manage the server:

```shell
make rpcclient
```

The RPC client will be in the `rpcclient` directory, named `friendnet-rpcclient` or
`friendnet-rpcclient.exe`.

---

Next: [Configuration](configuration.md)
