package solarctl

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)

// sim steps a controller every 5 s with readings from f.
type sim struct {
	t   *testing.T
	c   *Controller
	now time.Time
}

func newSim(t *testing.T) *sim {
	return &sim{t: t, c: New(DefaultConfig()), now: t0}
}

func (s *sim) run(d time.Duration, f func(speed uint8) Inputs) uint8 {
	var speed uint8
	for end := s.now.Add(d); s.now.Before(end); s.now = s.now.Add(5 * time.Second) {
		in := f(speed)
		in.Time = s.now
		if in.Updated.IsZero() {
			in.Updated = s.now
		}
		speed = s.c.Step(in)
	}
	return speed
}

// readings with the tank evenly at tank °C.
func readings(collector, inlet, outlet, tank float64) Inputs {
	return Inputs{Collector: collector, Inlet: inlet, Outlet: outlet, TankTop: tank, TankMiddle: tank, TankBottom: tank}
}

func TestStaysOffWhenCollectorNotHotEnough(t *testing.T) {
	s := newSim(t)
	if sp := s.run(10*time.Minute, func(uint8) Inputs { return readings(57, 50, 50, 50) }); sp != 0 || s.c.State() != Off {
		t.Fatalf("speed %d state %v, want off", sp, s.c.State())
	}
}

func TestPrimesAtFullSpeedThenRuns(t *testing.T) {
	s := newSim(t)
	sp := s.run(5*time.Second, func(uint8) Inputs { return readings(70, 50, 50, 50) })
	if sp != 100 || s.c.State() != Priming {
		t.Fatalf("after start: speed %d state %v, want 100 Priming", sp, s.c.State())
	}
	s.run(25*time.Second, func(uint8) Inputs { return readings(70, 50, 55, 50) })
	if s.c.State() != Priming {
		t.Fatalf("at 30 s: state %v, want still Priming", s.c.State())
	}
	s.run(10*time.Second, func(uint8) Inputs { return readings(70, 50, 55, 50) })
	if s.c.State() != Running {
		t.Fatalf("after priming: state %v, want Running", s.c.State())
	}
}

func TestSlowsDownWhenRiseBelowTargetButNotBelowMinimum(t *testing.T) {
	s := newSim(t)
	sp := s.run(30*time.Minute, func(uint8) Inputs { return readings(70, 50, 52, 50) }) // 2 °C rise, target 5
	if sp != uint8(DefaultConfig().MinSpeed) {
		t.Fatalf("speed %d, want minimum %v", sp, DefaultConfig().MinSpeed)
	}
	if s.c.State() != Running {
		t.Fatalf("state %v, want Running (2 °C rise is still collecting)", s.c.State())
	}
}

func TestSpeedsUpWhenRiseAboveTarget(t *testing.T) {
	s := newSim(t)
	s.run(30*time.Minute, func(uint8) Inputs { return readings(70, 50, 52, 50) }) // settle at minimum
	sp := s.run(10*time.Minute, func(uint8) Inputs { return readings(80, 50, 60, 50) })
	if sp < 80 {
		t.Fatalf("speed %d after 10 min of a 10 °C rise, want at least 80", sp)
	}
	if sp = s.run(10*time.Minute, func(uint8) Inputs { return readings(80, 50, 60, 50) }); sp != 100 {
		t.Fatalf("speed %d after 20 min, want 100", sp)
	}
}

func TestSettlesWithSimplePlant(t *testing.T) {
	// Rise falls as flow rises: rise = 300 / speed, so the target 5 °C needs 60%.
	s := newSim(t)
	sp := s.run(time.Hour, func(speed uint8) Inputs {
		rise := 0.0
		if speed > 0 {
			rise = 300 / float64(speed)
		}
		return readings(75, 50, 50+rise, 50)
	})
	if sp < 57 || sp > 63 {
		t.Fatalf("speed %d, want about 60", sp)
	}
}

func TestStopsAtSunsetAfterHold(t *testing.T) {
	s := newSim(t)
	s.run(10*time.Minute, func(uint8) Inputs { return readings(70, 50, 55, 50) })
	s.run(2*time.Minute, func(uint8) Inputs { return readings(52, 50, 50.5, 50) })
	if s.c.State() != Running {
		t.Fatalf("after 2 min of no rise: %v, want still Running (StopAfter is 3 min)", s.c.State())
	}
	if sp := s.run(2*time.Minute, func(uint8) Inputs { return readings(52, 50, 50.5, 50) }); sp != 0 || s.c.State() != Off {
		t.Fatalf("after 4 min: speed %d state %v, want off", sp, s.c.State())
	}
}

func TestRespectsMinimumOnAndOffTimes(t *testing.T) {
	s := newSim(t)
	s.run(30*time.Second, func(uint8) Inputs { return readings(70, 50, 55, 50) })
	// The rise disappears straight away, but the pump must run for MinOn (5 min).
	s.run(4*time.Minute, func(uint8) Inputs { return readings(51, 50, 50, 50) })
	if s.c.State() == Off {
		t.Fatal("stopped before MinOn")
	}
	s.run(35*time.Second, func(uint8) Inputs { return readings(51, 50, 50, 50) }) // to 5:05
	if s.c.State() != Off {
		t.Fatalf("state %v, want Off after MinOn", s.c.State())
	}
	// Hot again at once: must wait MinOff (5 min from the 5:00 stop) before restarting.
	s.run(4*time.Minute+50*time.Second, func(uint8) Inputs { return readings(70, 50, 50, 50) }) // to 9:55
	if s.c.State() != Off {
		t.Fatal("restarted before MinOff")
	}
	s.run(10*time.Second, func(uint8) Inputs { return readings(70, 50, 50, 50) })
	if s.c.State() == Off {
		t.Fatal("did not restart after MinOff")
	}
}

func TestTankFullStopsImmediatelyAndResumesBelowResume(t *testing.T) {
	s := newSim(t)
	s.run(10*time.Minute, func(uint8) Inputs { return readings(100, 85, 90, 85) })
	if s.c.State() != Running {
		t.Fatalf("state %v, want Running", s.c.State())
	}
	if sp := s.run(5*time.Second, func(uint8) Inputs { return readings(100, 90, 95, 92) }); sp != 0 {
		t.Fatalf("speed %d at tank 92 °C, want 0", sp)
	}
	s.run(10*time.Minute, func(uint8) Inputs { return readings(110, 89, 89, 89) })
	if s.c.State() != Off {
		t.Fatal("restarted at 89 °C, above TankResume")
	}
	s.run(10*time.Minute, func(uint8) Inputs { return readings(110, 87, 92, 87) })
	if s.c.State() == Off {
		t.Fatalf("did not restart below TankResume: %s", s.c.Reason)
	}
}

func TestHottestTankSensorCountsForTankFull(t *testing.T) {
	s := newSim(t)
	in := Inputs{Collector: 100, Inlet: 80, Outlet: 85, TankTop: 85, TankMiddle: 80, TankBottom: 93}
	if sp := s.run(time.Minute, func(uint8) Inputs { return in }); sp != 0 {
		t.Fatalf("speed %d with bottom sensor at 93 °C, want 0", sp)
	}
}

func TestDoesNotRefillVeryHotCollector(t *testing.T) {
	s := newSim(t)
	s.run(time.Minute, func(uint8) Inputs { return readings(140, 60, 60, 60) })
	if s.c.State() != Off {
		t.Fatal("started with collector at 140 °C")
	}
	s.run(time.Minute, func(uint8) Inputs { return readings(125, 60, 60, 60) })
	if s.c.State() != Off {
		t.Fatal("started at 125 °C, above CollectorOK")
	}
	s.run(time.Minute, func(uint8) Inputs { return readings(115, 60, 60, 60) })
	if s.c.State() == Off {
		t.Fatalf("did not start once cooled: %s", s.c.Reason)
	}
}

func TestStaleOrBadReadingsStopThePump(t *testing.T) {
	s := newSim(t)
	s.run(10*time.Minute, func(uint8) Inputs { return readings(70, 50, 55, 50) })
	stale := s.now.Add(-2 * time.Minute)
	if sp := s.run(5*time.Second, func(uint8) Inputs {
		in := readings(70, 50, 55, 50)
		in.Updated = stale
		return in
	}); sp != 0 {
		t.Fatalf("speed %d with 2-minute-old readings, want 0", sp)
	}

	s = newSim(t)
	s.run(10*time.Minute, func(uint8) Inputs { return readings(70, 50, 55, 50) })
	if sp := s.run(5*time.Second, func(uint8) Inputs { return readings(70, 50, 250, 50) }); sp != 0 {
		t.Fatalf("speed %d with outlet 250 °C, want 0", sp)
	}
}

func TestKeepsRunningWhileCollectorReadsLikeInlet(t *testing.T) {
	// As observed: with water flowing the collector sensor reads about the
	// inlet temperature, but a 2.3 °C rise means heat is still being collected.
	s := newSim(t)
	s.run(time.Minute, func(uint8) Inputs { return readings(97, 88.5, 91, 88.5) }) // start from a hot, drained collector
	if sp := s.run(time.Hour, func(uint8) Inputs { return readings(88.9, 88.9, 91.2, 88.5) }); sp == 0 {
		t.Fatalf("stopped with a 2.3 °C rise: %s", s.c.Reason)
	}
}

func TestStartsFromTankBottom(t *testing.T) {
	// The middle is coldest but water is drawn from the bottom, so the bottom decides.
	s := newSim(t)
	in := Inputs{Collector: 94, Inlet: 88, Outlet: 88, TankTop: 85, TankMiddle: 80, TankBottom: 88}
	if s.run(time.Minute, func(uint8) Inputs { return in }); s.c.State() != Off {
		t.Fatal("started with the collector only 6 °C above the tank bottom")
	}
	in.Collector = 97
	if s.run(time.Minute, func(uint8) Inputs { return in }); s.c.State() == Off {
		t.Fatalf("did not start 9 °C above the tank bottom: %s", s.c.Reason)
	}
}

func TestReasonSaysBelowWhenCollectorColder(t *testing.T) {
	s := newSim(t)
	s.run(time.Minute, func(uint8) Inputs { return readings(24.5, 25, 25, 87.4) })
	if want := "collector 62.9 °C below tank"; s.c.Reason != want {
		t.Fatalf("reason %q, want %q", s.c.Reason, want)
	}
}
