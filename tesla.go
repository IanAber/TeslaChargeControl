package main

import (
	"log"
	"time"
)

// This function will look at the various inverter parameters and work out if there is power available for car charging or water heating
// It bases this calculation on the current battery state of charge, the battery charging current and the difference between the setpoint
// and the actual battery voltage
func calculatePowerAvailable() {
	//	var iBatt float32
	//	var soc float32
	//	var frequency float64
	var delta int16
	//	lastPowerState := 0
	//	var step int16

	powerTicker := time.NewTicker(time.Second * 5)
	for range powerTicker.C {
		batAmps := iValues.GetBatAmps()
		//		log.Printf("SOC = %f : Amps = %f", iValues.soc, batAmps)
		if iValues.soc > 98 || (iValues.soc > 50 && batAmps > 30.0) {
			for iSlave := range slaves {
				slaves[iSlave].Enable(true)
			}
		}

		powerState := iValues.GetChargeLevel()
		log.Printf("powerState = %d", powerState)
		// Set the total car charging current for all cars charging
		carCurrent := float32(0.0)
		for i := range slaves {
			carCurrent += float32(slaves[i].GetCurrent()) / 100.0
		}
		TeslaParameters.SetCurrent(carCurrent)

		if iValues.AutoGn {
			// If the generator is running turn off the Tesla
			TeslaParameters.SetMaxAmps(0)
		} else if powerState == 1 { // If the delta is less than the minimum we can take more power
			// Inverter current is at 48V so approx. 5 times car current. We should push it up in small stages
			// if the state of charge of the battery > 98% and battery current is less than 100A go up 5 amps til we hit the top.
			if iValues.soc > 98 && batAmps > -100 {
				delta = 5
			} else {
				delta = int16(batAmps / 10) // Charging shows as a positive inverter current
			}
			TeslaParameters.SetSystemAmps(48) // I am not sure why we need to do this here.
			if carCurrent > 1 {
				// Car is charging so try and increase the charge rate
				TeslaParameters.ChangeCurrent(delta)
			} else {
				// No car charging requested so set the available current to 15.0 amps and turn up the auxiliary heater
				//				log.Println("Tesla not charging to default to 15A and increase electrolyser")
				TeslaParameters.SetMaxAmps(15.0)
				// If the frequency is over 60.9 the solar inverters are throttled, so we should whack the electrolyser up to full immediately
				if iValues.frequency > 60.9 {
					delta = 100
				} else {
					delta = 1
				}
			}
		} else if powerState == -1 { // If the delta is more than the max we need to reduce the load to give the battery chance to charge up
			delta = int16((batAmps + 10) / 5) // Inverter current is at 48V so 5 times the 240 car current
			if iValues.soc > 97 {
				// if the battery is more than 97% full then reduce the car by a maximum of 5A at a time
				delta = -5
				if iValues.Log || TeslaParameters.IsLogging() {
					log.Println("battery over 97% so only reducing Tesla by 5 amps")
				}
			}
			if iValues.Log || TeslaParameters.IsLogging() {
				log.Printf("car = %f, changing it %dA", carCurrent, delta)
			}
			TeslaParameters.ChangeCurrent(delta)
		}
	}
}
