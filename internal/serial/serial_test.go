package serial

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestListPortsDoesNotOpenDevices(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for _, name := range []string{"cu.Android-Vlink", "cu.usbserial", "tty.fake"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	ports, err := listPorts(filepath.Join(dir, "cu.*"))
	if err != nil || len(ports) != 2 {
		t.Fatalf("ports: %v %v", ports, err)
	}
}

func TestOpenValidationAndCancellation(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		port string
		baud int
	}{{"", 38400}, {"/tmp/not-a-device", 38400}, {"/dev/cu.fake", 12345}} {
		if _, err := Open(t.Context(), tc.port, tc.baud); err == nil {
			t.Fatalf("accepted %+v", tc)
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Open(ctx, "/dev/cu.fake", 38400); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
}
