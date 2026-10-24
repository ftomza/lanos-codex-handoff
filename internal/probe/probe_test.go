package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/elm327"
)

// The fixture remains exact. Responses to commands absent from Issue #1 are
// explicitly synthetic test inputs; they are never recorded as hardware facts.
type adapter struct {
	responses   map[string]string
	commands    []string
	deadlines   map[string]time.Duration
	pending     []byte
	writeBuffer bytes.Buffer
	failCommand string
	latePrompt  bool
	failed      bool
	disconnect  bool
	closed      bool
}

func fixtureAdapter(t *testing.T) *adapter {
	t.Helper()
	b, err := os.ReadFile("../elm327/testdata/vgate-2026-10-24.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Exchanges []struct{ Command, Raw string }
	}
	if err := json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	a := &adapter{responses: make(map[string]string, len(f.Exchanges)), deadlines: make(map[string]time.Duration)}
	for _, x := range f.Exchanges {
		a.responses[x.Command] = x.Raw
	}
	// Explicitly synthetic: unknown next bitmap, adapter voltage and MIL response.
	a.responses["0120"] = "NO DATA\r>"
	a.responses["ATRV"] = "14.2V\r>"
	a.responses["0101"] = "410100000000\r>"
	return a
}

func (a *adapter) Write(ctx context.Context, b []byte) (int, error) {
	a.writeBuffer.Write(b)
	if strings.HasSuffix(a.writeBuffer.String(), "\r") {
		command := strings.TrimSuffix(a.writeBuffer.String(), "\r")
		a.writeBuffer.Reset()
		a.commands = append(a.commands, command)
		deadline, _ := ctx.Deadline()
		a.deadlines[command] = time.Until(deadline)
		if command == a.failCommand && !a.failed {
			a.failed = true
			return len(b), nil
		}
		raw, ok := a.responses[command]
		if !ok {
			raw = "NO DATA\r>"
		}
		a.pending = append(a.pending, []byte(raw)...)
	}
	return len(b), nil
}
func (a *adapter) Read(ctx context.Context, b []byte) (int, error) {
	if len(a.pending) > 0 {
		n := copy(b, a.pending)
		a.pending = a.pending[n:]
		return n, nil
	}
	if a.failed && a.disconnect {
		return 0, io.EOF
	}
	if a.failed && a.latePrompt {
		a.latePrompt = false
		<-ctx.Done()
		a.pending = []byte("410C0000\r>")
		return 0, ctx.Err()
	}
	<-ctx.Done()
	return 0, ctx.Err()
}
func (a *adapter) Close() error { a.closed = true; return nil }

func runFixture(t *testing.T, scan bool) (Report, *adapter) {
	t.Helper()
	a := fixtureAdapter(t)
	r, err := New(Config{ScanSupported: scan}, func(context.Context) (elm327.Transport, error) { return a, nil })
	if err != nil {
		t.Fatal(err)
	}
	report, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return report, a
}

func capability(t *testing.T, r Report, pid byte) Capability {
	t.Helper()
	for _, c := range r.Capabilities {
		if c.PID == elm327.HexPID(pid) {
			return c
		}
	}
	t.Fatalf("no PID %02X", pid)
	return Capability{}
}

func TestFixtureBaselineAndCapabilities(t *testing.T) {
	t.Parallel()
	report, a := runFixture(t, false)
	if report.Protocol == nil || report.Protocol.Code != "A5" || report.StoredDTCs.Status != "available" || len(report.StoredDTCs.Codes) != 0 {
		t.Fatalf("bad fixture report: %+v", report)
	}
	if c := capability(t, report, 5); c.Value == nil || *c.Value != 78 || c.Status != "available" {
		t.Fatalf("coolant: %+v", c)
	}
	if c := capability(t, report, 0x10); c.Support != "unsupported" || c.Value != nil {
		t.Fatalf("MAF: %+v", c)
	}
	if c := capability(t, report, 0x2F); c.Support != "not_yet_tested" {
		t.Fatalf("fuel level guessed: %+v", c)
	}
	for _, blocked := range []string{"0110", "012F", "04", "0902", "0120"} {
		if slices.Contains(a.commands, blocked) {
			t.Fatalf("unexpected command %s", blocked)
		}
	}
	if a.deadlines["0100"] < 29*time.Second || a.deadlines["0100"] > 31*time.Second {
		t.Fatalf("initial deadline: %s", a.deadlines["0100"])
	}
	if !a.closed || len(report.Commands) != len(a.commands) {
		t.Fatal("source not closed or command audit incomplete")
	}
}

func TestContinuationAndNoDataAreNotUnsupported(t *testing.T) {
	t.Parallel()
	report, a := runFixture(t, true)
	if !slices.Contains(a.commands, "0120") || slices.Contains(a.commands, "0140") || slices.Contains(a.commands, "0110") {
		t.Fatalf("wrong discovery: %v", a.commands)
	}
	if c := capability(t, report, 0x2F); c.Support != "not_yet_tested" {
		t.Fatal("unknown continuation treated as unsupported")
	}
	if c := capability(t, report, 0x0B); c.Support != "supported" || c.Status != "temporarily_unavailable" || c.Value != nil {
		t.Fatalf("NO DATA misclassified: %+v", c)
	}
	if report.Status != "partial" {
		t.Fatalf("scan must be partial: %s", report.Status)
	}
}

func TestTimeoutRecoveryReinitializes(t *testing.T) {
	t.Parallel()
	a := fixtureAdapter(t)
	a.failCommand = "010C"
	a.latePrompt = true
	r, err := New(Config{Timeout: 5 * time.Millisecond, Retries: 1, RecoveryTimeout: time.Second}, func(context.Context) (elm327.Transport, error) { return a, nil })
	if err != nil {
		t.Fatal(err)
	}
	r.wait = func(context.Context, time.Duration) error { return nil }
	report, err := r.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if report.Attempts != 2 || report.Status != "complete" {
		t.Fatalf("recovery: %+v", report)
	}
	count := 0
	for _, c := range a.commands {
		if c == "ATZ" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("did not reinitialize: %v", a.commands)
	}
	recoveries := 0
	for _, c := range report.Commands {
		if c.Action == "recovery" {
			recoveries++
			if !strings.Contains(c.RawResponse, ">") {
				t.Fatal("missing drained response")
			}
		}
	}
	if recoveries != 1 {
		t.Fatalf("recovery audit: %d", recoveries)
	}
}

func TestDisconnectReopensWithPromptAndFiniteRetries(t *testing.T) {
	t.Parallel()
	first := fixtureAdapter(t)
	first.failCommand = "010C"
	first.disconnect = true
	second := fixtureAdapter(t)
	second.pending = []byte(">")
	opens := 0
	r, err := New(Config{Retries: 1}, func(context.Context) (elm327.Transport, error) {
		opens++
		if opens == 1 {
			return first, nil
		}
		return second, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	r.wait = func(context.Context, time.Duration) error { return nil }
	report, err := r.Run(t.Context())
	if err != nil || report.Status != "complete" || opens != 2 || !first.closed || !second.closed {
		t.Fatalf("reconnect: %d %s %v", opens, report.Status, err)
	}
	opens = 0
	r, err = New(Config{Retries: 2}, func(context.Context) (elm327.Transport, error) { opens++; return nil, os.ErrNotExist })
	if err != nil {
		t.Fatal(err)
	}
	r.wait = func(context.Context, time.Duration) error { return nil }
	report, err = r.Run(t.Context())
	if !errors.Is(err, os.ErrNotExist) || opens != 3 || report.Status != "failed" {
		t.Fatalf("retry budget: %d %s %v", opens, report.Status, err)
	}
}
