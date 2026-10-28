package main

import (
	"flag"
	"fmt"
	"path/filepath"

	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/fuel"
	"gitlab.ozon.ru/ftomza/lanos-dashboard/internal/telemetry"
)

func registerFuelFlags(f *flag.FlagSet, o *dashboardOptions) {
	o.fuelConfig = fuel.DefaultConfig()
	c := &o.fuelConfig
	f.StringVar(&o.fuelMode, "fuel", "auto", "fuel computer: auto or off; live speed-density requires explicit IAT opt-in")
	f.StringVar(&o.fuelState, "fuel-state", "", "private trip JSON; default data/fuel/{sim,live}.json")
	f.BoolVar(&o.fuelReset, "fuel-reset-state", false, "explicitly start a new local trip and factor 1, including config/source changes")
	f.StringVar(&c.Model, "fuel-model", c.Model, "fuel model: auto, direct, maf or speed_density")
	f.Float64Var(&c.Displacement, "fuel-displacement", c.Displacement, "engine displacement in liters")
	f.Float64Var(&c.VE, "fuel-ve", c.VE, "assumed volumetric efficiency; not a measured VE map")
	f.Float64Var(&c.AFR, "fuel-afr", c.AFR, "assumed gasoline air/fuel mass ratio")
	f.Float64Var(&c.Density, "fuel-density", c.Density, "nominal gasoline density g/L")
	f.Float64Var(&c.LowSpeed, "fuel-low-speed", c.LowSpeed, "minimum reliable speed for instantaneous L/100 km")
	f.BoolVar(&c.AllowUnverifiedIAT, "fuel-allow-unverified-iat", false, "explicit low-confidence live estimate from actual unverified IAT; no temperature substitution")
}
func validateFuelFlags(o *dashboardOptions) error {
	if o.fuelMode != "auto" && o.fuelMode != "off" {
		return fmt.Errorf("fuel must be auto or off")
	}
	if o.fuelMode == "off" && o.fuelReset {
		return fmt.Errorf("fuel-reset-state requires enabled fuel computer")
	}
	o.fuelConfig.StaleAfter = o.staleAfter
	return o.fuelConfig.Validate()
}
func openFuel(o dashboardOptions, source string) (*fuel.Computer, error) {
	if o.fuelMode == "off" {
		return nil, nil
	}
	path := o.fuelState
	if path == "" {
		name := "sim.json"
		if source == telemetry.Live {
			name = "live.json"
		}
		path = filepath.Join("data", "fuel", name)
	}
	return fuel.Open(o.fuelConfig, source, path, o.fuelReset)
}
func closeFuel(c *fuel.Computer) error {
	if c == nil {
		return nil
	}
	return c.Close()
}
