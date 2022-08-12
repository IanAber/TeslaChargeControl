package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

/**
Electrolysers can be adjusted from 60% to 100%. Their current consumption is approximately 6.5A @ 60% to 11A at 100%
*/

const MaxStartPressure = 32.5 // We will not start the electrolysers if the tank pressure is above or equal to this

type ElectrolyserSetting struct {
	mu               sync.Mutex // Controls access
	currentSetting   int16      // -1 = switched off, 0 = stopped. Defines the target output
	requestedSetting int16      // The last setting request made to the Firefly
	turnedOnOff      time.Time  // This is the time the turn on or turn off command was sent
	gasPressure      float64    // The tank pressure in bar.
	status           string     // Status is OFF. Idle, Standby or Active
	Enabled          bool       // Only drive the electrolysers if the is set true.
	maxGasPressure   float64    // This is the pressure at which we no longer increase the productiuon rate
}

func closeResponse(response *http.Response) {
	if response != nil {
		if err := response.Body.Close(); err != nil {
			log.Println("Failed to close http response body - ", err)
		}
	}
}

func (e *ElectrolyserSetting) IsEnabled() bool {
	return e.Enabled
}

func (e *ElectrolyserSetting) GetRate() int16 {
	return e.currentSetting
}

func (e *ElectrolyserSetting) SetEnabled(enable bool) {
	e.Enabled = enable
}

/**
setElectrolyser - Calls the wEB service to set the electrolysers output.
*/
func (e *ElectrolyserSetting) setElectrolyser() {
	var jRate struct {
		Rate int64 `json:"rate"`
	}

	if !e.Enabled {
		return
	}
	jRate.Rate = int64(e.currentSetting)
	//	log.Println("Setting electrolyser to ", jRate.Rate, " (", e.currentSetting, ")")
	body, err := json.Marshal(jRate)
	if err != nil {
		log.Print(err)
		return
	}
	e.requestedSetting = int16(jRate.Rate)

	response, err := http.Post("http://firefly.home:20080/el/setrate", "application/json; charset=utf-8", bytes.NewBuffer(body))
	defer closeResponse(response)
	if err != nil {
		log.Print(err)
	} else {
		log.Println("Electrolyser rate requesed =", jRate.Rate)
	}
}

func (e *ElectrolyserSetting) turnOnElectrolysers() {
	if !e.Enabled {
		return
	}
	e.ReadSetting()
	if e.gasPressure >= MaxStartPressure {
		return
	}
	response, err := http.Post("http://firefly.home:20080/el/on", "application/json; charset=utf-8", nil)
	defer closeResponse(response)
	if err != nil {
		log.Print(err)
	} else {
		e.turnedOnOff = time.Now()
		log.Println("Electrolysers turned on")
	}
}

func (e *ElectrolyserSetting) turnOffElectrolysers() {
	if !e.Enabled {
		return
	}
	response, err := http.Post("http://firefly.home:20080/el/off", "application/json; charset=utf-8", nil)
	defer closeResponse(response)
	if err != nil {
		log.Print(err)
	} else {
		e.turnedOnOff = time.Now()
		log.Println("Electrolysers turned off")
	}
}

// MaxInt16 returns the larger of the two int8 variables passed in
func MaxInt16(a int16, b int16) int16 {
	if a > b {
		return a
	} else {
		return b
	}
}

// MinInt16 returns the smaller of the two int8 variables passed in
func MinInt16(a int16, b int16) int16 {
	if a < b {
		return a
	} else {
		return b
	}
}

func (e *ElectrolyserSetting) ChangeRate(step int16) bool {
	//	log.Printf("Change electrolyser by %d %%\n", step)
	if !e.Enabled {
		return false
	}
	if step > 0 {
		e.ReadSetting()
		if e.gasPressure >= maxGasPressure {
			// If pressure is at 34bar then we won't push the electorlyser up. This stops us form short cycling them when they are
			// already close to the cutoff and above the restart pressure
			return false
		}
		// If the solar production total is more than a minute old get it again.
		if SolarProduction.logged.Before(time.Now().Add(0 - time.Minute)) {
			getSolarStringData(nil)
		}

		// If we are not getting at lease 3kW of Solar Power then do not increase the electrolysers
		if SolarProduction.power <= 3000.0 {
			return false
		}
		if e.status == "OFF" {
			// electrolysers are turned off
			if time.Since(e.turnedOnOff) > (time.Minute * 5) {
				// Only allow one turn of/off command every 5 minutes minute
				//				log.Println("Turning on the electrolysers")
				e.turnOnElectrolysers()
			}
			return false
		}

		if e.status == "Standby" {
			// One or more electrolyser is in standby becuse the tank is up to maximum pressure so don't try and increase the output
			return false
		}

		if e.currentSetting < 100 {
			// We are not at full output so push it up by the step amount
			e.currentSetting = MinInt16(e.currentSetting+step, 100)
			// Set the electrolyser production rate to the new increased value
			e.setElectrolyser()
			//			log.Println("Electrolyser increased to ", e.currentSetting)
			// Tell the caller we accepted the request
			return true
		}
	} else {
		e.ReadSetting()
		//		log.Println("Electrolyser current setting =", e.currentSetting)
		if e.currentSetting > 0 {
			e.currentSetting = MaxInt16(e.currentSetting+step, 0)
			e.setElectrolyser()
			//			log.Println("Electrolyser decreased to ", e.currentSetting)
			return true
		}
	}
	return false
}

func Int16Abs(a int16) int16 {
	if a < 0 {
		return 0 - a
	} else {
		return a
	}
}

func (e *ElectrolyserSetting) ReadSetting() {
	response, err := http.Get("http://firefly.home:20080/el/getRate")
	if err != nil {
		log.Print(err)
		return
	}
	responseBytes, err := io.ReadAll(response.Body)
	if err != nil {
		log.Print(err)
		return
	}
	var Rate struct {
		Rate     int16   `json:"rate"`
		Pressure float64 `json:"gas"`
		Status   string  `json:"Status"`
	}
	err = json.Unmarshal(responseBytes, &Rate)
	if err != nil {
		log.Print(err)
		return
	}
	e.currentSetting = Rate.Rate
	e.gasPressure = Rate.Pressure
	e.status = Rate.Status
	//	log.Printf("Electrolyser returned gas=%f : setting=%d expected %d : status=%s\n", e.gasPressure, e.currentSetting, e.requestedSetting, e.status)
	// This was not what we expected so update it if it is more than 5% out.
	if Int16Abs(e.currentSetting-e.requestedSetting) > 5 {
		//		log.Println("Adjusting...")
		e.setElectrolyser()
	}
}

func (e *ElectrolyserSetting) preHeat() {
	if !e.Enabled {
		return
	}
	e.ReadSetting()
	if e.status == "OFF" {
		response, err := http.Post("http://firefly.home:20080/el/preheat", "application/json; charset=utf-8", nil)
		defer closeResponse(response)
		if err != nil {
			log.Print(err)
		} else {
			log.Println("Electrolyser preheat started")
		}
	}
}
