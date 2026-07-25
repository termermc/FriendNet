# We Got NAT Hole Punching!

With the release of FriendNet v1.2.0, most users can now directly connect to each other without needing to open ports,
even behind NATs and firewalls.

Previously, FriendNet relied on UPnP and manual port forwarding to establish direct connections between users on a
server. If neither side was able to open a port, direct connection attempts would fail and clients would fall back to
using the server as a relay. This was a big problem for users behind NATs and firewalls. Falling back to the relay is
*fine*, but not ideal, and reduces speed while increasing latency. Fortunately, there's a solution that can usually get
around these issues.

[NAT hole punching](https://en.wikipedia.org/wiki/Hole_punching_(networking)) (sometimes called "NAT traversal") is a
technique that allows two devices behind NATs and/or firewalls to establish direct connections. If you're curious about
how it works at a technical level, you can read Tailscale's
[How NAT traversal works](https://tailscale.com/blog/how-nat-traversal-works) blog post. At a high level though, as long
as the two peers are able to learn their own public IP addresses and send them to each other, they can usually establish
direct connections if they try to connect to each other at the same time.

We implemented NAT hole punching in FriendNet v1.2.0. If two clients and the server are running at least version 1.2.0,
there is a very good chance that the clients will be able to use NAT hole punching to connect to each other.

This is a feature that a lot of P2P file sharing applications lack; I particularly wish that Soulseek had it. I'm hoping
this release will make it easier for people to use FriendNet.

Enjoy!
