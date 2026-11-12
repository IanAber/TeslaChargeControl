package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"SystemController/EG4"
)

// The most recent EG4 reading, updated by Read12000XP after every poll that
// got an answer from at least one inverter.
var (
	latestEG4Mu sync.RWMutex
	latestEG4   *EG4.SystemReading
)

func setLatestEG4(sr *EG4.SystemReading) {
	latestEG4Mu.Lock()
	latestEG4 = sr
	latestEG4Mu.Unlock()
}

// getEG4 returns the latest EG4 reading as JSON: the system totals, battery and
// BMS values, and a Units entry for each inverter that answered. Errors names
// any inverter that did not answer, and AgeSeconds says how old the reading is.
// It returns 503 until the first successful poll.
//
// Add ?pretty (or pretty=true) for indented, readable JSON; without it the
// JSON is compact. pretty=false or pretty=0 also gives compact JSON.
func getEG4(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	latestEG4Mu.RLock()
	sr := latestEG4
	latestEG4Mu.RUnlock()
	if sr == nil {
		http.Error(w, "No EG4 inverter reading yet.", http.StatusServiceUnavailable)
		return
	}

	// error values marshal as {}, so send the messages instead.
	errs := make(map[string]string, len(sr.Errors))
	for name, e := range sr.Errors {
		errs[name] = e.Error()
	}
	resp := struct {
		*EG4.SystemReading
		Errors     map[string]string // shadows SystemReading.Errors
		AgeSeconds float64
	}{sr, errs, time.Since(sr.Time).Seconds()}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if wantPretty(r) {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(resp); err != nil {
		log.Println("getEG4() - ", err)
	}
}

// wantPretty reports whether the request asks for indented JSON with a
// "pretty" query parameter. A bare ?pretty counts as true.
func wantPretty(r *http.Request) bool {
	q := r.URL.Query()
	if !q.Has("pretty") {
		return false
	}
	switch strings.ToLower(q.Get("pretty")) {
	case "0", "false", "no", "off":
		return false
	}
	return true
}
