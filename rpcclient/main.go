package main

import (
	"flag"
	"log/slog"
	"os"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	var rpcAddr string
	var doCmd string
	flag.StringVar(
		&rpcAddr,
		"addr",
		"unix://friendnet-server.sock",
		`The RPC server address (such as "unix:///var/run/friendnet-server.sock" or "http://127.0.0.1:8080")`,
	)
	flag.StringVar(
		&doCmd,
		"cmd",
		"",
		"The RPC command to run instead of launching CLI",
	)
	flag.Parse()

	_ = logger
	println(rpcAddr)
}
