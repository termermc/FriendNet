.PHONY: help install-tools pb pbweb server client client-windows rpcclient run-rpcclient

help:
	echo "Read the Makefile to see options"

install-tools:
	go install github.com/bufbuild/buf/cmd/buf@v1.64.0
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
	go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.19.1

pb:
	cd protocol && buf lint
	cd protocol && buf generate

pbweb:
	cd webui && npx buf lint
	cd webui && npx buf generate

server:
	cd server && go build -o friendnet-server friendnet.org/server/cmd/server

client:
	cd webui && go generate && cd ../client && go build -o friendnet-client friendnet.org/client/cmd/client

client-windows:
	cd webui && go generate && cd ../client && GOOS=windows GOARCH=amd64 go build -o friendnet-client.exe friendnet.org/client/cmd/client

rpcclient:
	cd rpcclient && go build -o friendnet-rpcclient friendnet.org/rpcclient/cmd/cli

run-rpcclient:
	make rpcclient && cd server && ../rpcclient/friendnet-rpcclient
