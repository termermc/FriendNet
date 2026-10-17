# Used to provide certificates to scratch images.
FROM docker.io/alpine:3.24.1 AS certs

RUN apk add --no-cache ca-certificates

# Used to copy the Go toolchain.
FROM docker.io/golang:1.27.0-alpine3.24 AS go

FROM docker.io/node:26.8.2-alpine3.24 AS webui-builder

# Copy from Go image.
# We do this so that we can use a Node distribution from Docker Hub instead of doing `apk add`.
# I expect that the Node image will exist longer than the repo `apk add node` depends on.
RUN mkdir -p /usr/local/go
COPY --from=go /usr/local/go /usr/local/go
ENV PATH="$PATH:/usr/local/go/bin"
ENV GOROOT=/usr/local/go

WORKDIR /data/build

COPY ./build* .
COPY webui webui

RUN ./build webui

FROM go AS builder

WORKDIR /data/build

RUN mkdir -p browser
RUN mkdir -p client
RUN mkdir -p common
RUN mkdir -p mkcert
RUN mkdir -p protocol
RUN mkdir -p stun
RUN mkdir -p updater
RUN mkdir -p upnp
RUN mkdir -p webui

RUN mkdir -p webui/dist

COPY browser/go.mod browser
COPY browser/go.sum browser
COPY client/go.mod client
COPY client/go.sum client
COPY common/go.mod common
COPY common/go.sum common
COPY mkcert/go.mod mkcert
COPY mkcert/go.sum mkcert
COPY protocol/go.mod protocol
COPY protocol/go.sum protocol
COPY stun/go.mod stun
COPY stun/go.sum stun
COPY updater/go.mod updater
COPY updater/go.sum updater
COPY upnp/go.mod upnp
COPY upnp/go.sum upnp
COPY webui/go.mod webui
COPY webui/go.sum webui

RUN cd client && go mod download

COPY browser browser
COPY client client
COPY common common
COPY mkcert mkcert
COPY protocol protocol
COPY stun stun
COPY updater updater
COPY upnp upnp
COPY webui webui

COPY --from=webui-builder /data/build/webui/dist/ webui/dist

COPY ./build* .

RUN ./build client-noui

FROM scratch

ENV PATH="$PATH:/usr/bin"

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /data/build/client/friendnet-client /usr/bin/client

WORKDIR /var/lib/friendnet

CMD ["client", "-headless"]
