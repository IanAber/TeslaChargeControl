package main

import (
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/stianeikeland/go-rpio"
)

const CpuTempFile = "/sys/class/thermal/thermal_zone0/temp"
const FanPin = 17

// GetCpuTemp returns the RPi's CPU temperature in °C
func GetCpuTemp() (float64, error) {
	data, err := os.ReadFile(CpuTempFile)
	if err != nil {
		return -1, err
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return -1, err
	}
	return value / 1000, nil
}

/*
ManageCpuTemp tries to keep the temperature of the CPU below 48 Celcius
*/
func ManageCpuTemp() {
	t := time.NewTicker(time.Second)

	for {
		<-t.C

		pin := rpio.Pin(FanPin)
		pin.Mode(rpio.Output)
		if t, err := GetCpuTemp(); err != nil || t > 49.0 {
			if pin.Read() != 0 {
				log.Print("Temp = ", t, " Turn on the fan")
				pin.Low()
			}
		} else {
			if (t < 46) && (pin.Read() == 0) {
				log.Print("Temp = ", t, " Turn off the fan")
				pin.High()
			}
		}
	}
}
