package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/stianeikeland/go-rpio"
)

// The status page (ac_status.html) and the tracker pages get their live values
// from one Server-Sent Events stream, /realtime/statusEvents. publishStatus
// builds a statusMessage every second from the values the controller already
// holds in memory and pushes it whenever something has changed, and at least
// every statusHeartbeat so pages can tell the controller is still alive.

const (
	statusInterval  = time.Second
	statusHeartbeat = 10 * time.Second
	solarInterval   = 5 * time.Second
)

var statusHub = newEventHub()

// TemperatureReadings holds the latest temperatures from the ESP sensors in
// tenths of a degree C, as logged to the temperatures table.
type TemperatureReadings struct {
	SolarCollector int16
	SolarInlet     int16
	SolarOutlet    int16
	HotTankTop     int16
	HotTankMiddle  int16
	HotTankBottom  int16

	ColdTankBottom     int16 // the buffer tank
	ColdTankTop        int16
	ColdTankMiddle     int16
	MatsInput          int16
	MatsOutput         int16
	DehumidifierOutput int16

	GroundLoopOut int16
	ChillerOut    int16
	FromHouse     int16
	Bypass        int16
	GroundLoopIn  int16
	ToHouse       int16
}

var (
	liveTempsMu sync.Mutex
	liveTemps   TemperatureReadings
	liveTempsAt time.Time

	liveSolarMu sync.Mutex
	liveSolar   *SolarStrings
	liveSolarAt time.Time
)

// setLiveTemperatures records the latest temperature readings for the status stream.
func setLiveTemperatures(t TemperatureReadings) {
	liveTempsMu.Lock()
	defer liveTempsMu.Unlock()
	liveTemps = t
	liveTempsAt = time.Now()
}

var solarClient = &http.Client{Timeout: 3 * time.Second}

// readSolarStrings fetches the string inverter readings from the solar
// monitor on port 8081.
func readSolarStrings() (*SolarStrings, error) {
	resp, err := solarClient.Get("http://localhost:8081")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var s SolarStrings
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// pollSolarStrings keeps liveSolar up to date while anyone is watching the
// status stream.
func pollSolarStrings() {
	tick := time.NewTicker(solarInterval)
	defer tick.Stop()
	for range tick.C {
		if statusHub.Clients() == 0 {
			continue
		}
		s, err := readSolarStrings()
		if err != nil {
			log.Print("Solar strings - ", err)
			continue
		}
		liveSolarMu.Lock()
		liveSolar, liveSolarAt = s, time.Now()
		liveSolarMu.Unlock()
		SolarProduction.power = s.TotalPower
		SolarProduction.logged = time.Now()
	}
}

type statusTesla struct {
	Current float32    `json:"current"` // A, all cars
	MaxAmps float32    `json:"maxAmps"` // A available to the cars
	Cars    []teslaCar `json:"cars"`
}

type teslaCar struct {
	Current float32 `json:"current"`
	MaxAmps float32 `json:"maxAmps"`
	Status  string  `json:"status"`
	Stopped bool    `json:"stopped"`
}

// statusMessage is one event on /realtime/statusEvents. Temperatures are in
// degrees C. Temps, Solar and EG4 are omitted until the controller has a
// reading for them; their Time fields say how old the reading is.
type statusMessage struct {
	Time int64 `json:"time"` // Unix ms when the message was built

	Inverter struct {
		Frequency        float64 `json:"frequency"`
		VSetpoint        float32 `json:"vSetpoint"`
		VBatt            float32 `json:"vBatt"`
		IBatt            float32 `json:"iBatt"` // Sunny Island DC current, + discharging
		IBattAvg         float32 `json:"iBattAvg"`
		SOC              float32 `json:"soc"`
		Power            float32 `json:"power"` // kW
		GeneratorRunning bool    `json:"generatorRunning"`
	} `json:"inverter"`

	BMS struct {
		VBat                float64 `json:"vBat"`
		IBat                float64 `json:"iBat"` // + charging
		TBat                float64 `json:"tBat"`
		ChargeVolts         float64 `json:"chargeVolts"`
		DischargeVolts      float64 `json:"dischargeVolts"`
		ChargeCurrentMax    float64 `json:"chargeCurrentMax"`
		DischargeCurrentMax float64 `json:"dischargeCurrentMax"`
		SOC                 uint16  `json:"soc"`
		SOH                 uint16  `json:"soh"`
	} `json:"bms"`

	Tesla statusTesla `json:"tesla"`

	Equipment struct {
		Fans bool `json:"fans"`
		AC   bool `json:"ac"`
	} `json:"equipment"`

	SolarPump       uint8  `json:"solarPump"`       // %
	SolarPumpState  string `json:"solarPumpState"`  // Off, Priming or Running
	SolarPumpReason string `json:"solarPumpReason"` // why, e.g. "tank full (92.3 °C)"

	Temps *statusTemps `json:"temps,omitempty"`

	// Solar strings in the order the solar monitor reports them (A, B, ... K).
	Solar *statusSolar `json:"solar,omitempty"`

	EG4 *statusEG4 `json:"eg4,omitempty"`
}

// statusTemps holds temperatures in degrees C.
type statusTemps struct {
	Time               int64   `json:"time"`
	SolarCollector     float32 `json:"solarCollector"`
	SolarInlet         float32 `json:"solarInlet"`
	SolarOutlet        float32 `json:"solarOutlet"`
	HotTankTop         float32 `json:"hotTankTop"`
	HotTankMiddle      float32 `json:"hotTankMiddle"`
	HotTankBottom      float32 `json:"hotTankBottom"`
	BufferTankTop      float32 `json:"bufferTankTop"`
	BufferTankMiddle   float32 `json:"bufferTankMiddle"`
	BufferTankBottom   float32 `json:"bufferTankBottom"`
	MatsInput          float32 `json:"matsInput"`
	MatsOutput         float32 `json:"matsOutput"`
	DehumidifierOutput float32 `json:"dehumidifierOutput"`
	GroundLoopIn       float32 `json:"groundLoopIn"`
	GroundLoopOut      float32 `json:"groundLoopOut"`
	ToHouse            float32 `json:"toHouse"`
	FromHouse          float32 `json:"fromHouse"`
	ChillerOut         float32 `json:"chillerOut"`
	Bypass             float32 `json:"bypass"`
}

// statusSolar holds the solar strings in the order the solar monitor reports
// them (A, B, ... K).
type statusSolar struct {
	Time    int64         `json:"time"`
	Total   float32       `json:"total"` // W
	Strings []solarString `json:"strings"`
}

type solarString struct {
	Watts float32 `json:"watts"`
	Volts float32 `json:"volts"`
	Amps  float32 `json:"amps"`
}

// statusEG4 is a summary of the latest EG4 reading; eg4.html has the detail.
type statusEG4 struct {
	Time         int64    `json:"time"`
	PV           float64  `json:"pv"`
	ACCouple     float64  `json:"acCouple"`
	Load         float64  `json:"load"`
	Battery      float64  `json:"battery"` // + charging
	SOC          float64  `json:"soc"`
	NotAnswering []string `json:"notAnswering"`
}

func ms(t time.Time) int64 { return t.UnixMilli() }

// buildStatus collects the current values. Time is left zero so two messages
// can be compared to see whether anything changed.
func buildStatus() *statusMessage {
	var m statusMessage

	inv := &m.Inverter
	inv.Frequency = iValues.GetFrequency()
	inv.VSetpoint = iValues.GetSetPoint()
	inv.VBatt = iValues.GetVolts()
	inv.IBatt = iValues.GetAmps()
	inv.IBattAvg = iValues.GetAvgAmps()
	inv.SOC = iValues.GetSOC()
	inv.Power = iValues.GetPower()

	iValues.mu.Lock()
	inv.GeneratorRunning = iValues.AutoGn
	b := &m.BMS
	b.VBat, b.IBat, b.TBat = iValues.bmsVBat, iValues.bmsIBat, iValues.bmsTBat
	b.ChargeVolts, b.DischargeVolts = iValues.bmsChargeVolts, iValues.bmsDischargeVolts
	b.ChargeCurrentMax, b.DischargeCurrentMax = iValues.bmsChargeCurrentMax, iValues.bmsDichargeCurrentMax
	b.SOC, b.SOH = iValues.bmsSOC, iValues.bmsSOH
	iValues.mu.Unlock()

	m.Tesla.Current, m.Tesla.MaxAmps = TeslaParameters.GetValues()
	m.Tesla.Cars = make([]teslaCar, len(slaves))
	for i := range slaves {
		c := &m.Tesla.Cars[i]
		c.Current = float32(slaves[i].GetCurrent()) / 100
		c.MaxAmps = float32(slaves[i].GetAllowed()) / 100
		c.Status = slaves[i].GetStatus()
		c.Stopped = slaves[i].GetStopped()
	}

	m.Equipment.Fans = rpio.ReadPin(17) == rpio.Low
	m.Equipment.AC = rpio.ReadPin(27) == rpio.Low
	if solarPump != nil {
		m.SolarPump = solarPump.Speed()
		m.SolarPumpState, m.SolarPumpReason = solarPump.State()
	}

	liveTempsMu.Lock()
	t, tAt := liveTemps, liveTempsAt
	liveTempsMu.Unlock()
	if !tAt.IsZero() {
		c := func(v int16) float32 { return float32(v) / 10 }
		m.Temps = &statusTemps{ms(tAt),
			c(t.SolarCollector), c(t.SolarInlet), c(t.SolarOutlet),
			c(t.HotTankTop), c(t.HotTankMiddle), c(t.HotTankBottom),
			c(t.ColdTankTop), c(t.ColdTankMiddle), c(t.ColdTankBottom),
			c(t.MatsInput), c(t.MatsOutput), c(t.DehumidifierOutput),
			c(t.GroundLoopIn), c(t.GroundLoopOut), c(t.ToHouse), c(t.FromHouse), c(t.ChillerOut), c(t.Bypass)}
	}

	liveSolarMu.Lock()
	s, sAt := liveSolar, liveSolarAt
	liveSolarMu.Unlock()
	if s != nil {
		m.Solar = &statusSolar{Time: ms(sAt), Total: s.TotalPower}
		for _, str := range s.SolarStrings {
			m.Solar.Strings = append(m.Solar.Strings, solarString{str.Power, str.Voltage, str.Current})
		}
	}

	latestEG4Mu.RLock()
	sr := latestEG4
	latestEG4Mu.RUnlock()
	if sr != nil {
		m.EG4 = &statusEG4{Time: ms(sr.Time), PV: float64(sr.PVPower), ACCouple: float64(sr.ACCouplePower),
			Load: float64(sr.LoadPower), Battery: float64(sr.BatteryPower), SOC: float64(sr.SOC), NotAnswering: []string{}}
		for name := range sr.Errors {
			m.EG4.NotAnswering = append(m.EG4.NotAnswering, name)
		}
	}
	return &m
}

// publishStatus pushes a status message every second when something has
// changed, and every statusHeartbeat regardless.
func publishStatus() {
	go pollSolarStrings()

	var last []byte
	var lastSent time.Time
	tick := time.NewTicker(statusInterval)
	defer tick.Stop()
	for range tick.C {
		m := buildStatus()
		cmp, err := json.Marshal(m)
		if err != nil {
			log.Print("publishStatus() - ", err)
			continue
		}
		if bytes.Equal(cmp, last) && time.Since(lastSent) < statusHeartbeat {
			continue
		}
		last = cmp
		lastSent = time.Now()
		m.Time = ms(lastSent)
		msg, err := json.Marshal(m)
		if err != nil {
			log.Print("publishStatus() - ", err)
			continue
		}
		statusHub.Publish(msg)
	}
}

// statusEvents streams status messages as Server-Sent Events.
func statusEvents(w http.ResponseWriter, r *http.Request) {
	statusHub.ServeHTTP(w, r)
}

// getStatus returns the latest status message once, for browsers that cannot
// use the event stream.
func getStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	msg := statusHub.Latest()
	if msg == nil {
		http.Error(w, "No status yet.", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := fmt.Fprintf(w, "%s\n", msg); err != nil {
		log.Print(err)
	}
}
