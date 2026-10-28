// Command dashboard serves simulator or explicitly selected read-only live telemetry.
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
	return runWith(ctx, args, output, dashboardDependencies{})
}

func runWith(ctx context.Context, args []string, output io.Writer, deps dashboardDependencies) error {
	o, err := parseDashboardFlags(args, output)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if deps.listen == nil {
		deps.listen = (&net.ListenConfig{}).Listen
	}
	listener, err := deps.listen(ctx, "tcp", o.listen)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", o.listen, err)
	}
	source, name, err := createSource(o, deps)
	if err != nil {
		return errors.Join(err, listener.Close())
	}
	computer, err := openFuel(o, name)
	if err != nil {
		return errors.Join(err, source.Close(), listener.Close())
	}
	recorder, historyDir, err := openHistory(o, name, computer)
	if err != nil {
		return errors.Join(err, source.Close(), listener.Close(), closeFuel(computer))
	}
	app, err := server.New(server.Config{SourceName: name, StaleAfter: o.staleAfter, Fuel: computer, HistoryDir: historyDir, Recorder: recorder})
	if err != nil {
		return errors.Join(err, source.Close(), listener.Close(), closeFuel(computer), closeHistory(recorder))
	}
	label := "SIMULATOR; no vehicle connected"
	if name == telemetry.Live {
		label = "LIVE; read-only " + o.probe.Transport + "; waiting for adapter"
	}
	if _, err := fmt.Fprintf(output, "dashboard: %s at http://%s\n", label, listener.Addr()); err != nil {
		return errors.Join(err, source.Close(), listener.Close(), closeFuel(computer), closeHistory(recorder))
	}
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
