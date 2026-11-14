package main

import (
	"log"
	"sync"

	"github.com/golang/glog"
	"github.com/stianeikeland/go-rpio"
)

/****************************************************************************************
* GPIO pins used by the controller:-
*
* https://pinout.xyz/pinout/wiringpi
*
* Wiring Pi	GPIO 	PIN	Function
*-------------------
*  0		17		11	Cabinet fans
*  1		18		12	PWM output for the solar pump
*  2		27		13	Solar pump power relay
****************************************************************************************/

const solarPumpPWMPin = 18
const solarPumpRelayPin = 27

// SolarPumpControl drives the pump that lifts water from the bottom of the
// solar tank to the collectors: PWM speed on GPIO 18, power relay on GPIO 27.
// ManageSolarPump decides the speed.
type SolarPumpControl struct {
	mu     sync.Mutex
	speed  uint8  // duty cycle percentage; 0 means stopped
	state  string // controller state, e.g. "Running"
	reason string // why the controller is in that state
}

func init() {
	if err := rpio.Open(); err != nil {
		glog.Errorf("Failed to open the GPIO ports. - %s\n", err)
	}
	// Start the pulse width modulation on the solar pump
	p := rpio.Pin(solarPumpPWMPin)
	p.Mode(rpio.Pwm)
	p.Freq(100000) // 100kHz
	p.DutyCycle(0, 100)

	// Make sure the relay is off
	stopSolarPump()
}

func NewSolarPumpControl() *SolarPumpControl {
	return &SolarPumpControl{state: "Off", reason: "starting up"}
}

// Set runs the pump at speed percent, switching the relay on, or stops it
// (relay off) when speed is 0.
func (s *SolarPumpControl) Set(speed uint8) {
	if speed > 100 {
		log.Print("Maximum pump speed is 100. Requested speed is ", speed)
		speed = 100
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rpio.Pin(solarPumpPWMPin).DutyCycle(uint32(speed), 100)
	if speed > 0 {
		startSolarPump()
	} else {
		stopSolarPump()
	}
	s.speed = speed
}

// Speed returns the pump duty cycle in percent; 0 means stopped.
func (s *SolarPumpControl) Speed() uint8 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.speed
}

// SetState records the controller state and the reason for it, for the status page.
func (s *SolarPumpControl) SetState(state, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state, s.reason = state, reason
}

// State returns the controller state and the reason for it.
func (s *SolarPumpControl) State() (state, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.reason
}

func startSolarPump() {
	p := rpio.Pin(solarPumpRelayPin)
	p.Mode(rpio.Output)
	p.Low()
}

func stopSolarPump() {
	p := rpio.Pin(solarPumpRelayPin)
	p.Mode(rpio.Output)
	p.High()
}
