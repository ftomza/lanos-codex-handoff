package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func TestDashboardLiveFlags(t *testing.T) {
	t.Parallel()
	o, err := parseDashboardFlags([]string{"--source=elm327"}, &bytes.Buffer{})
	if err != nil || o.probe.Transport != "ble" || o.probe.BLEName != "IOS-Vlink" || o.probe.Retries != 2 || o.source != "elm327" {
		t.Fatalf("defaults: %+v %v", o, err)
	}
	for _, args := range [][]string{
		{"--transport=ble"}, {"--source=elm327", "--interval=1s"}, {"--source=elm327", "--port=/dev/cu.fake"},
		{"--source=elm327", "--baud=38400"}, {"--source=elm327", "--ble-name="}, {"--source=elm327", "--timeout=0s"},
		{"--source=elm327", "--transport=serial"}, {"--source=elm327", "--transport=serial", "--port=/dev/cu.fake", "--ble-name=IOS-Vlink"},
		{"--source=elm327", "--retries=6"}, {"--source=elm327", "--stale-after=0s"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			t.Parallel()
			if _, err := parseDashboardFlags(args, &bytes.Buffer{}); err == nil {
				t.Fatalf("unsafe flags: %v", args)
			}
		})
	}
}

func TestBindFailureDoesNotCreateLiveSource(t *testing.T) {
	t.Parallel()
	cause := errors.New("address in use")
	created := false
	err := runWith(t.Context(), []string{"--source=elm327"}, &bytes.Buffer{}, dashboardDependencies{
		listen: func(context.Context, string, string) (net.Listener, error) { return nil, cause },
		live: func(probe.Config) (telemetry.Source, error) {
			created = true
			return nil, errors.New("should not create")
		},
	})
	if !errors.Is(err, cause) || created {
		t.Fatalf("hardware before bind: %v created=%t", err, created)
	}
}

func TestHelpDoesNotCreateSourceOrListener(t *testing.T) {
	t.Parallel()
	called := false
	deps := dashboardDependencies{listen: func(context.Context, string, string) (net.Listener, error) {
		called = true
		return nil, errors.New("unexpected")
	}, live: func(probe.Config) (telemetry.Source, error) { called = true; return nil, errors.New("unexpected") }}
	var out bytes.Buffer
	if err := runWith(t.Context(), []string{"--help"}, &out, deps); err != nil || called || !strings.Contains(out.String(), "elm327") {
		t.Fatalf("help: %v called=%t %s", err, called, out.String())
	}
}

func TestLiveServerSurvivesTerminalSourceFailure(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	bound := make(chan net.Listener, 1)
	started := make(chan struct{})
	done := make(chan error, 1)
	deps := dashboardDependencies{
		listen: func(ctx context.Context, network, address string) (net.Listener, error) {
			l, err := (&net.ListenConfig{}).Listen(ctx, network, "127.0.0.1:0")
			if err == nil {
				bound <- l
			}
			return l, err
		},
		live: func(cfg probe.Config) (telemetry.Source, error) {
			if cfg.Transport != "ble" {
				t.Error("wrong transport")
			}
			return &terminalSource{started: started}, nil
		},
	}
	go func() { done <- runWith(ctx, []string{"--source=elm327", "--fuel=off"}, &bytes.Buffer{}, deps) }()
	listener := <-bound
	<-started
	requestCtx, stop := context.WithTimeout(t.Context(), 2*time.Second)
	defer stop()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, "http://"+listener.Addr().String()+"/api/snapshot", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := (&http.Client{Timeout: 2 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || !bytes.Contains(data, []byte(`"source":"live"`)) {
		t.Fatalf("server unavailable after source failure: %s %v", data, err)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown stalled")
	}
}

type terminalSource struct {
	started chan struct{}
	sent    bool
}

func (s *terminalSource) Start(context.Context) error { close(s.started); return nil }
func (s *terminalSource) Read(ctx context.Context) (telemetry.Snapshot, error) {
	if !s.sent {
		s.sent = true
		return telemetry.Snapshot{Source: telemetry.Live, Health: telemetry.Disconnected, Message: "restart dashboard"}, nil
	}
	<-ctx.Done()
	return telemetry.Snapshot{}, ctx.Err()
}
func (s *terminalSource) Close() error { return nil }
