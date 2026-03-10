.PHONY: \
	help \
	install-tools \
	pb \
	server \
	server-linux-amd64 \
	client \
	client-noui \
	client-windows-amd64 \
	client-windows-amd64-noui \
	client-linux-amd64 \
	client-linux-amd64-noui \
	client-darwin-arm64 \
	client-darwin-arm64-noui \
	rpcclient \
	rpcclient-linux-amd64 \
	run-rpcclient \
	release-artifacts \
	server-docker \
	server-docker-publish

help:
	echo "Read the Makefile to see options"

install-tools:
	go install github.com/bufbuild/buf/cmd/buf@v1.64.0
	go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
	go install connectrpc.com/connect/cmd/protoc-gen-connect-go@v1.19.1

pb:
	cd protocol && buf lint
	cd protocol && buf generate
	cd webui && npx buf lint
	cd webui && npx buf generate
	cd server-widget && npx buf lint
	cd server-widget && npx buf generate

server:
	cd server && go build -o friendnet-server friendnet.org/server/cmd/server

server-linux-amd64:
	cd server && GOOS=linux GOARCH=amd64 go build -o friendnet-server friendnet.org/server/cmd/server

client:
	cd webui && go generate && cd ../client && go build -o friendnet-client friendnet.org/client/cmd/client

client-noui:
	cd client && go build -o friendnet-client friendnet.org/client/cmd/client

client-windows-amd64:
	cd webui && go generate && cd ../client && GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o friendnet-client.exe friendnet.org/client/cmd/client

client-windows-amd64-noui:
	cd client && GOOS=windows GOARCH=amd64 go build -ldflags="-H windowsgui" -o friendnet-client.exe friendnet.org/client/cmd/client

client-linux-amd64:
	cd webui && go generate && cd ../client && GOOS=linux GOARCH=amd64 go build -o friendnet-client friendnet.org/client/cmd/client

client-linux-amd64-noui:
	cd client && GOOS=linux GOARCH=amd64 go build -o friendnet-client friendnet.org/client/cmd/client

client-darwin-arm64:
	cd webui && go generate && cd ../client && GOOS=darwin GOARCH=arm64 go build -o friendnet-client friendnet.org/client/cmd/client

client-darwin-arm64-noui:
	cd client && GOOS=darwin GOARCH=arm64 go build -o friendnet-client friendnet.org/client/cmd/client

rpcclient:
	cd rpcclient && go build -o friendnet-rpcclient friendnet.org/rpcclient/cmd/cli

rpcclient-linux-amd64:
	cd rpcclient && GOOS=linux GOARCH=amd64 go build -o friendnet-rpcclient friendnet.org/rpcclient/cmd/cli

run-rpcclient:
	make rpcclient && cd server && ../rpcclient/friendnet-rpcclient

release-artifacts:
	rm -rf /tmp/fn-release
	mkdir /tmp/fn-release
	make client-linux-amd64 && mv client/friendnet-client /tmp/fn-release/friendnet-client-linux_amd64
	make client-windows-amd64 && mv client/friendnet-client.exe /tmp/fn-release/friendnet-client-windows_amd64.exe
	make client-darwin-arm64 && mv client/friendnet-client /tmp/fn-release/friendnet-client-macos_arm64
	make server-linux-amd64 && mv server/friendnet-server /tmp/fn-release/server
	make rpcclient-linux-amd64 && mv rpcclient/friendnet-rpcclient /tmp/fn-release/rpcclient
	chmod +x /tmp/fn-release/*
	cd /tmp/fn-release && tar -czf friendnet-server-linux_amd64.tar.gz server rpcclient
	rm /tmp/fn-release/friendnet-server && rm /tmp/fn-release/friendnet-rpcclient
	echo "Artifacts in /tmp/fn-release"

server-docker:
	docker build -t git.termer.net/termer/friendnet-server:latest -f server.Dockerfile .

server-docker-publish:
	make server-docker && docker push git.termer.net/termer/friendnet-server:latest
