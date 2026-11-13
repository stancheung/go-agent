package main

import (
	"context"
	"go-agent/internal/cli"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	repl := cli.NewRepl()
	repl.Run(ctx)
}
