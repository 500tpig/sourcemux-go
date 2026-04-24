package main

import (
	"fmt"
	"os"

	"github.com/bettas/grok-search-go/internal/config"
	"github.com/bettas/grok-search-go/internal/server"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if err := server.Run(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
