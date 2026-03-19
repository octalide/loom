package main

import (
	"context"
	"fmt"
	"os"

	"github.com/octalide/loom/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "loom: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	s, err := server.New()
	if err != nil {
		return err
	}
	return s.Run(context.Background())
}
