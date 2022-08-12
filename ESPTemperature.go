package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

type ESPTemperature struct {
	url          string
	error        string
	Ambient      float32    `json:"temperature"`
	Humidity     float32    `json:"humidity"`
	Temperatures [6]float32 `json:"rtd"`
	LastUpdate   time.Time
	LastError    error
	Updated      bool
	mu           sync.Mutex
}

type newValues struct {
	Ambient      float32    `json:"temperature"`
	Humidity     float32    `json:"humidity"`
	Temperatures [6]float32 `json:"rtd"`
}

func NewESPTemperature(url string) *ESPTemperature {
	esp := new(ESPTemperature)
	esp.url = url
	return esp
}

func (esp *ESPTemperature) readTemperatures() {
	var values newValues
	if resp, err := http.Get(esp.url + "/ajax/climate"); err != nil {
		esp.LastError = err
		log.Println(err)
		return
	} else {
		if bytes, err := io.ReadAll(resp.Body); err != nil {
			esp.LastError = err
			log.Println(err)
			return
		} else {
			esp.mu.Lock()
			defer esp.mu.Unlock()
			if err := json.Unmarshal(bytes, &values); err != nil {
				esp.LastError = err
				log.Println(err)
				return
			} else {
				errValue := false
				if values.Ambient < 200 && values.Ambient > -50 {
					esp.Ambient = values.Ambient
				} else {
					errValue = true
				}
				if values.Humidity <= 100 && values.Humidity >= 0 {
					esp.Humidity = values.Humidity
				} else {
					errValue = true
				}
				for t := 0; t < 6; t++ {
					if values.Temperatures[t] < 200 && values.Temperatures[t] > -50 {
						esp.Temperatures[t] = values.Temperatures[t]
					} else {
						errValue = true
					}
				}
				if !errValue {
					esp.LastUpdate = time.Now()
					esp.Updated = true
				} else {
					log.Printf("Bad temperatures from %s - ambient = %f:%f | temps =%f:%f:%f:%f:%f:%f\n",
						esp.url, values.Ambient, values.Humidity,
						values.Temperatures[0], values.Temperatures[1], values.Temperatures[2],
						values.Temperatures[3], values.Temperatures[4], values.Temperatures[5])
				}
			}
		}
	}
	return
}

func (esp *ESPTemperature) getAmbient() (temperature float32, humidity float32) {
	esp.mu.Lock()
	defer esp.mu.Unlock()
	return esp.Ambient, esp.Humidity
}

func (esp *ESPTemperature) getLastUpdate() time.Time {
	esp.mu.Lock()
	defer esp.mu.Unlock()
	return esp.LastUpdate
}

func (esp *ESPTemperature) getTemperatures() [6]float32 {
	esp.mu.Lock()
	defer esp.mu.Unlock()
	return esp.Temperatures
}
