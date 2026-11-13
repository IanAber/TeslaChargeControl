package main

import (
	"log"
	"sync"
	"time"

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

// SolarPumpControl drives the pump that circulates water from the hot tank
// through the solar collectors. ManageSolarPump decides the speed.
type SolarPumpControl struct {
	mu        sync.Mutex
	setting   uint8     // Duty cycle percentage of the pump
	startTime time.Time // When the pump was started
	minSpeed  uint8     // When the speed drops below this the pump is stopped
	startSecs uint      // Seconds after starting at full speed before the pump may slow down
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
	return &SolarPumpControl{
		minSpeed:  10,
		startTime: time.Now(),
		startSecs: 30,
	}
}

func (s *SolarPumpControl) SetSpeed(speed uint8) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if speed > 100 {
		log.Print("Maximum pump speed is 100. Requested speed is ", speed)
	}
	rpio.Pin(solarPumpPWMPin).DutyCycle(uint32(speed), 100)
	s.setting = speed
}

// Increase starts the pump at full speed if it is stopped, or speeds it up by 1%.
func (s *SolarPumpControl) Increase() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setting < 100 {
		if s.setting == 0 {
			s.startTime = time.Now()
			s.setting = 100
		} else {
			s.setting++
		}
		rpio.Pin(solarPumpPWMPin).DutyCycle(uint32(s.setting), 100)
	}
	startSolarPump() // In case the pump is stopped.
}

// Decrease slows the pump by 1%, or stops it once it is at the minimum speed.
// It does nothing during the start-up period.
func (s *SolarPumpControl) Decrease() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setting == 0 || time.Since(s.startTime) < time.Duration(s.startSecs)*time.Second {
		return
	}
	p := rpio.Pin(solarPumpPWMPin)
	if s.setting > s.minSpeed {
		s.setting--
		p.DutyCycle(uint32(s.setting), 100)
		startSolarPump()
	} else {
		s.setting = 0
		p.DutyCycle(0, 100)
		stopSolarPump()
	}
}

// Speed returns the pump duty cycle in percent; 0 means stopped.
func (s *SolarPumpControl) Speed() uint8 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.setting
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
