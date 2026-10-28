package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestHelpAndInvalidArgsNeverNeedHardware(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	if err := run(context.Background(), []string{"--help"}, &out); err != nil || !strings.Contains(out.String(), "input") {
		t.Fatal(err, out.String())
	}
	for _, args := range [][]string{{}, {"--input=x", "unexpected"}, {"--input=x", "--session=../escape"}, {"--input=x", "--from=bad"}} {
		out.Reset()
		if err := run(t.Context(), args, &out); err == nil {
			t.Fatal("accepted", args)
		}
	}
}
