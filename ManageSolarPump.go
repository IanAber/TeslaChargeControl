package main

type SolarTemps struct {
	collector  int16
	input      int16
	output     int16
	exchanger  int16
	tankTop    int16
	tankMid    int16
	tankBottom int16
}

var tempUpdate chan (*SolarTemps)

func init() {
	tempUpdate = make(chan (*SolarTemps))
}

// ManageSolarPump manages the pump pushing water from the hot tank throught the solar collectors.
func ManageSolarPump() {

	for {
		// Triggere every time new temperatures are read.
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
		// tenk is now the least of the three temperatures

		// If the collector is 5 or more degrees above the tank or the exchanger is more than 2 degrees above the tank
		// start the pump or increase it if it is already running
		if (temps.collector > (tank + 50)) || (temps.exchanger > (tank + 20)) {
			Heater.IncreasePump()
		} else {
			Heater.DecreasePump()
		}
		// If the temperature of the water going to the collectors is warmer than the collector temperature, decrease the pump speed
		if temps.input > temps.collector {
			Heater.DecreasePump()
		}
	}
}
