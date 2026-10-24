package main

import (
	"bytes"
	"context"
	"testing"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/probe"
)

func TestAdapterModelOverrideAndUnknown(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"another adapter", ""} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			var out, stderr bytes.Buffer
			called := false
			err := run(t.Context(), []string{"--port=/dev/cu.fake", "--json", "--adapter-model=" + model}, &out, &stderr, dependencies{
				probe: func(ctx context.Context, cfg probe.Config) (probe.Report, error) {
					called = true
					if cfg.AdapterModel != model {
						t.Fatalf("model override: %q, want %q", cfg.AdapterModel, model)
					}
					return probe.Report{}, nil
				},
			})
			if err != nil || !called {
				t.Fatalf("run: %v, called=%t", err, called)
			}
		})
	}
}
