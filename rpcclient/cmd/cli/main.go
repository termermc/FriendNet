package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"connectrpc.com/connect"
	"friendnet.org/protocol/pb/serverrpc/v1/serverrpcv1connect"
	"friendnet.org/rpcclient"
)

func main() {
	var rpcAddrRaw string
	var token string
	var doCmd string
	flag.StringVar(
		&rpcAddrRaw,
		"addr",
		"unix://friendnet-server.sock",
		`The RPC server address (such as "unix:///var/run/friendnet-server.sock" or "http://127.0.0.1:8080")`,
	)
	flag.StringVar(
		&token,
		"token",
		"",
		"The bearer token to use for authenticating with the RPC server",
	)
	flag.StringVar(
		&doCmd,
		"cmd",
		"",
		"The RPC command to run instead of launching CLI",
	)
	flag.Parse()

	var addrProto string
	var addr string
	{
		idx := strings.Index(rpcAddrRaw, "://")
		if idx == -1 {
			panic(fmt.Errorf(`address %q is missing a protocol (should be something like "http://127.0.0.1:8080" or "unix:///tmp/server.sock")`, rpcAddrRaw))
		}

		addrProto = rpcAddrRaw[:idx]
		addr = rpcAddrRaw[idx+3:]
	}

	clientTimeout := 5 * time.Second
	dailer := &net.Dialer{
		Timeout: clientTimeout,
	}

	var transport http.RoundTripper
	switch addrProto {
	case "http":
		fallthrough
	case "https":
		break
	case "unix":
		transport = &http.Transport{
			DialContext: func(ctx context.Context, network string, _ string) (net.Conn, error) {
				return dailer.DialContext(ctx, "unix", addr)
			},
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	default:
		panic(fmt.Errorf(`unsupported address protocol %q`, addrProto))
	}

	httpClient := &http.Client{
		Transport: transport,
		Timeout:   clientTimeout,
	}

	var baseUrl string
	if addrProto == "unix" {
		// Fake address to satisfy the URL requirement.
		baseUrl = "http://localhost"
	} else {
		baseUrl = rpcAddrRaw
	}
	rpcClient := serverrpcv1connect.NewServerRpcServiceClient(
		httpClient,
		baseUrl,
		connect.WithGRPCWeb(),
	)

	headers := make(http.Header)
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}

	cli := rpcclient.NewCli(rpcClient, rpcclient.WithHeaders(headers))

	if doCmd != "" {
		doErr := cli.Do(doCmd)
		if doErr != nil {
			_, _ = fmt.Fprintln(os.Stderr, doErr.Error()+"\n")
		}
		return
	}

	cli.Run()
}
