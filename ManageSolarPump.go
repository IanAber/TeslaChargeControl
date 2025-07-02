package main

import (
	"go.einride.tech/pid"
	"log"
	"time"
)

type SolarTemps struct {
	collector int16
	input     int16
	output    int16
	//	exchanger  int16
	tankTop    int16
	tankMid    int16
	tankBottom int16
}

var (
	tempUpdate chan *SolarTemps
	logSolar   bool
)

func init() {
	tempUpdate = make(chan *SolarTemps)
}

// ManageSolarPump manages the pump pushing water from the hot tank throught the solar collectors.
// Temperatrues are in Celsius * 10 and returned to us as int16 values
func ManageSolarPump() {
	loops := 3
	controller := pid.Controller{
		Config: pid.ControllerConfig{
			ProportionalGain: 2.0,
			IntegralGain:     1.0,
			DerivativeGain:   1.0,
		},
	}
	for {
		// Triggers every time new temperatures are read. (approx 5 seconds)
		temps := <-tempUpdate

		//		log.Println("Temperatures updated.")
		// Get the solar temperatures
		tank := temps.tankTop
		if tank > temps.tankMid {
			tank = temps.tankMid
		}
		if tank > temps.tankBottom {
			tank = temps.tankBottom
		}
		// tank is now the least of the three temperatures

		// Calculate the p, i & d terms
		// p = difference between what we have and what we want. We want a lift of 3 degrees
		controller.Update(pid.ControllerInput{
			ReferenceSignal: 3.0,
			//			ActualSignal:     float64(temps.exchanger - tank),
			ActualSignal:     float64(temps.output - temps.input),
			SamplingInterval: 5 * time.Second,
		})

		if logSolar {
			log.Printf("solar : error = %f, signal = %f, integral = %f, derivative = %f : tank = %f, exchanger = %f",
				controller.State.ControlError, controller.State.ControlSignal, controller.State.ControlErrorIntegral,
				//				controller.State.ControlErrorDerivative, float64(tank)/10, float64(temps.exchanger)/10)
				controller.State.ControlErrorDerivative, float64(tank)/10, float64(temps.output)/10)
		}
		// If the collector is 5 or more degrees above the tank or
		// the pump is running and the exchanger is more than 2 degrees above the input
		// start the pump or increase it if it is already running

		// If the collector is more than 95C then increase the pump regardless.
		if temps.collector > 950 {
			if logSolar {
				log.Printf("solar : collector = %d so Increase solar pump speed", temps.collector)
			}
			Heater.IncreasePump()
		} else {
			// Slow the response down a little
			if loops <= 0 {
				//				if (temps.collector > (temps.tankTop + 50)) || ((Heater.solarPumpSetting > 0) && (temps.exchanger > (temps.input + 20))) {
				if (temps.collector > (temps.tankTop + 50)) || ((Heater.solarPumpSetting > 0) && (temps.output > (temps.input + 20))) {
					if logSolar {
						log.Printf("solar : collector = %d ,tankTop = %d, pump = %d, exchange = %d, input = %d so Increase solar pump speed",
							//							temps.collector, temps.tankTop, Heater.solarPumpSetting, temps.exchanger, temps.input)
							temps.collector, temps.tankTop, Heater.solarPumpSetting, temps.output, temps.input)
					}
					Heater.IncreasePump()
				} else {
					if logSolar {
						log.Printf("solar : collector = %d ,tankTop = %d, pump = %d, exchange = %d, input = %d so Decrease solar pump speed",
							//							temps.collector, temps.tankTop, Heater.solarPumpSetting, temps.exchanger, temps.input)
							temps.collector, temps.tankTop, Heater.solarPumpSetting, temps.output, temps.input)
					}
					Heater.DecreasePump()
				}
				loops = 4
			} else {
				// If the temperature of the water going to the collectors is warmer than the collector temperature, decrease the pump speed
				if temps.input > temps.collector {
					if logSolar {
						log.Printf(`solar : collector(%d)  < input(%d) so Decrease solar pump speed`, temps.collector, temps.input)
					}
					Heater.DecreasePump()
				}
			}
		}
		loops--
	}
}
