// Command obdprobe-ble is the internal native GATT backend for obdprobe.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blehelper"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	flags := flag.NewFlagSet("obdprobe-ble", flag.ContinueOnError)
	name := flags.String("name", "", "explicit BLE device name")
	if err := flags.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *name == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "explicit --name required; use the obdprobe CLI")
		os.Exit(1)
	}
	if err := blehelper.Serve(ctx, *name, os.Stdin, os.Stdout, blehelper.OpenNative); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
