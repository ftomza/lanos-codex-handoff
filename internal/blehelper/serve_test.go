package blehelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
)

type fakeConnection struct {
	closed   bool
	write    func([]byte) (int, error)
	closeErr error
}

func (f *fakeConnection) Write(ctx context.Context, b []byte) (int, error) { return f.write(b) }
func (f *fakeConnection) Close() error                                     { f.closed = true; return f.closeErr }

func TestHelperPreservesBytesAndAcknowledgements(t *testing.T) {
	t.Parallel()
	var input, output bytes.Buffer
	encoder := json.NewEncoder(&input)
	for _, m := range []blewire.Message{{Type: "write", ID: 1, Data: []byte("ATI\r")}, {Type: "close"}} {
		if err := encoder.Encode(m); err != nil {
			t.Fatal(err)
		}
	}
	f := &fakeConnection{}
	err := Serve(t.Context(), "IOS-Vlink", &input, &output, func(ctx context.Context, name string, notify func([]byte), lost func(error)) (Connection, error) {
		if name != "IOS-Vlink" {
			t.Fatal(name)
		}
		f.write = func(b []byte) (int, error) {
			if string(b) != "ATI\r" {
				t.Fatal(string(b))
			}
			notify([]byte("ELM327 "))
			notify([]byte("v2.3\r>"))
			return len(b), nil
		}
		return f, nil
	})
	if err != nil || !f.closed {
		t.Fatalf("serve: %v closed=%t", err, f.closed)
	}
	var messages []blewire.Message
	decoder := json.NewDecoder(&output)
	for {
		var m blewire.Message
		if err := decoder.Decode(&m); errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, m)
	}
	if len(messages) != 4 || messages[0].Type != "ready" || messages[0].Version != 1 || messages[3].Type != "ack" || messages[3].ID != 1 {
		t.Fatalf("events: %+v", messages)
	}
	if string(append(messages[1].Data, messages[2].Data...)) != "ELM327 v2.3\r>" {
		t.Fatal("notification data changed")
	}
}

func TestHelperFailuresAndCleanup(t *testing.T) {
	t.Parallel()
	for _, input := range []string{`{"type":"write","id":0,"data":"QVRJDQ=="}`, `{"type":"write","id":1}`, `{"type":"unknown"}`, strings.Repeat("x", blewire.MaxLine+1)} {
		t.Run(input[:min(len(input), 40)], func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			f := &fakeConnection{write: func([]byte) (int, error) { return 0, errors.New("write failed") }}
			err := Serve(t.Context(), "IOS-Vlink", strings.NewReader(input+"\n"), &output, func(context.Context, string, func([]byte), func(error)) (Connection, error) { return f, nil })
			if err == nil || !f.closed || !strings.Contains(output.String(), `"error"`) {
				t.Fatalf("failure: %v closed=%t output=%s", err, f.closed, output.String())
			}
		})
	}
}

func TestHelperOpenFailure(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	cause := errors.New("Bluetooth access denied")
	err := Serve(t.Context(), "IOS-Vlink", strings.NewReader(""), &output, func(context.Context, string, func([]byte), func(error)) (Connection, error) { return nil, cause })
	if !errors.Is(err, cause) || strings.Contains(output.String(), `"ready"`) || !strings.Contains(output.String(), "Bluetooth access denied") {
		t.Fatalf("startup: %v %s", err, output.String())
	}
}

func TestHelperSeparatesTransientDisconnectFromCleanupFailure(t *testing.T) {
	t.Parallel()
	for _, cleanup := range []bool{false, true} {
		t.Run(fmt.Sprint(cleanup), func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			f := &fakeConnection{}
			if cleanup {
				f.closeErr = errors.New("unsubscribe failed")
			}
			err := Serve(t.Context(), "IOS-Vlink", strings.NewReader(""), &output, func(ctx context.Context, _ string, _ func([]byte), lost func(error)) (Connection, error) {
				lost(blewire.ErrDisconnected)
				return f, nil
			})
			if (err != nil) != cleanup || !f.closed {
				t.Fatalf("cleanup=%t err=%v closed=%t", cleanup, err, f.closed)
			}
			if strings.Contains(output.String(), `"code":"disconnected"`) == cleanup {
				t.Fatalf("wrong transient classification: %s", output.String())
			}
			if cleanup && !strings.Contains(output.String(), "unsubscribe failed") {
				t.Fatal("cleanup evidence lost")
			}
		})
	}
}

func TestHelperReportsUnavailableWithoutFatalExit(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	err := Serve(t.Context(), "IOS-Vlink", strings.NewReader(""), &output, func(context.Context, string, func([]byte), func(error)) (Connection, error) {
		return nil, blewire.ErrUnavailable
	})
	if err != nil || !strings.Contains(output.String(), `"code":"unavailable"`) {
		t.Fatalf("unavailable: %v output=%s", err, output.String())
	}
}

func TestHelperJoinedFatalOpenErrorIsNotClassifiedTransient(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	cause := errors.New("native cleanup failed")
	err := Serve(t.Context(), "IOS-Vlink", strings.NewReader(""), &output, func(context.Context, string, func([]byte), func(error)) (Connection, error) {
		return nil, errors.Join(blewire.ErrUnavailable, cause)
	})
	if !errors.Is(err, cause) || strings.Contains(output.String(), `"code":"unavailable"`) {
		t.Fatalf("joined fatal error hidden: %v %s", err, output.String())
	}
}
