// Command dashboard runs the local simulator dashboard without hardware access.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/server"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/sim"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, output io.Writer) error {
	flags := flag.NewFlagSet("dashboard", flag.ContinueOnError)
	flags.SetOutput(output)
	sourceName := flags.String("source", "sim", "telemetry source (sim only; no hardware access)")
	listen := flags.String("listen", "127.0.0.1:8080", "HTTP address; use 0.0.0.0:8080 for trusted LAN access")
	interval := flags.Duration("interval", 250*time.Millisecond, "simulator sample interval (at least 50ms)")
	staleAfter := flags.Duration("stale-after", 3*time.Second, "measurement expiry (must exceed sample interval)")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *sourceName != "sim" {
		return fmt.Errorf("unsupported source %q: M0-M2 support sim only", *sourceName)
	}
	if flags.NArg() != 0 || *interval < 50*time.Millisecond || *staleAfter <= *interval {
		return fmt.Errorf("invalid configuration: no positional arguments; interval >= 50ms and stale-after > interval required")
	}
	source, err := sim.New(*interval)
	if err != nil {
		return err
	}
	app, err := server.New(server.Config{SourceName: telemetry.Simulator, StaleAfter: *staleAfter})
	if err != nil {
		return err
	}
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", *listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", *listen, err)
	}
	logger := log.New(output, "dashboard: ", log.LstdFlags)
	logger.Printf("SIMULATOR at http://%s; no vehicle connected", listener.Addr())
	return serve(ctx, listener, app, source)
}

func serve(ctx context.Context, listener net.Listener, app *server.Server, source telemetry.Source) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpServer := &http.Server{
		Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
		MaxHeaderBytes: 16 << 10,
		BaseContext:    func(net.Listener) context.Context { return runCtx },
	}
	results := make(chan error, 2)
	go func() { results <- app.Run(runCtx, source) }()
	go func() {
		err := httpServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		results <- err
	}()
	var result error
	completed := 0
	select {
	case <-ctx.Done():
	case result = <-results:
		completed++
	}
	cancel() // Cancel SSE request contexts before Shutdown waits for them.
	shutdownCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	result = errors.Join(result, httpServer.Shutdown(shutdownCtx))
	if shutdownCtx.Err() != nil {
		result = errors.Join(result, httpServer.Close())
	}
	for completed < 2 {
		result = errors.Join(result, <-results)
		completed++
	}
	return result
}
