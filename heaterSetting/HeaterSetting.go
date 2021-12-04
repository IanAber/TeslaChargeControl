package heaterSetting

import (
	"github.com/golang/glog"
	"github.com/stianeikeland/go-rpio"
	"math"
	"sync"
	"time"
)

/****************************************************************************************
* GPIO outputs are:-
*
* Pin	Function
*-------------------
*  0	Fan							(17)
*  1	PWM output for Solar Pump
*  2	AC enable					(27)
*  3	6kW heater element (high)
*  4	Pump Enable					(23)
*  5	2.5kW heater element (med)
****************************************************************************************/

const pump = 23 // Pump is on GPIO 4

// Array of heater SSR port pins in order of least powerful to most powerful
var heaters = [...]uint8{6, 24, 22}

type HeaterSetting struct {
	enabled bool        // Only turn on heater elements if enabled == true
	pump    bool        // Defines the logical pump operation. Actual operation will be delayed on turning off
	mu      sync.Mutex  // Controls access
	timer   *time.Timer // Used to turn the pump off after a delay to ensure all
	// the heated water is pumped into the main storage tank
	currentSetting     uint8     // o = off. Defines which elements are switched on
	maxSetting         uint8     // Calculated by the constructor based on the number of heater elements defined
	dontDecreaseBefore time.Time // This is used to hold off a decrease to give the string inverters a chance to ramp up
	dontIncreaseBefore time.Time // This is used to prevent an increase if we have just increased within a short time
	// to stop running up to quickly.
	hotTankTemp int16 // Hot tank temperature (Deg C x 10) Max allowed = 95C (950)
}

func New() *HeaterSetting {
	h := new(HeaterSetting)
	h.enabled = true
	h.hotTankTemp = 1000
	h.SetHeater(0) // Ensures all ports are configured correctly
	h.maxSetting = uint8(math.Pow(2, float64(len(heaters)))) - 1
	h.dontDecreaseBefore = time.Now()
	h.dontIncreaseBefore = time.Now()
	return h
}

// SetHeater /*
// Set the heater power.
func (h *HeaterSetting) SetHeater(setting uint8) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// On/Off control
	if !h.enabled {
		setting = 0
	}

	// Overheating prevention
	if h.hotTankTemp > 950 {
		setting = 0
	}

	if setting == 0 {
		// Schedule the pump to stop if it is not already scheduled
		h.pump = false
		if h.timer == nil {
			h.timer = time.AfterFunc(time.Second*30, h.turnOffPump)
		}
	} else {
		// Start the pump first
		if h.timer != nil {
			h.timer.Stop()
			h.timer = nil
		}
		h.pump = true
		h.turnOnPump()
	}
	// Set the heating elements up
	h.currentSetting = setting
	if h.currentSetting > h.maxSetting {
		h.currentSetting = h.maxSetting
	}

	// Actually turn on or off the heating elements
	h.setHeater()
}

func (h *HeaterSetting) turnOffPump() {
	// Turn the pump off and mark it stopped. Also turn off all heaters just in case they are on.
	h.mu.Lock()
	defer h.mu.Unlock()
	h.timer = nil
	h.pump = false
	h.currentSetting = 0
	// Actually turn the elements off if they are not already...
	h.setHeater()
	// Now turn the pump off
	p := rpio.Pin(pump)
	p.Mode(rpio.Output)
	p.High()
}

func (h *HeaterSetting) turnOnPump() {
	// Turn the pump on and cancel any waiting off function
	if h.timer != nil {
		h.timer.Stop()
	}
	err := rpio.Open()
	if err != nil {
		glog.Errorf("Failed to open the GPIO ports. - %s\n", err)
		return
	}
	p := rpio.Pin(pump)
	p.Mode(rpio.Output)
	p.Low()
	if p.Read() != 0 {
		glog.Errorln("Failed to turn the pump on!")
	}

}

// Increase the heater current. Return true if we did increase it or false if we are already at maximum.
func (h *HeaterSetting) Increase(frequency float64) bool {
	var setting = h.currentSetting
	if setting < h.maxSetting {
		if h.dontIncreaseBefore.After(time.Now()) { // If we just increased the setting and are waiting for the inverters to react hold off further increases
			return true
		}
		h.SetHeater(setting + 1)
		if frequency > 60.0 {
			// Based on how high above 60Hz the frequency is we should hold this new level to let the string inverters
			// ramp up. Hold for 15 seconds for each Hz over 60.
			h.dontDecreaseBefore = time.Now().Add(time.Duration((frequency - 60.0) * float64(time.Second) * 15))
		} else {
			h.dontDecreaseBefore = time.Now()
		}
		h.dontIncreaseBefore = time.Now().Add(time.Second * 5)
		return true
	} else {
		return false
	}
}

// Decrease /*
// Drop the heater current. Return true if we dropped it or false if we are already fully off
// ignoreTime tells us not to wait for the string inverters. This is used if we are dropping the
// heater because we are ramping up the car.
func (h *HeaterSetting) Decrease(ignoreTime bool) bool {
	var setting = h.currentSetting
	if setting > 0 {
		if !ignoreTime && h.dontDecreaseBefore.After(time.Now()) {
			// We are still holding the heater in case the string inverters are able to ramp up so pretend we
			// decreased but don't actually change anything
			return true
		} else {
			h.SetHeater(setting - 1)
			return true
		}
	} else {
		return false
	}
}

// Internal function to drive the port pins controlling the Solid state Relays
func (h *HeaterSetting) setHeater() {
	err := rpio.Open()
	if err != nil {
		glog.Errorf("Failed to open the GPIO ports. - %s\n", err)
		return
	}
	for i, p := range heaters {
		val := ((h.currentSetting >> uint(i)) & 1) > 0
		pin := rpio.Pin(p)
		pin.Mode(rpio.Output)
		if val {
			pin.High()
		} else {
			pin.Low()
		}
	}
}

func (h *HeaterSetting) GetSetting() uint8 {
	return h.currentSetting
}

func (h *HeaterSetting) GetEnabled() string {
	if h.enabled {
		return "ON"
	} else {
		return "OFF"
	}
}

func (h *HeaterSetting) SetEnabled(bSetting bool) {
	h.enabled = bSetting
	if !bSetting {
		h.SetHeater(0)
	}
}

func (h *HeaterSetting) GetHotTankTemp() int16 {
	return h.hotTankTemp
}

func (h *HeaterSetting) SetHotTankTemp(t int16) {
	h.hotTankTemp = t
	if t > 950 {
		h.SetHeater(0)
	}
}

func (h *HeaterSetting) GetPump() bool {
	pin := rpio.Pin(pump)
	return pin.Read() == 0
}
