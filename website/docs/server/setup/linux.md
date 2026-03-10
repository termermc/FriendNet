# Installing on Linux

The FriendNet server can run on any Linux distro running a kernel released in the last ~10 years.

---

Before doing anything, you need to allow FriendNet through your firewall.
The default FriendNet server port is `20038`, and we will be using it for the rest of this
guide. If you used something else, replace `20038` with whatever you are using.

<details>
<summary>Open Port with UFW (Debian, Ubuntu)</summary>

```shell
sudo ufw allow 20038/udp comment 'FriendNet'
```

</details>

<details>
<summary>Open Port with IPTables</summary>

```shell
sudo iptables -A INPUT -p udp -m udp --dport 20038 -j ACCEPT
sudo iptables -A OUTPUT -p udp -m udp --sport 20038 -j ACCEPT
```

</details>

---

Before downloading and starting the server, we'll want to increase the system's UDP buffer sizes:

```shell
# Add sysctl entries.
sudo tee /etc/sysctl.d/99-quic-udp-buffer-increase.conf >/dev/null <<'EOF'
net.core.rmem_max=7500000
net.core.wmem_max=7500000
EOF

# Apply the sysctl entries.
sudo sysctl --system
```

This helps improve performance for the protocol, as it uses the UDP-based QUIC for its transport.
More information can be found [here](https://github.com/quic-go/quic-go/wiki/UDP-Buffer-Sizes).

---

Now that we have allowed the server's port through our firewall, we can download the server.

First, go to the FriendNet [releases](https://github.com/termermc/FriendNet/releases) page on GitHub and
download the binary for your architecture. You most likely want `friendnet-server-linux_amd64.tar.gz`.

Extract the files in the archive:

```shell
tar -xf friendnet-server-linux_*
```

You should now have two files:

```
server
rpcclient
```

To create the server's config file, run it:

```shell
./friendnet-server
```

And then close it by pressing `Ctrl+C`.

---

Before modifying settings, you can optionally add a systemd service file.

Replace `/path/to/friendnet` with the path to the directory where the server and RPC client are located.

```
[Unit]
Description=FriendNet Server
After=network.target

[Service]
ExecStart=/path/to/friendnet/friendnet-server
Restart=unless-stopped
RestartSec=30
User=friendnet
Group=friendnet
WorkingDirectory=/path/to/friendnet

[Install]
WantedBy=multi-user.target
```

---

Next: [Configuration](configuration.md)
