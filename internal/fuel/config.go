// Package fuel estimates gasoline consumption and maintains local trip accounting.
package fuel

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"
)

type Config struct {
	Model              string        `json:"model"`
	Displacement       float64       `json:"displacement_l"`
	VE                 float64       `json:"ve"`
	AFR                float64       `json:"afr"`
	Density            float64       `json:"density_g_l"`
	LowSpeed           float64       `json:"low_speed_kmh"`
	MaxFlow            float64       `json:"max_flow_lph"`
	MaxMAF             float64       `json:"max_maf_g_s"`
	AllowUnverifiedIAT bool          `json:"allow_unverified_iat"`
	StaleAfter         time.Duration `json:"global_stale_ns"`
}

func DefaultConfig() Config {
	return Config{Model: "auto", Displacement: 1.5, VE: .75, AFR: 14.7, Density: 745, LowSpeed: 5, MaxFlow: 100, MaxMAF: 200, StaleAfter: 3 * time.Second}
}

func finite(v float64) bool          { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func bounded(v, lo, hi float64) bool { return finite(v) && v >= lo && v <= hi }

func (c Config) Validate() error {
	if !slices.Contains([]string{"auto", "direct", "maf", "speed_density"}, c.Model) {
		return fmt.Errorf("invalid fuel model")
	}
	for _, x := range []struct{ v, lo, hi float64 }{{c.Displacement, .5, 8}, {c.VE, .1, 1.2}, {c.AFR, 10, 20}, {c.Density, 600, 900}, {c.LowSpeed, 1, 30}, {c.MaxFlow, 1, 200}, {c.MaxMAF, 1, 500}} {
		if !bounded(x.v, x.lo, x.hi) {
			return fmt.Errorf("fuel setting outside accepted bounds")
		}
	}
	if c.StaleAfter <= 0 {
		return fmt.Errorf("fuel freshness must be positive")
	}
	return nil
}

func (c Config) fingerprint() string {
	b, _ := json.Marshal(c)
	return fmt.Sprintf("%x", sha256.Sum256(b))
}
