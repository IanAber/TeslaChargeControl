package solarctl

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConfigJSONRoundTrip(t *testing.T) {
	want := DefaultConfig()
	want.Ti = 90 * time.Second
	want.PrimeTime = 1500 * time.Millisecond
	b, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got Config
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip changed the config:\n got %+v\nwant %+v", got, want)
	}
}

func TestConfigJSONMissingFieldsKeepDefaults(t *testing.T) {
	cfg := DefaultConfig()
	if err := json.Unmarshal([]byte(`{"minSpeed": 45, "tiSec": 120}`), &cfg); err != nil {
		t.Fatal(err)
	}
	want := DefaultConfig()
	want.MinSpeed, want.Ti = 45, 2*time.Minute
	if cfg != want {
		t.Fatalf("got %+v\nwant %+v", cfg, want)
	}
}

func TestValidate(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Fatalf("default config is invalid: %v", err)
	}
	for name, change := range map[string]func(*Config){
		"zero Ti":            func(c *Config) { c.Ti = 0 },
		"resume above max":   func(c *Config) { c.TankResume = c.TankMax + 1 },
		"collector ok > max": func(c *Config) { c.CollectorOK = c.CollectorMax },
		"min speed > 100":    func(c *Config) { c.MinSpeed = 101 },
		"no rise >= target":  func(c *Config) { c.NoRise = c.TargetRise },
		"zero max step":      func(c *Config) { c.MaxStep = 0 },
		"zero max age":       func(c *Config) { c.MaxAge = 0 },
	} {
		cfg := DefaultConfig()
		change(&cfg)
		if cfg.Validate() == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestSetConfigWhileRunning(t *testing.T) {
	s := newSim(t)
	hot := func(uint8) Inputs { return readings(60, 40, 45, 40) } // rise 5 = target
	s.run(5*time.Minute, hot)
	if s.c.State() != Running {
		t.Fatalf("expected Running, got %s", s.c.State())
	}
	before := s.c.Speed()

	cfg := s.c.Config()
	cfg.TargetRise = 3 // rise is now 2 above target: the pump should speed up gradually
	cfg.MinSpeed = before + 10
	s.c.SetConfig(cfg)
	if got := s.c.Speed(); got != before+10 {
		t.Fatalf("speed not raised to the new minimum: %.1f", got)
	}
	speed := s.run(5*time.Second, hot)
	if float64(speed) > before+10+cfg.MaxStep+0.5 {
		t.Fatalf("speed jumped to %d after changing the target", speed)
	}
	if s.c.State() != Running {
		t.Fatalf("changing the config stopped the pump: %s", s.c.Reason)
	}
}

func TestDecodeConfigRejectsUnknownFields(t *testing.T) {
	if _, err := DecodeConfig([]byte(`{"minSpeeed": 45}`), DefaultConfig()); err == nil {
		t.Fatal("expected an error for a misspelt field")
	}
	cfg, err := DecodeConfig([]byte(`{"minSpeed": 45}`), DefaultConfig())
	if err != nil || cfg.MinSpeed != 45 || cfg.TankMax != DefaultConfig().TankMax {
		t.Fatalf("got %+v, %v", cfg, err)
	}
}
