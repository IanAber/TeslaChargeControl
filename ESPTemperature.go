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

func NewESPTemperature(url string) *ESPTemperature {
	esp := new(ESPTemperature)
	esp.url = url
	return esp
}

func (esp *ESPTemperature) readTemperatures() {
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
			if err := json.Unmarshal(bytes, &esp); err != nil {
				esp.LastError = err
				log.Println(err)
				return
			} else {
				esp.LastUpdate = time.Now()
				esp.Updated = true
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
