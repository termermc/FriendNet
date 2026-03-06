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

Now that we have allowed the server's port through our firewall, we can download the server.

First, go to the FriendNet [releases](https://github.com/termermc/FriendNet) page on Github and
download the binary for your architecture. You most likely want `friendnet-server-linux_amd64.tar.gz`.

Extract the files in the archive:

```shell
tar -xf friendnet-server-linux_*
```

You should now have two files:

```
friendnet-server
friendnet-rpcclient
```


