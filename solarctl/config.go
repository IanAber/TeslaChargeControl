package solarctl

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

// configJSON is how a Config is stored in the config file and sent to the
// web page: durations are whole or fractional seconds.
type configJSON struct {
	StartDelta   float64 `json:"startDelta"`
	StopAfter    float64 `json:"stopAfterSec"`
	MinOn        float64 `json:"minOnSec"`
	MinOff       float64 `json:"minOffSec"`
	PrimeTime    float64 `json:"primeTimeSec"`
	TargetRise   float64 `json:"targetRise"`
	NoRise       float64 `json:"noRise"`
	MinSpeed     float64 `json:"minSpeed"`
	Kp           float64 `json:"kp"`
	Ti           float64 `json:"tiSec"`
	MaxStep      float64 `json:"maxStep"`
	TankMax      float64 `json:"tankMax"`
	TankResume   float64 `json:"tankResume"`
	CollectorMax float64 `json:"collectorMax"`
	CollectorOK  float64 `json:"collectorOK"`
	MaxAge       float64 `json:"maxAgeSec"`
}

func secs(d time.Duration) float64 { return d.Seconds() }
func dur(s float64) time.Duration  { return time.Duration(math.Round(s * float64(time.Second))) }

func (c Config) toJSON() configJSON {
	return configJSON{
		StartDelta: c.StartDelta, StopAfter: secs(c.StopAfter), MinOn: secs(c.MinOn), MinOff: secs(c.MinOff),
		PrimeTime: secs(c.PrimeTime), TargetRise: c.TargetRise, NoRise: c.NoRise, MinSpeed: c.MinSpeed,
		Kp: c.Kp, Ti: secs(c.Ti), MaxStep: c.MaxStep, TankMax: c.TankMax, TankResume: c.TankResume,
		CollectorMax: c.CollectorMax, CollectorOK: c.CollectorOK, MaxAge: secs(c.MaxAge),
	}
}

func (j configJSON) config() Config {
	return Config{
		StartDelta: j.StartDelta, StopAfter: dur(j.StopAfter), MinOn: dur(j.MinOn), MinOff: dur(j.MinOff),
		PrimeTime: dur(j.PrimeTime), TargetRise: j.TargetRise, NoRise: j.NoRise, MinSpeed: j.MinSpeed,
		Kp: j.Kp, Ti: dur(j.Ti), MaxStep: j.MaxStep, TankMax: j.TankMax, TankResume: j.TankResume,
		CollectorMax: j.CollectorMax, CollectorOK: j.CollectorOK, MaxAge: dur(j.MaxAge),
	}
}

// MarshalJSON writes the config with durations in seconds.
func (c Config) MarshalJSON() ([]byte, error) { return json.Marshal(c.toJSON()) }

// UnmarshalJSON reads a config written by MarshalJSON. Fields missing from
// the JSON keep the value already in c, so unmarshal onto DefaultConfig() to
// fill gaps with defaults.
func (c *Config) UnmarshalJSON(b []byte) error {
	j := c.toJSON()
	if err := json.Unmarshal(b, &j); err != nil {
		return err
	}
	*c = j.config()
	return nil
}

// Validate reports settings the controller cannot work with.
func (c Config) Validate() error {
	var errs []error
	check := func(ok bool, format string, a ...any) {
		if !ok {
			errs = append(errs, fmt.Errorf(format, a...))
		}
	}
	temp := func(name string, v float64) {
		check(v > -40 && v < 200, "%s must be between -40 and 200 °C", name)
	}
	check(c.StartDelta > 0, "start difference must be above 0 °C")
	check(c.StopAfter >= 0, "stop delay cannot be negative")
	check(c.MinOn >= 0, "minimum run time cannot be negative")
	check(c.MinOff >= 0, "minimum rest time cannot be negative")
	check(c.PrimeTime >= 0, "priming time cannot be negative")
	check(c.TargetRise > 0, "target rise must be above 0 °C")
	check(c.NoRise >= 0 && c.NoRise < c.TargetRise, "no-rise threshold must be at least 0 and below the target rise")
	check(c.MinSpeed >= 0 && c.MinSpeed <= 100, "minimum speed must be 0 to 100 %%")
	check(c.Kp >= 0, "proportional gain cannot be negative")
	check(c.Ti >= time.Second, "integral time must be at least 1 s")
	check(c.MaxStep > 0 && c.MaxStep <= 100, "largest speed step must be above 0 and at most 100 %%")
	temp("tank maximum", c.TankMax)
	temp("tank resume", c.TankResume)
	temp("collector maximum", c.CollectorMax)
	temp("collector refill", c.CollectorOK)
	check(c.TankResume < c.TankMax, "tank resume must be below the tank maximum")
	check(c.CollectorOK < c.CollectorMax, "collector refill temperature must be below the collector maximum")
	check(c.MaxAge >= time.Second, "maximum reading age must be at least 1 s")
	return errors.Join(errs...)
}

// Config returns the settings in use.
func (c *Controller) Config() Config { return c.cfg }

// SetConfig changes the settings without stopping the pump. The PI loop
// carries on from the current speed; a new target rise does not cause a
// sudden jump, the integral moves the speed towards it. Call it from the
// goroutine that calls Step.
func (c *Controller) SetConfig(cfg Config) {
	// Re-express the last error against the new target so the proportional
	// term does not kick on the next step.
	c.lastErr += c.cfg.TargetRise - cfg.TargetRise
	c.cfg = cfg
	if c.state == Running {
		c.speed = math.Max(cfg.MinSpeed, math.Min(100, c.speed))
	}
}

// DecodeConfig reads JSON settings over base, rejecting unknown fields so a
// misspelt setting is not silently ignored. It does not validate the result.
func DecodeConfig(b []byte, base Config) (Config, error) {
	j := base.toJSON()
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&j); err != nil {
		return base, err
	}
	return j.config(), nil
}
