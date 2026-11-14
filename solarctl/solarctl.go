// Package solarctl decides the speed of the solar collector pump.
//
// The system is a drainback loop: the pump lifts water from the bottom of an
// open (atmospheric) tank to evacuated-tube collectors on the roof, and it
// falls back into the tank above the water surface. When the pump stops the
// collectors drain, so stopping is always safe and is what the controller does
// whenever it is unsure.
//
// The controller is a state machine:
//
//	Off      pump stopped. Starts when the collector is StartDelta above the
//	         tank bottom (where the pump draws from), the pump has been off for
//	         MinOff, the tank is not full and the collector is not over
//	         CollectorMax. The collector sensor is only compared with the tank
//	         while the pump is off: with water flowing it reads close to the
//	         inlet, so the rise across the collectors is used instead.
//	Priming  100% for PrimeTime to lift the water to the roof.
//	Running  a PI loop sets the speed so the outlet is TargetRise above the
//	         inlet: more rise than the target speeds the pump up, less slows
//	         it down, never below MinSpeed (the slowest speed that still lifts
//	         water to the roof). Stops after MinOn once the rise has been
//	         below NoRise for StopAfter, so it never pumps hot tank water
//	         through collectors that are cooling it.
//
// Whatever the state, the pump stops at once if the tank reaches TankMax
// (until it cools to TankResume) or the temperatures are stale or implausible.
package solarctl

import (
	"fmt"
	"math"
	"time"
)

// Config holds the controller settings. Temperatures are in degrees C and
// speeds in percent.
type Config struct {
	StartDelta float64       // collector above the tank bottom needed to start
	StopAfter  time.Duration // the rise must stay below NoRise this long to stop
	MinOn      time.Duration // shortest run once started
	MinOff     time.Duration // shortest rest before starting again
	PrimeTime  time.Duration // time at 100% to fill the collectors

	TargetRise float64       // wanted outlet minus inlet while running
	NoRise     float64       // a rise below this counts as nothing collected
	MinSpeed   float64       // slowest speed that still lifts water to the roof
	Kp         float64       // % speed per degree of rise error
	Ti         time.Duration // integral time
	MaxStep    float64       // largest speed change per update, %

	TankMax      float64 // stop collecting at this tank temperature (hottest sensor)
	TankResume   float64 // allow collecting again once the hottest sensor is below this
	CollectorMax float64 // don't start (refill hot, dry collectors) above this
	CollectorOK  float64 // ... until the collector has cooled below this

	MaxAge time.Duration // temperatures older than this stop the pump
}

// DefaultConfig returns settings suited to the 3750 l open drainback tank.
func DefaultConfig() Config {
	return Config{
		StartDelta: 8,
		StopAfter:  3 * time.Minute,
		MinOn:      5 * time.Minute,
		MinOff:     5 * time.Minute,
		PrimeTime:  30 * time.Second,

		TargetRise: 5,
		NoRise:     1,
		MinSpeed:   40,
		Kp:         4,
		Ti:         5 * time.Minute,
		MaxStep:    5,

		TankMax:      92,
		TankResume:   88,
		CollectorMax: 130,
		CollectorOK:  120,

		MaxAge: time.Minute,
	}
}

// Inputs is one set of temperature readings.
type Inputs struct {
	Time    time.Time // when the controller is stepped
	Updated time.Time // when the temperatures were last read successfully

	Collector, Inlet, Outlet        float64
	TankTop, TankMiddle, TankBottom float64
}

// State is the controller state.
type State int

const (
	Off State = iota
	Priming
	Running
)

func (s State) String() string {
	switch s {
	case Priming:
		return "Priming"
	case Running:
		return "Running"
	}
	return "Off"
}

// Controller holds the state between updates. Create it with New.
type Controller struct {
	cfg Config

	state     State
	since     time.Time // when the current state began
	speed     float64
	lastErr   float64
	lastTime  time.Time
	stopSince time.Time // when a stop condition started to hold; zero if not holding

	tankFull     bool
	collectorHot bool

	// Reason explains the current state, e.g. "tank full" or "collector 6.2 °C above tank".
	Reason string
}

// New returns a controller in the Off state, allowed to start straight away.
func New(cfg Config) *Controller {
	return &Controller{cfg: cfg, Reason: "starting up"}
}

// State returns the current state.
func (c *Controller) State() State { return c.state }

// Speed returns the current pump speed in percent; 0 when off.
func (c *Controller) Speed() float64 { return c.speed }

func plausible(v float64) bool { return v > -40 && v < 200 && !math.IsNaN(v) }

// Step takes new readings and returns the pump speed to use, 0 to 100.
func (c *Controller) Step(in Inputs) uint8 {
	cfg := c.cfg
	now := in.Time
	dt := now.Sub(c.lastTime)
	if c.lastTime.IsZero() || dt <= 0 {
		dt = 0
	}
	c.lastTime = now

	// Safety checks that stop the pump in any state.
	for _, v := range []float64{in.Collector, in.Inlet, in.Outlet, in.TankTop, in.TankMiddle, in.TankBottom} {
		if !plausible(v) {
			c.stop(now, "temperature reading out of range")
			return 0
		}
	}
	if in.Updated.IsZero() || now.Sub(in.Updated) > cfg.MaxAge {
		c.stop(now, "temperature readings are stale")
		return 0
	}
	tankHot := math.Max(in.TankTop, math.Max(in.TankMiddle, in.TankBottom))
	if tankHot >= cfg.TankMax {
		c.tankFull = true
	} else if tankHot < cfg.TankResume {
		c.tankFull = false
	}
	if c.tankFull {
		c.stop(now, fmt.Sprintf("tank full (%.1f °C)", tankHot))
		return 0
	}
	lift := in.Collector - in.TankBottom // how much hotter the collector is than the water we would send it

	switch c.state {
	case Off:
		if in.Collector >= cfg.CollectorMax {
			c.collectorHot = true
		} else if in.Collector < cfg.CollectorOK {
			c.collectorHot = false
		}
		switch {
		case c.collectorHot:
			c.Reason = fmt.Sprintf("collector too hot to refill (%.1f °C)", in.Collector)
		case lift < cfg.StartDelta:
			c.Reason = fmt.Sprintf("collector %.1f °C above tank", lift)
		case !c.since.IsZero() && now.Sub(c.since) < cfg.MinOff:
			c.Reason = "waiting after last run"
		default:
			c.state, c.since, c.speed = Priming, now, 100
			c.Reason = fmt.Sprintf("collector %.1f °C above tank", lift)
		}

	case Priming:
		c.speed = 100
		if now.Sub(c.since) >= cfg.PrimeTime {
			c.state = Running
			c.lastErr = (in.Outlet - in.Inlet) - cfg.TargetRise
			c.stopSince = time.Time{}
			c.Reason = "primed"
		}

	case Running:
		rise := in.Outlet - in.Inlet
		e := rise - cfg.TargetRise // positive: more rise than wanted, so speed up
		if dt > 0 {
			// Velocity-form PI: change the speed rather than compute it, which
			// keeps it smooth and stops the integral winding up at the limits.
			step := cfg.Kp*(e-c.lastErr) + cfg.Kp*e*dt.Seconds()/cfg.Ti.Seconds()
			step = math.Max(-cfg.MaxStep, math.Min(cfg.MaxStep, step))
			c.speed = math.Max(cfg.MinSpeed, math.Min(100, c.speed+step))
		}
		c.lastErr = e

		var why string
		if rise < cfg.NoRise {
			why = fmt.Sprintf("only %.1f °C rise", rise)
		}
		if why == "" {
			c.stopSince = time.Time{}
			c.Reason = fmt.Sprintf("rise %.1f °C, target %.1f °C", rise, cfg.TargetRise)
			break
		}
		if c.stopSince.IsZero() {
			c.stopSince = now
		}
		c.Reason = why
		if now.Sub(c.stopSince) >= cfg.StopAfter && now.Sub(c.since) >= cfg.MinOn {
			c.stop(now, why)
		}
	}
	return uint8(math.Round(c.speed))
}

// stop turns the pump off, recording when so MinOff can be enforced.
func (c *Controller) stop(now time.Time, why string) {
	if c.state != Off {
		c.since = now
	}
	c.state, c.speed, c.stopSince = Off, 0, time.Time{}
	c.Reason = why
}
