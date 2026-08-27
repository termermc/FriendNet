# Used to provide certificates to scratch images.
FROM docker.io/alpine:3.24.1 AS certs

RUN apk add --no-cache ca-certificates

# Used to copy the Go toolchain.
FROM docker.io/golang:1.27.0-alpine3.24 AS go

WORKDIR /data/

RUN apk add busybox-static

RUN ln -s /bin/busybox.static ./sh
RUN ln -s /bin/busybox.static ./ln

FROM go AS adminui-builder

RUN apk add nodejs npm

WORKDIR /data/build

# Dummy workspace file that lets us use tools.
RUN printf "go 1.27.0\n\nuse (\n./adminui\n./tool\n)\n" > go.work
COPY Taskfile.yml .
COPY tool tool
COPY adminui adminui

RUN go tool task adminui

#FROM docker.io/golang:1.27.0-alpine3.24 AS builder
FROM scratch AS builder

COPY --from=go /bin/busybox.static /bin/busybox.static
COPY --from=go /data/sh /bin/sh
COPY --from=go /data/ln /bin/ln
RUN ln -s /bin/busybox.static /bin/mkdir
RUN mkdir /tmp
RUN mkdir -p /usr/local/go

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=go /usr/local/go /usr/local/go

ENV PATH=/bin/:/usr/local/go/bin
ENV GOROOT=/usr/local/go

WORKDIR /data/build

COPY Taskfile.yml .

# Dummy workspace file that lets us use tools.
RUN printf "go 1.27.0\n\nuse (\n./ahocorasick\n./common\n./protocol\n./rpcclient\n./server\n./stun\n./tool\n./updater\n)\n" > go.work

RUN mkdir -p adminui
RUN mkdir -p ahocorasick
RUN mkdir -p common
RUN mkdir -p protocol
RUN mkdir -p rpcclient
RUN mkdir -p server
RUN mkdir -p stun
RUN mkdir -p tool
RUN mkdir -p updater

RUN mkdir -p adminui/dist

COPY adminui/go.mod adminui
COPY adminui/go.sum adminui
COPY ahocorasick/go.mod ahocorasick
COPY ahocorasick/go.sum ahocorasick
COPY common/go.mod common
COPY common/go.sum common
COPY protocol/go.mod protocol
COPY protocol/go.sum protocol
COPY rpcclient/go.mod rpcclient
COPY rpcclient/go.sum rpcclient
COPY server/go.mod server
COPY server/go.sum server
COPY stun/go.mod stun
COPY stun/go.sum stun
COPY tool/go.mod tool
COPY tool/go.sum tool
COPY updater/go.mod updater
COPY updater/go.sum updater

RUN cd server && go mod download
RUN cd rpcclient && go mod download

COPY tool tool
COPY common common
COPY updater updater
COPY protocol protocol
COPY adminui adminui
COPY rpcclient rpcclient
COPY stun stun
COPY ahocorasick ahocorasick
COPY server server

COPY --from=adminui-builder /data/build/adminui/dist/ adminui/dist

RUN go tool task rpcclient
RUN go tool task server-noui

FROM scratch

ENV PATH="/usr/bin"

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /data/build/server/friendnet-server /usr/bin/server
COPY --from=builder /data/build/rpcclient/friendnet-rpcclient /usr/bin/rpcclient

WORKDIR /var/lib/friendnet

CMD ["server", "-config", "/etc/friendnet/server.json"]
