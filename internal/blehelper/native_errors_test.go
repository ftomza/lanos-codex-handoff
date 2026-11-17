package blehelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/blewire"
)

func TestNativeTimeoutMappingIsOperationSpecific(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, operation string
		cause           error
		timeout         bool
	}{
		{"write", "write", errors.New("timeout on Write()"), true},
		{"subscribe", "subscribe", errors.New("timeout on EnableNotifications"), true},
		{"unsubscribe", "unsubscribe", errors.New("timeout on EnableNotifications"), true},
		{"wrong operation", "write", errors.New("timeout on EnableNotifications"), false},
		{"permission", "write", os.ErrPermission, false},
		{"lookalike", "write", errors.New("permission denied: timeout on Write()"), false},
		{"wrapped", "write", fmt.Errorf("other error: %w", errors.New("timeout on Write()")), false},
		{"unknown", "write", errors.New("timeout on Read()"), false},
		{"nil", "write", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := nativeError(tc.operation, tc.cause)
			if errors.Is(err, blewire.ErrTimeout) != tc.timeout {
				t.Fatalf("wrong native timeout classification: %v", err)
			}
			if tc.timeout && !strings.Contains(err.Error(), tc.cause.Error()) {
				t.Fatalf("lost native evidence: %v", err)
			}
			if !tc.timeout && !errors.Is(err, tc.cause) {
				t.Fatalf("changed unrecognized cause: %v", err)
			}
		})
	}
}

func TestHelperNativeTimeoutCompletion(t *testing.T) {
	t.Parallel()
	writeTimeout := fmt.Errorf("%w: timeout on Write()", blewire.ErrTimeout)
	notifyTimeout := fmt.Errorf("%w: timeout on EnableNotifications", blewire.ErrTimeout)
	for _, tc := range []struct {
		name           string
		primary, close error
		retry          bool
	}{
		{"clean timeout", writeTimeout, nil, true},
		{"timeout with unsubscribe timeout", writeTimeout, notifyTimeout, true},
		{"lost callback and timeout", errors.Join(blewire.ErrDisconnected, writeTimeout), notifyTimeout, true},
		{"unknown cleanup", writeTimeout, errors.New("unsubscribe failed"), false},
		{"permission cleanup", writeTimeout, os.ErrPermission, false},
		{"mixed cleanup", writeTimeout, errors.Join(notifyTimeout, blewire.ErrDisconnected), false},
		{"unknown primary", errors.New("bad IPC"), notifyTimeout, false},
		{"shutdown cleanup timeout", nil, notifyTimeout, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			input := ""
			if tc.primary != nil {
				input = "{\"type\":\"write\",\"id\":1,\"data\":\"QVRJDQ==\"}\n"
			}
			f := &fakeConnection{write: func([]byte) (int, error) { return 0, tc.primary }, closeErr: tc.close}
			err := Serve(t.Context(), "IOS-Vlink", strings.NewReader(input), &output, func(context.Context, string, func([]byte), func(error)) (Connection, error) { return f, nil })
			if (err == nil) != tc.retry || !f.closed {
				t.Fatalf("retry=%t err=%v closed=%t output=%s", tc.retry, err, f.closed, output.String())
			}
			decoder := json.NewDecoder(&output)
			var final blewire.Message
			for {
				var m blewire.Message
				decodeErr := decoder.Decode(&m)
				if errors.Is(decodeErr, io.EOF) {
					break
				}
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				if m.Type == "ack" {
					t.Fatal("failed write acknowledged before final cleanup outcome")
				}
				if m.Type == "error" {
					final = m
				}
			}
			if final.Type != "error" || (final.Code == "timeout") != tc.retry {
				t.Fatalf("wrong final timeout code: %+v", final)
			}
			for _, cause := range []error{tc.primary, tc.close} {
				if cause != nil && !strings.Contains(final.Error, cause.Error()) {
					t.Fatalf("lost cause %v: %+v", cause, final)
				}
			}
		})
	}
}

type failNativeOutput struct{ writes int }

func (w *failNativeOutput) Write(data []byte) (int, error) {
	w.writes++
	if w.writes > 1 {
		return 0, os.ErrPermission
	}
	return len(data), nil
}

func TestHelperNativeTimeoutDoesNotHideOutputFailure(t *testing.T) {
	t.Parallel()
	f := &fakeConnection{write: func([]byte) (int, error) { return 0, blewire.ErrTimeout }}
	err := Serve(t.Context(), "IOS-Vlink", strings.NewReader("{\"type\":\"write\",\"id\":1,\"data\":\"QVRJDQ==\"}\n"), &failNativeOutput{}, func(context.Context, string, func([]byte), func(error)) (Connection, error) { return f, nil })
	if !errors.Is(err, os.ErrPermission) || !f.closed {
		t.Fatalf("output/cleanup failure hidden: %v closed=%t", err, f.closed)
	}
}
