# Development

This document outlines development tools and practices used for FriendNet.

## Philosophy

These are the philosophies that guide FriendNet development.

### Independence

FriendNet's main guiding principle is independence. A client should only ever need a server to operate, and a server
should not need any external components to operate.

Some examples of this principle:
 - Server binaries are statically-linked and do not depend on the existence of any global system libraries
 - Servers store their state in a local SQLite database instead of an external database server
 - Clients use STUN servers provided by the server, including the server itself

### Compatibility

When it comes to the protocol and its implementation, the main guiding principle is compatibility. The very first
release of the FriendNet client should be able to connect to the latest release of the server, and vice versa. In the
absence of support for any feature, clients and servers should gracefully degrade to simpler functionality. Users often
have very good reasons for not updating, and we should not force them to do so.

In short, **we do not break compatibility**.

### Wariness of Dependencies

For the actual source code, we try to be prudent about dependencies. External dependencies are a liability, and we
should resort to the Go standard library and our own code as much as possible. For example, we did not need the full
range of STUN functionality, so we implemented our own stripped-down STUN client and server. If a dependency is
desirable, consider whether we can vendor it. For example, the [ahocorasick](ahocorasick) implementation we use for
search filtering is vendored.

First-party dependencies like the `golang.org/x/` modules or the `@solid-primitives` are fine because they are official
extensions of libraries we already use.

Dependencies outside already-trusted groups must be considered with scrutiny. Updates must also be justified, not
applied blindly.

### Documentation

From the protocol to the source code, FriendNet must be documented thoroughly. We must not assume we will be the only
implementation of the protocol, and we want to make it as easy as possible for others to both use and adapt or implement
FriendNet. Documentation must also be [user-facing](website/docs) as much as possible.

### No CGo

We do not use CGo. It makes cross-compilation hard and static linking even harder. It also adds additional build
dependencies for maintainers and packagers. CGo is banned in this project with no exceptions.

## Client Development

Developing the client involves two separate components: the client daemon and the web UI. The client daemon is written
in Go and exposes its functionality over gRPC (or gRPC-Web) and ConnectRPC. The web UI is written in TypeScript using
Solid.js as its UI framework, and communicating with the client daemon via ConnectRPC.

Separating the client's logic with its UI has a few benefits:
 - No need for a separate client implementation for headless use on a server (Soulseek, in contrast, requires a
[different client](https://github.com/slskd/slskd) for headless use)
 - Clear separation of concerns
 - Easier implementation of alternate UIs, both web and native
 - Easier automation

### Web UI Development

If you are already running a client, you can connect to it using a development build of the web UI.

To start running the web UI in development mode, run `npm run dev` in the [webui](webui) directory. You should see a URL
show up in the terminal.

Copy the debug URL, then append the following to the end of it

`?token=<client RPC token>&rpc=<client RPC URL>`

where `<client RPC token>` is the token printed at client startup, and `<client RPC URL>` is the RPC URL printed at
client startup. Your final URL should look something like this:

`http://localhost:5173?token=Aan7RbpZavqMjz3mo1wXT38YGAkUqvDigyccyRb3iPI&rpc=http://127.0.0.1:20042`

This specifies the RPC URL and token required to communicate with the client daemon. You can now enjoy hot reloading
and all the other benefits of Solid.js and Vite's development tools.
