# Compiling

To compile the FriendNet client, you need the following prerequisites:

- The latest [Go compiler](https://go.dev)
- [Node.js](https://nodejs.org/en) 24.0 or higher
- [Git](https://git-scm.com/)
- [make](https://www.gnu.org/software/make/) (If you are on Linux, you probably have it already)

First, clone the repository:

```shell
git clone https://github.com/termermc/FriendNet.git
```

Then, compile the client:

```shell
make client
```

The compiled client will be in the `client` directory, named something like `friendnet-client` or
`friendnet-client.exe`.
