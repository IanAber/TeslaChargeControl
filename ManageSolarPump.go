package main

import (
	"flag"
	"log"
	"time"

	"SystemController/solarctl"
)

// SolarTemps is one set of solar loop and tank temperatures, in tenths of a
// degree C, passed from GetTemperatures to ManageSolarPump.
type SolarTemps struct {
	collector  int16
	input      int16 // water going up to the collectors
	output     int16 // water coming back to the tank
	tankTop    int16
	tankMid    int16
	tankBottom int16
	updated    time.Time // when these were last read successfully
}

var (
	tempUpdate  chan *SolarTemps
	logSolar    bool
	solarConfig = solarctl.DefaultConfig()
)

func init() {
	tempUpdate = make(chan *SolarTemps)
	flag.Float64Var(&solarConfig.MinSpeed, "solarMinSpeed", solarConfig.MinSpeed,
		"Slowest solar pump speed (%) that still lifts water to the collectors")
	flag.Float64Var(&solarConfig.TargetRise, "solarTargetRise", solarConfig.TargetRise,
		"Wanted temperature rise (°C) across the collectors while the solar pump runs")
	flag.Float64Var(&solarConfig.TankMax, "solarTankMax", solarConfig.TankMax,
		"Stop the solar pump when the hottest solar tank sensor reaches this (°C)")
	flag.Float64Var(&solarConfig.TankResume, "solarTankResume", solarConfig.TankResume,
		"Allow the solar pump again once the hottest solar tank sensor is below this (°C)")
}

// ManageSolarPump runs the solar pump controller (package solarctl) each time
// new temperatures arrive, about every 5 seconds, and drives the pump.
func ManageSolarPump() {
	c := solarctl.New(solarConfig)
	log.Printf("Solar pump controller: %+v", solarConfig)
	lastState := solarctl.State(-1)

	for temps := range tempUpdate {
		in := solarctl.Inputs{
			Time:       time.Now(),
			Updated:    temps.updated,
			Collector:  float64(temps.collector) / 10,
			Inlet:      float64(temps.input) / 10,
			Outlet:     float64(temps.output) / 10,
			TankTop:    float64(temps.tankTop) / 10,
			TankMiddle: float64(temps.tankMid) / 10,
			TankBottom: float64(temps.tankBottom) / 10,
		}
		speed := c.Step(in)
		if speed != solarPump.Speed() {
			solarPump.Set(speed)
		}
		state := c.State()
		solarPump.SetState(state.String(), c.Reason)

		// Log every state change, and every update when solar logging is on.
		if state != lastState || logSolar {
			log.Printf("solar : %s at %d%% (%s) : collector %.1f, in %.1f, out %.1f, tank %.1f/%.1f/%.1f",
				state, speed, c.Reason, in.Collector, in.Inlet, in.Outlet, in.TankTop, in.TankMiddle, in.TankBottom)
		}
		lastState = state
	}
}
