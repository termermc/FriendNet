.PHONY: install-tools pb server client

install-tools:
	go install github.com/bufbuild/buf/cmd/buf@v1.64.0
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11

pb:
	cd protocol && buf lint
	cd protocol && buf generate

server:
	cd server && go build -o friendnet-server friendnet.org/server/cmd/server

client:
	cd client && go build -o friendnet-client friendnet.org/client/cmd/client
