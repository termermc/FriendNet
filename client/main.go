package main

import (
	"context"
	"flag"
	"fmt"
	"os"
)

func main() {
	configPath := flag.String("config", "client.json", "path to client config JSON")
	statePath := flag.String("state", "client_state.json", "path to client state JSON")
	flag.Parse()

	config, err := LoadClientConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	state, err := LoadClientState(*statePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load state: %v\n", err)
		os.Exit(1)
	}

	certStore, err := NewJSONCertStore(*statePath, &state)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to init cert store: %v\n", err)
		os.Exit(1)
	}

	cli := NewCLI(os.Stdin, os.Stdout, *configPath, *statePath, &config, &state, certStore)
	if err := cli.Run(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "client error: %v\n", err)
		os.Exit(1)
	}
}
