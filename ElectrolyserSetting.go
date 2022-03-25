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

type ElectrolyserSetting struct {
	enabled            bool       // Only turn on electrolysers if enabled == true
	mu                 sync.Mutex // Controls access
	currentSetting     uint8      // o = off. Defines the target output
	dontDecreaseBefore time.Time  // This is used to hold off a decrease to give the string inverters a chance to ramp up
	dontIncreaseBefore time.Time  // This is used to prevent an increase if we have just increased within a short time
	// to stop running up too quickly.
}

//func NewElectrolyserSetting() *ElectrolyserSetting {
//	h := new(ElectrolyserSetting)
//	h.enabled = true
//	h.dontDecreaseBefore = time.Now()
//	h.dontIncreaseBefore = time.Now()
//	return h
//}

/**
setElectrolyser - Calls the wEB service to set the electrolysers output.
*/
func (e *ElectrolyserSetting) setElectrolyser() {
	var jRate struct {
		Rate int64 `json:"rate"`
	}

	jRate.Rate = int64(e.currentSetting)
	body, err := json.Marshal(jRate)
	if err != nil {
		log.Print(err)
		return
	}
	_, err = http.Post("http://firefly.home:20080/el/setrate", "application/json; charset=utf-8", bytes.NewBuffer(body))
	if err != nil {
		log.Print(err)
	} else {
		log.Println("Electrolyser rate set to ", jRate.Rate)
	}
}

func (e *ElectrolyserSetting) GetSetting() uint8 {
	return e.currentSetting
}

func (e *ElectrolyserSetting) Increase(step uint8) bool {
	e.ReadSetting()
	if e.currentSetting < 100 {
		e.currentSetting += step
		if e.currentSetting > 100 {
			e.currentSetting = 100
		}
		e.setElectrolyser()
		//		log.Println("Electrolyser increased to ", e.currentSetting)
		return true
	} else {
		//		log.Println("Electrolyser is already at 100%")
		return false
	}
}

func (e *ElectrolyserSetting) Decrease(step uint8) bool {
	e.ReadSetting()
	if e.currentSetting > 0 {
		if e.currentSetting > step {
			e.currentSetting -= step
		} else {
			e.currentSetting = 0
		}
		e.setElectrolyser()
		//		log.Println("Electrolyser decreased to ", e.currentSetting)
		return true
	} else {
		//		log.Println("Electrolyser is already at 0%")
		return false
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
		Rate uint8 `json:"rate"`
	}
	err = json.Unmarshal(responseBytes, &Rate)
	if err != nil {
		log.Print(err)
		return
	}
	e.currentSetting = Rate.Rate
	//	log.Println("Electrolyser rate is ", Rate.Rate)
}

func (e *ElectrolyserSetting) preHeat() {
	_, err := http.Post("http://firefly.home:20080/el/preheat", "application/json; charset=utf-8", nil)
	if err != nil {
		log.Print(err)
	} else {
		log.Println("Electrolyser preheat started")
	}
}
