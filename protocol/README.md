# Overview

The FriendNet protocol operates over QUIC, with clients making outbound connections to servers.

The protocol is mostly based around a simple request-response model, where a request is initiated by the creation of a BiDi stream,
with a protocol message written immediately to it by the opener.

After the initial stream message has been received, it is up to the receiver to either send back a reply, read extra data (if applicable), or close the stream.

There are 3 message classes:
 - C2S (Client to Server) messages are sent by the client to the server.
 - S2C (Server to Client) messages are sent by the server to the client.
 - C2C (Client to Client) messages are sent by a client to another client (either directly or by proxy of the server).

# Message Layout

Protocol messages are encoded with the following layout (ranges are bytes, inclusive):
0-3: The message type (uint32, little endian)
4-7: The payload length, in bytes (uint32, little endian)
8-?: The payload

The message type is a value of the ProtoMessageType enum.
The payload will be a protobuf message corresponding to the type's enum name.
For example, for the type PROTO_ERROR, the payload will be of type ProtoMessageError.

Message layout shall not change between versions, although the data following the message is protocol-defined and may change.

# Version Negotiation

The protocol negotiation stage must occur immediately after a connection is opened.
Steps are as follows:
1. Client opens a BiDi stream.
2. Client sends PROTO_VERSION with its protocol version.
3. Server sends either PROTO_VERSION_ACCEPTED or PROTO_VERSION_REJECTED.
   a. If PROTO_VERSION_REJECTED, the server's protocol version, a reason enum value and optionally a message will be provided.
   b. If PROTO_VERSION_ACCEPTED, the server's protocol version will be provided.
   c. If the server sends any message type other than PROTO_VERSION, it will send PROTO_ERROR of type PROTO_ERROR_UNEXPECTED_REPLY.

Any errors in the negotiation stage, including incorrect version, will result in the termination of the connection.
If the negotiation is not finished in a timeout defined by the server, the connection will be terminated without a reason.
The server will try to send any error or rejection messages on the protocol negotiation stream (if any) before closing the connection,
but the connection will be terminated regardless if a message send timeout is reached.

If the client received PROTO_VERSION_ACCEPTED, the version is now negotiation and the authentication handshake stage may begin.
Once the version is accepted, the client may safely assume that the server supports its protocol version.
Even if the server reports that its version is different from the clients, the client may safely assume that all messages it receives will
be compatible with its version, and that the server will understand all messages it receives from the client.

Note that the client will not receive any PROTO_PING messages during version negotiation, and it should also not send any itself.

The protocol version negotiation process shall not change between versions.

# Handshake and Authentication

The handshake stage must occur immediately after the protocol version is negotiated.
Steps are as follows:
1. Client opens a BiDi stream.
2. Client sends PROTO_AUTHENTICATE with the proper credentials.
3. Server sends either PROTO_AUTH_ACCEPTED or PROTO_AUTH_REJECTED
   a. If PROTO_AUTH_REJECTED, a reason enum value and optionally a message will be provided.
   b. If PROTO_AUTH_ACCEPTED, information about the authenticated user will be provided.
   c. If the server received any message type other than PROTO_AUTHENTICATE, it will reply with PROTO_ERROR of type PROTO_ERROR_UNEXPECTED_REPLY.

Any errors in the handshake process, including invalid credentials, will result in the termination of the connection.
If the handshake is not finished in a timeout defined by the server, the connection will be terminated without a reason.
The server will try to send any error or rejection messages on the handshake stream (if any) before closing the connection,
but the connection will be terminated regardless if a message send timeout is reached.

If the client received PROTO_AUTH_ACCEPTED, the connection is now authenticated and a session has been established.
Note that the client will not receive any PROTO_PING messages during the handshake, and it should also not send any itself.

# Ping

Both the client and server are expected to reply to new Bidi steams of PROTO_PING with PROTO_PONG.
The server at its discretion may reply to PROTO_PING with an error of type PROTO_ERROR_RATE_LIMITED, which shall not be reason for the client
to terminate the connection. The client, however, must not reply to PROTO_PING with any error.
Besides the exception mentioned above, if either party replies to PROTO_PING with anything other than PROTO_PONG, the connection must be
terminated. This also applies if a reply timeout is reached.

A client's PROTO_PING messages shall not be rate limited if sent at a rate of 1 per second or lower.
Clients are not required to send PROTO_PING messages, but may do so for their own purposes.
Regardless of which party is sending the ping, the timestamp sent along with it must be an accurate UNIX epoch millisecond for when the message was sent.

Note that all of the above only apply to authenticated clients. The server has no responsibility to respond to ping requests sent while a client is unauthenticated.

# Versioning

The protocol uses semantic versioning (MAJOR.MINOR.PATCH).
The patch version may introduce new features that are fully backwards compatible with the previous version.
Changes introduced in patch versions must not be required for clients to continue to work normally.
The minor version may introduce new features and small backwards incompatible changes that do not break older clients.
The minor version may not change the handshake process.
The major version may change anything with no regard to backwards compatibility, except for version negotiation.

Examples:
v1.0.0-v1.0.1:
 - Introduces a new status indicator, but clients do not need it to work normally
v1.0.1-v1.1.0:
 - Introduces a new chat message type which clients on earlier versions do not understand
 - Removes the ability to fetch online users, requests for them will return empty now
v1.1.0-v2.0.0:
 - Reworks the handshake process
 - Changes the chat message format
 - Removes unpaginated file fetching

## Compatibility Expectations

If the server accepts a client's version, it can be safely assumed that the client's version is fully supported.
If the client's version differs from the server's but the server accepts it, the client must be prepared to ignore unrecognized messages and fields.
If the client's version matches the server's, unrecognized messages and fields should be treated as erroneous behavior on the part of the server.
In the case that the client's version is greater than the server's, the client must be prepared to handle cases where messages are unrecognized by the server.

# Proxy Streams

When a direct connection is not possible or desired, a client may send a proxy request on a new BiDi to the server specifying the client it wishes
to connect to. Upon receipt of the request, the server will open a BiDi stream to the client with a proxy message indicating the client on the other end.

The server will not read any messages past the initial proxy messages on either side; it will proxy all further stream data transparently. If either side
cancels their stream, the server will close the other side's stream and end the proxy stream.

In cases where the server is unable to connect to the desired destination, it will cancel the stream without sending any data.
It does not send any failure message because there would be no way for the client that requested the proxy to know whether the message was sent by the server
or sent by the destination client.

# About Paths

Paths within the protocol are local to users and based on what they choose to share.
All paths must begin with `/`.

For example: `/shared music/Kevin MacLeod/Monkeys Spinning Monkeys.mp3`

Typically, the first directory in the path will be a shared folder, but the protocol itself has no concept of shares.
