package main

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/server"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/sim"
)

func TestCommandConfiguration(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"help", []string{"--help"}, false},
		{"unknown source rejected", []string{"--source=unknown"}, true},
		{"negative interval", []string{"--interval=-1s"}, true},
		{"stale before sample", []string{"--interval=5s", "--stale-after=1s"}, true},
		{"unexpected argument", []string{"anything"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			if err := run(t.Context(), tc.args, &output); (err != nil) != tc.wantErr {
				t.Fatalf("run error: %v; output: %s", err, output.String())
			}
		})
	}
}

func TestShutdownWithActiveSSE(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	source, err := sim.New(time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	app, err := server.New(server.Config{})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- serve(ctx, listener, app, source) }()
	requestCtx, stop := context.WithTimeout(t.Context(), 2*time.Second)
	defer stop()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, "http://"+listener.Addr().String()+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	cancel()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown blocked on active SSE")
	}
}
