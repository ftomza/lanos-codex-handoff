// Command obdprobe performs finite, read-only ELM/OBD discovery on macOS.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout, os.Stderr, dependencies{}); err != nil {
		fmt.Fprintln(os.Stderr, "obdprobe:", err)
		os.Exit(1)
	}
}
