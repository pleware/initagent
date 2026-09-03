// Command fakecoder is a stand-in coding CLI for completion, scheduler,
// lease, and fence tests. The behaviour lives in internal/fakecoder; this
// wrapper owns only signal handling and the process exit status.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ErzenXz/overseer/internal/fakecoder"
)

func main() {
	cfg, err := fakecoder.ParseArgs(os.Args[1:], os.Stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "fakecoder: %v\n", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := fakecoder.Run(ctx, cfg, os.Stderr)
	stop()

	os.Exit(code)
}
