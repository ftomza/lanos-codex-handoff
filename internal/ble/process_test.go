package ble

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBLEChild(t *testing.T) {
	t.Parallel()
	scenario := os.Getenv("LANOS_BLE_TEST_SCENARIO")
	if scenario == "" {
		return
	}
	if scenario == "stall-start" {
		time.Sleep(20 * time.Second)
		return
	}
	if scenario == "bad-ready" {
		fmt.Println(`{"type":"ready","version":99}`)
		os.Exit(0)
	}
	fmt.Println(`{"type":"ready","version":1}`)
	if scenario == "oversize" {
		fmt.Println(strings.Repeat("x", 40000))
		return
	}
	if scenario == "overflow" {
		for range 8 {
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "data", "data": []byte(strings.Repeat("x", 16000))})
		}
		time.Sleep(20 * time.Second)
		return
	}
	if scenario == "eof" {
		os.Exit(0)
	}
	reader := bufio.NewScanner(os.Stdin)
	for reader.Scan() {
		var m struct {
			Type string `json:"type"`
			ID   uint64 `json:"id"`
			Data []byte `json:"data"`
		}
		if err := json.Unmarshal(reader.Bytes(), &m); err != nil {
			os.Exit(2)
		}
		if m.Type == "close" {
			if scenario == "stall-close" {
				time.Sleep(20 * time.Second)
			}
			os.Exit(0)
		}
		if m.Type != "write" {
			os.Exit(3)
		}
		if scenario == "no-ack" {
			continue
		}
		if scenario == "wrong-ack" {
			m.ID += 10
		}
		if scenario == "late-ack" {
			if m.ID == 1 {
				time.Sleep(100 * time.Millisecond)
			}
			if m.ID == 2 {
				time.Sleep(300 * time.Millisecond)
			}
		}
		encoder := json.NewEncoder(os.Stdout)
		_ = encoder.Encode(map[string]any{"type": "data", "data": []byte("41 0C ")})
		_ = encoder.Encode(map[string]any{"type": "ack", "id": m.ID})
		_ = encoder.Encode(map[string]any{"type": "data", "data": []byte("0D 43\r>")})
	}
	os.Exit(0)
}

func testDevice(t *testing.T, scenario string) *Device {
	t.Helper()
	cmd := childCommand(t, scenario)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	d, err := openCommand(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func childCommand(t *testing.T, scenario string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestBLEChild$")
	cmd.Env = append(os.Environ(), "LANOS_BLE_TEST_SCENARIO="+scenario)
	return cmd
}

func TestInterleavedNotificationsAndOpenContextLifetime(t *testing.T) {
	t.Parallel()
	d := testDevice(t, "normal") // The open context is already canceled here.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	if n, err := d.Write(ctx, []byte("010C\r")); n != 5 || err != nil {
		t.Fatalf("write: %d %v", n, err)
	}
	var result strings.Builder
	buffer := make([]byte, 3)
	for !strings.Contains(result.String(), ">") {
		n, err := d.Read(ctx, buffer)
		if err != nil {
			t.Fatal(err)
		}
		result.Write(buffer[:n])
	}
	if result.String() != "41 0C 0D 43\r>" {
		t.Fatalf("fragments: %q", result.String())
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Read(ctx, buffer); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("read after close: %v", err)
	}
}

func TestFailuresAreBounded(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"eof", "oversize", "overflow", "wrong-ack", "no-ack"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			openCtx, stop := context.WithTimeout(t.Context(), 3*time.Second)
			defer stop()
			d, openErr := openCommand(openCtx, childCommand(t, scenario))
			if openErr != nil {
				if scenario == "eof" || scenario == "oversize" || scenario == "overflow" {
					return
				}
				t.Fatal(openErr)
			}
			defer func() { _ = d.Close() }()
			ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
			defer cancel()
			if scenario == "wrong-ack" || scenario == "no-ack" {
				if _, err := d.Write(ctx, []byte("ATI\r")); err == nil {
					t.Fatal("bad acknowledgement accepted")
				}
				return
			}
			if scenario == "overflow" {
				for {
					d.mu.Lock()
					failed, changed := d.err, d.changed
					d.mu.Unlock()
					if failed != nil {
						break
					}
					select {
					case <-changed:
					case <-ctx.Done():
						t.Fatal("overflow was not rejected")
					}
				}
			}
			_, err := d.Read(ctx, make([]byte, 1))
			if err == nil {
				t.Fatal("missing transport failure")
			}
		})
	}
}

func TestStartupCancellationReapsChild(t *testing.T) {
	t.Parallel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"stall-start", "bad-ready"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(executable, "-test.run=^TestBLEChild$")
			cmd.Env = append(os.Environ(), "LANOS_BLE_TEST_SCENARIO="+scenario)
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			if _, err := openCommand(ctx, cmd); err == nil {
				t.Fatal("invalid startup succeeded")
			}
			if cmd.ProcessState == nil {
				t.Fatal("child not reaped")
			}
		})
	}
}

func TestMissingHelperAndValidation(t *testing.T) {
	t.Parallel()
	_, err := Open(t.Context(), Config{Name: "IOS-Vlink", Helper: "/nonexistent/obdprobe-ble"})
	if err == nil || !strings.Contains(err.Error(), "build-obdprobe") {
		t.Fatalf("missing helper: %v", err)
	}
	_, err = Open(t.Context(), Config{Name: "\n"})
	if err == nil {
		t.Fatal("invalid name accepted")
	}
}

func TestLateAckCannotCompleteNextWrite(t *testing.T) {
	t.Parallel()
	d := testDevice(t, "late-ack")
	ctx, stop := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer stop()
	if _, err := d.Write(ctx, []byte("ATI\r")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("first timeout: %v", err)
	}
	next, cancel := context.WithTimeout(t.Context(), 180*time.Millisecond)
	defer cancel()
	if _, err := d.Write(next, []byte("ATI\r")); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("late ack misattributed: %v", err)
	}
}

func TestCloseUnblocksReadAndPendingWrite(t *testing.T) {
	t.Parallel()
	d := testDevice(t, "no-ack")
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	results := make(chan error, 2)
	go func() { _, err := d.Read(ctx, make([]byte, 16)); results <- err }()
	go func() { _, err := d.Write(ctx, []byte("ATI\r")); results <- err }()
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		select {
		case err := <-results:
			if !errors.Is(err, os.ErrClosed) {
				t.Fatalf("close: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("close did not unblock operation")
		}
	}
}

func TestUnresponsiveCloseIsBoundedAndReaped(t *testing.T) {
	t.Parallel()
	d := testDevice(t, "stall-close")
	started := time.Now()
	if err := d.Close(); err == nil {
		t.Fatal("expected forced cleanup to report child termination")
	}
	if time.Since(started) > 4*time.Second || d.cmd.ProcessState == nil {
		t.Fatal("child cleanup exceeded bound or did not reap")
	}
}
