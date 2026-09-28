package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/corkine/ci-transfer/internal/cli"
	"github.com/corkine/ci-transfer/internal/transfer"
)

var version = "dev"

func main() {
	args, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(2)
	}
	if args.Help {
		fmt.Print(cli.Usage)
		return
	}
	if args.Version {
		fmt.Println("ci-transfer", version)
		return
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := transfer.Run(ctx, args.Options); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
