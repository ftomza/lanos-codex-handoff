// Command milanalyze reads saved archives without access to a vehicle transport.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/milanalysis"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	f := flag.NewFlagSet("milanalyze", flag.ContinueOnError)
	var usage bytes.Buffer
	f.SetOutput(&usage)
	input := f.String("input", "", "saved MIL archive directory (offline; required)")
	session := f.String("session", "", "optional saved MIL session id")
	incident := f.String("incident", "", "optional incident id (requires --session)")
	asJSON := f.Bool("json", false, "emit deterministic versioned JSON")
	if err := f.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, err = io.Copy(out, &usage)
			return err
		}
		return err
	}
	if *input == "" || f.NArg() != 0 {
		return fmt.Errorf("explicit --input and no positional arguments required")
	}
	o := milanalysis.Options{Session: *session, Incident: *incident}
	if err := o.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r, err := milanalysis.Analyze(ctx, *input, o)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(r)
	}
	return text(out, r)
}
