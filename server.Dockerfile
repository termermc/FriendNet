FROM docker.io/golang:1.26.5-alpine3.24 AS builder

RUN apk add make nodejs npm

WORKDIR /data/build

RUN mkdir -p common
RUN mkdir -p updater
RUN mkdir -p protocol
RUN mkdir -p adminui
RUN mkdir -p rpcclient
RUN mkdir -p stun
RUN mkdir -p ahocorasick
RUN mkdir -p server

COPY common/go.mod common
COPY common/go.sum common
COPY updater/go.mod updater
COPY protocol/go.mod protocol
COPY protocol/go.sum protocol
COPY adminui/go.mod adminui
COPY rpcclient/go.mod rpcclient
COPY rpcclient/go.sum rpcclient
COPY stun/go.mod stun
COPY stun/go.sum stun
COPY ahocorasick/go.mod ahocorasick
COPY server/go.mod server
COPY server/go.sum server

RUN cd server && go mod download
RUN cd rpcclient && go mod download

COPY Makefile .
COPY common common
COPY updater updater
COPY protocol protocol
COPY adminui adminui
COPY rpcclient rpcclient
COPY stun stun
COPY ahocorasick ahocorasick
COPY server server

RUN make rpcclient
RUN make server

FROM docker.io/alpine:3.24.1 AS certs

RUN apk add --no-cache ca-certificates

FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /data/build/server/friendnet-server /usr/bin/server
COPY --from=builder /data/build/rpcclient/friendnet-rpcclient /usr/bin/rpcclient

WORKDIR /var/lib/friendnet

CMD ["/usr/bin/server", "-config", "/etc/friendnet/server.json"]
