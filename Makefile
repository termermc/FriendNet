.PHONY: help install-tools pb server client

help:
	echo "Read the Makefile to see options"

install-tools:
	go install github.com/bufbuild/buf/cmd/buf@v1.64.0
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
	go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.19.1

pb:
	cd protocol && buf lint
	cd protocol && buf generate

server:
	cd server && go build -o friendnet-server friendnet.org/server/cmd/server

client:
	cd client && go build -o friendnet-client friendnet.org/client/cmd/client
