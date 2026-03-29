# Docker

Setting up a FriendNet server with Docker is pretty straightforward.

This guide assumes you already have Docker installed and running.

You should still read the [linux guide](linux.md) to get a better understanding of setting up the server on Linux.
In particular, the sysctl changes that need to be made for better performance.

## Structure

The Docker image expects the following structure:

- `/var/lib/friendnet` - The image's working directory and data directory
- `/etc/friendnet` - The directory that contains the `server.json` config file

When running the image, you should map these to volumes or local directories.

The `server` and `rpcclient` binaries are in `/usr/bin`, so calling `server` or `rpcclient` will work.

## Getting the Image

To get started, you will want to either pull the pre-built amd64 image or build it yourself.

### Pulling

```shell
docker pull git.termer.net/termer/friendnet-server:latest
```

### Building

You will need to have `make` installed.
The make script expects that rootless Docker is running.
If you need to use root, prefix the make command with `sudo`.

```shell
git clone https://github.com/termermc/friendnet.git
make server-docker
```

## Compose

A pre-made `compose.yml` file can be found [here](https://raw.githubusercontent.com/termermc/FriendNet/refs/heads/master/server.compose.yml).

## Using `rpcclient`

The `rpcclient` binary is in the image's PATH.

### With Compose

```shell
docker compose exec -it friendnet-server rpcclient
```

### Without Compose

```shell
docker exec -it <container> rpcclient
```

---

In the management and configuration guides, keep the previous sections of this guide in mind.

Next: [Configuration](configuration.md)
