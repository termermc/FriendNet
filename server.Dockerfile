FROM docker.io/golang:1.26.1-alpine3.23 AS builder

RUN apk add make

WORKDIR /data/build

RUN mkdir -p common
RUN mkdir -p protocol
RUN mkdir -p server
RUN mkdir -p rpcclient

COPY common/go.mod common
COPY common/go.sum common
COPY protocol/go.mod protocol
COPY protocol/go.sum protocol
COPY server/go.mod server
COPY server/go.sum server
COPY rpcclient/go.mod rpcclient
COPY rpcclient/go.sum rpcclient

RUN cd server && go mod download
RUN cd rpcclient && go mod download

COPY Makefile .
COPY common common
COPY protocol protocol
COPY server server
COPY rpcclient rpcclient

RUN make server
RUN make rpcclient

FROM docker.io/alpine:3.23.3

COPY --from=builder /data/build/server/friendnet-server /usr/bin/server
COPY --from=builder /data/build/rpcclient/friendnet-rpcclient /usr/bin/rpcclient

WORKDIR /var/lib/friendnet

CMD ["server", "-config", "/etc/friendnet/server.json"]
