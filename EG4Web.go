package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"SystemController/EG4"
)

// The most recent EG4 reading, updated by Read12000XP after every poll that
// got an answer from at least one inverter, and the hub that pushes it to
// clients of /realtime/eg4Events.
var (
	latestEG4Mu sync.RWMutex
	latestEG4   *EG4.SystemReading

	eg4Hub = newEventHub()
)

// setLatestEG4 stores a new reading and pushes it to every event-stream client.
func setLatestEG4(sr *EG4.SystemReading) {
	latestEG4Mu.Lock()
	latestEG4 = sr
	latestEG4Mu.Unlock()

	msg, err := json.Marshal(eg4Response(sr))
	if err != nil {
		log.Println("setLatestEG4() - ", err)
		return
	}
	eg4Hub.Publish(msg)
}

// eg4Response is the JSON form of a reading used by getEG4 and the event
// stream: the reading itself plus Errors as text and AgeSeconds.
func eg4Response(sr *EG4.SystemReading) any {
	// error values marshal as {}, so send the messages instead.
	errs := make(map[string]string, len(sr.Errors))
	for name, e := range sr.Errors {
		errs[name] = e.Error()
	}
	return struct {
		*EG4.SystemReading
		Errors     map[string]string // shadows SystemReading.Errors
		AgeSeconds float64
	}{sr, errs, time.Since(sr.Time).Seconds()}
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

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if wantPretty(r) {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(eg4Response(sr)); err != nil {
		log.Println("getEG4() - ", err)
	}
}

// eg4Events streams EG4 readings as Server-Sent Events. Each event's data is
// the same JSON that getEG4 returns: the latest reading as soon as the client
// connects, then every new reading as it is polled.
func eg4Events(w http.ResponseWriter, r *http.Request) {
	eg4Hub.ServeHTTP(w, r)
}

// eg4Sample is one point in the history returned by getEG4History, with the
// short field names the dashboard uses.
type eg4Sample struct {
	T     int64                    `json:"t"` // Unix ms
	PV    float64                  `json:"pv"`
	AC    float64                  `json:"ac"`
	Load  float64                  `json:"load"`
	SOC   float64                  `json:"soc"`
	V     float64                  `json:"v"`
	A     float64                  `json:"a"`
	Units map[string]eg4UnitSample `json:"units"`
}

type eg4UnitSample struct {
	PV   float64 `json:"pv"`
	AC   float64 `json:"ac"`
	Load float64 `json:"load"`
	A    float64 `json:"a"`
}

// eg4HistoryPoints is roughly how many points getEG4History returns; longer
// windows are averaged into wider buckets to stay near it.
const eg4HistoryPoints = 720

// getEG4History returns logged EG4 readings for the last ?minutes=N (default
// 15, maximum 1440) from the eg4_system and eg4_inverter tables:
//
//	{"bucketSeconds": 2, "samples": [{"t":..., "pv":..., "units": {"XP-1": {...}}}, ...]}
//
// Readings are averaged into buckets of bucketSeconds so a long window returns
// about 720 points; short windows come back at full resolution.
func getEG4History(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	minutes := 15
	if s := r.URL.Query().Get("minutes"); s != "" {
		m, err := strconv.Atoi(s)
		if err != nil || m < 1 || m > 1440 {
			http.Error(w, "minutes must be a whole number from 1 to 1440.", http.StatusBadRequest)
			return
		}
		minutes = m
	}
	db := pDB
	if db == nil {
		http.Error(w, "The database is not connected.", http.StatusServiceUnavailable)
		return
	}
	bucket := (minutes*60 + eg4HistoryPoints - 1) / eg4HistoryPoints // seconds, at least 1
	since := time.Now().Add(-time.Duration(minutes) * time.Minute)

	rows, err := db.Query(`SELECT FLOOR(UNIX_TIMESTAMP(logged) / ?) AS b,
	                              ROUND(AVG(UNIX_TIMESTAMP(logged)) * 1000),
	                              AVG(pv_power), AVG(ac_couple_power), AVG(load_power),
	                              AVG(soc), AVG(battery_voltage), AVG(battery_current_bms)
	                         FROM logging.eg4_system
	                        WHERE logged >= ?
	                        GROUP BY b
	                        ORDER BY b`, bucket, since)
	if err != nil {
		eg4HistoryError(w, err)
		return
	}
	var samples []*eg4Sample
	byBucket := map[int64]*eg4Sample{}
	for rows.Next() {
		var b int64
		s := &eg4Sample{Units: map[string]eg4UnitSample{}}
		if err := rows.Scan(&b, &s.T, &s.PV, &s.AC, &s.Load, &s.SOC, &s.V, &s.A); err != nil {
			rows.Close()
			eg4HistoryError(w, err)
			return
		}
		samples = append(samples, s)
		byBucket[b] = s
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		eg4HistoryError(w, err)
		return
	}

	// Inverter rows are bucketed by their poll's time so they line up with
	// the system rows above.
	rows, err = db.Query(`SELECT FLOOR(UNIX_TIMESTAMP(s.logged) / ?) AS b, i.name,
	                             AVG(i.pv_power), AVG(i.ac_couple_power), AVG(i.load_power), AVG(i.battery_current)
	                        FROM logging.eg4_inverter i
	                        JOIN logging.eg4_system s ON s.id = i.system_id
	                       WHERE s.logged >= ?
	                       GROUP BY b, i.name`, bucket, since)
	if err != nil {
		eg4HistoryError(w, err)
		return
	}
	for rows.Next() {
		var b int64
		var name string
		var u eg4UnitSample
		if err := rows.Scan(&b, &name, &u.PV, &u.AC, &u.Load, &u.A); err != nil {
			rows.Close()
			eg4HistoryError(w, err)
			return
		}
		if s := byBucket[b]; s != nil {
			s.Units[name] = u
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		eg4HistoryError(w, err)
		return
	}

	if samples == nil {
		samples = []*eg4Sample{}
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if wantPretty(r) {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(struct {
		BucketSeconds int          `json:"bucketSeconds"`
		Samples       []*eg4Sample `json:"samples"`
	}{bucket, samples}); err != nil {
		log.Println("getEG4History() - ", err)
	}
}

func eg4HistoryError(w http.ResponseWriter, err error) {
	log.Printf("Error reading EG4 history from the database - %s", err)
	http.Error(w, "Failed to read the EG4 history.", http.StatusInternalServerError)
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
