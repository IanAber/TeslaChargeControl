package main

import (
	"encoding/json"
	"fmt"
	"github.com/stianeikeland/go-rpio"
	"io"
	"log"
	"net/http"
	"time"
)

type PinData struct {
	Port0 bool `json:"port0"`
	Port1 bool `json:"port1"`
	Port2 bool `json:"port2"`
	Port3 bool `json:"port3"`
	Port4 bool `json:"port4"`
	Port5 bool `json:"port5"`
	Port6 bool `json:"port6"`
}

type PumpData struct {
	Logged string `json:"logged"`
	PSOP   int    `json:"PSOP"`
}

type TempData struct {
	Logged             string  `json:"logged"`
	DehumidifierOutput float32 `json:"DehumidifierOutput"`
	MatsInput          float32 `json:"MatsInput"`
	MatsOutput         float32 `json:"MatsOutput"`
	CondenserIn        float32 `json:"CondenserIn"`
	CondenserOut       float32 `json:"CondenserOut"`
	GeneratorIn        float32 `json:"GeneratorIn"`
	GeneratorOut       float32 `json:"GeneratorOut"`
	EvaporatorIn       float32 `json:"EvaporatorIn"`
	EvaporatorOut      float32 `json:"EvaporatorOut"`
	HotTankTop         float32 `json:"HotTankTop"`
	HotTankBottom      float32 `json:"HotTankBottom"`
	HotTankMiddle      float32 `json:"HotTankMiddle"`
	BufferTankBottom   float32 `json:"BufferTankBottom"`
	BufferTankTop      float32 `json:"BufferTankTop"`
	BufferTankMiddle   float32 `json:"BufferTankMiddle"`
	SolarCollector     float32 `json:"SolarCollector"`
	SolarInlet         float32 `json:"SolarInlet"`
	SolarOutlet        float32 `json:"SolarOutlet"`
}

type SolarData struct {
	Logged time.Time `json:"logged"`
	A      float32   `json:"A"`
	B      float32   `json:"B"`
	C      float32   `json:"C"`
	D      float32   `json:"D"`
	E      float32   `json:"E"`
	F      float32   `json:"F"`
	G      float32   `json:"G"`
	H      float32   `json:"H"`
	I      float32   `json:"I"`
	J      float32   `json:"J"`
	K      float32   `json:"K"`
}

type SolarStrings struct {
	SolarStrings []struct {
		Power   float32 `json:"watts"`
		Voltage float32 `json:"volts"`
		Current float32 `json:"amps"`
	} `json:"strings"`
	TotalPower float32 `json:"total"`
}

// GetTimeRange returns the start and end times passed as query parameters.

func GetTimeRange(r *http.Request) (start time.Time, end time.Time, err error) {
	params := r.URL.Query()
	values := params["start"]
	if len(values) != 1 {
		err = fmt.Errorf("exactly one 'start=' value must be supplied for start time")
		return
	}
	timeVal, err := time.Parse("2006-1-2 15:4", values[0])
	if err != nil {
		return
	} else {
		start = timeVal
	}

	values = params["end"]
	if len(values) != 1 {
		err = fmt.Errorf("exactly one 'start=' value must be supplied for start time")
		return
	}
	timeVal, err = time.Parse("2006-1-2 15:4", values[0])
	if err != nil {
		return
	} else {
		end = timeVal
	}
	//log.Println("Date/time requested from ", start, " to ", end)
	return
}

/*
*
getSolarStringData fetches the data from the string inverters
*/
func getSolarStringData(data *SolarData) {
	var stringData SolarStrings

	if resp, err := http.Get("http://localhost:8081"); err != nil {
		log.Print(err)
	} else {
		defer func() {
			if err = resp.Body.Close(); err != nil {
				log.Print(err)
			}
		}()

		if body, err := io.ReadAll(resp.Body); err != nil {
			log.Print(err)
		} else {
			if err := json.Unmarshal(body, &stringData); err != nil {
				log.Print(err)
			} else {
				if data != nil {
					for idx, str := range stringData.SolarStrings {
						switch idx {
						case 0:
							data.A = str.Power
						case 1:
							data.B = str.Power
						case 2:
							data.C = str.Power
						case 3:
							data.D = str.Power
						case 4:
							data.E = str.Power
						case 5:
							data.F = str.Power
						case 6:
							data.G = str.Power
						case 7:
							data.H = str.Power
						case 8:
							data.I = str.Power
						case 9:
							data.J = str.Power
						case 10:
							data.K = str.Power
						default:
							log.Print("We got more than 11 strings of solar data. Logging only the first 11")
						}
					}
					data.Logged = time.Now()
				}
				SolarProduction.power = stringData.TotalPower
				SolarProduction.logged = time.Now()
			}
		}
	}
}

/*
*
getData returns the data for the main AC_Status page.
*/
func getData(w http.ResponseWriter, _ *http.Request) {

	var result struct {
		Pins  PinData   `json:"pins"`
		Pumps PumpData  `json:"pumps"`
		Temps TempData  `json:"temps"`
		Solar SolarData `json:"solar"`
	}

	result.Pins.Port0 = rpio.ReadPin(17) == rpio.Low
	result.Pins.Port2 = rpio.ReadPin(27) == rpio.Low
	result.Pins.Port3 = rpio.ReadPin(22) == rpio.High
	result.Pins.Port4 = rpio.ReadPin(23) == rpio.Low
	result.Pins.Port5 = rpio.ReadPin(6) == rpio.High
	result.Pins.Port6 = rpio.ReadPin(24) == rpio.High

	getSolarStringData(&result.Solar)

	if rows, err := pDB.Query(`SELECT Logged, MatsInput, MatsOutput, 
       					CondenserIn, CondenserOut, GeneratorIn, GeneratorOut, EvaportatorIn, EvaporatorOut,
       					HotTankTop, HotTankMiddle, HotTankBottom, BufferTankTop, BufferTankMiddle, BufferTankBottom, 
       					SolarCollector, SolarInlet, SolarOutlet, DehumidifierOutput
					FROM temperatures ORDER BY Logged DESC LIMIT 1`); err != nil {
		ReturnJSONError(w, "Temperature Data", err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		if rows.Next() {
			if err := rows.Scan(&result.Temps.Logged, &result.Temps.MatsInput, &result.Temps.MatsOutput,
				&result.Temps.CondenserIn, &result.Temps.CondenserOut, &result.Temps.GeneratorIn, &result.Temps.GeneratorOut,
				&result.Temps.EvaporatorIn, &result.Temps.EvaporatorOut,
				&result.Temps.HotTankTop, &result.Temps.HotTankMiddle, &result.Temps.HotTankBottom,
				&result.Temps.BufferTankTop, &result.Temps.BufferTankMiddle, &result.Temps.BufferTankBottom,
				&result.Temps.SolarCollector, &result.Temps.SolarInlet, &result.Temps.SolarOutlet, &result.Temps.DehumidifierOutput); err != nil {
				ReturnJSONError(w, "Temperature Data", err, http.StatusInternalServerError, true)
				return
			}
		}
	}
	if rows, err := pDB.Query("select logged, pump_power FROM solar_pump ORDER BY logged DESC LIMIT 1"); err != nil {
		ReturnJSONError(w, "Pump Data", err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		if rows.Next() {
			if err := rows.Scan(&result.Pumps.Logged, &result.Pumps.PSOP); err != nil {
				ReturnJSONError(w, "Pump Data", err, http.StatusInternalServerError, true)
				return
			}
		}
	}
	if jBytes, err := json.Marshal(result); err != nil {
		ReturnJSONError(w, "Data Marshal", err, http.StatusInternalServerError, true)
		return
	} else {
		_, err := fmt.Fprintf(w, string(jBytes))
		if err != nil {
			log.Println(err)
		}
	}
}

type BufferTankValue struct {
	Logged string  `json:"logged"`
	Tsc0   float32 `json:"TSC0"`
	Tsc1   float32 `json:"TSC1"`
	Tsc2   float32 `json:"TSC2"`
}

// getBufferTankData returns the set of buffer tank values between the provided start and end times as a JSON array
//
//	{
//		"logged":string
//		"TSC0":float
//		"TSC1":float
//		"TSC2":float
//	}
func getBufferTankData(w http.ResponseWriter, r *http.Request) {
	var Results []*BufferTankValue
	const DeviceString = "Buffer Tank Data"

	start, end, err := GetTimeRange(r)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, false)
		return
	}

	if rows, err := pDB.Query(`select min(unix_timestamp(logged)) as logged, avg(BufferTankBottom), avg(BufferTankTop), avg(BufferTankMiddle)
					FROM temperatures WHERE logged BETWEEN ? AND ? GROUP BY unix_timestamp(logged) DIV 60`, start, end); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		for rows.Next() {
			result := new(BufferTankValue)
			if err := rows.Scan(&result.Logged, &result.Tsc0, &result.Tsc1, &result.Tsc2); err != nil {
				ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
				return
			}
			result.Tsc0 /= 10.0
			result.Tsc1 /= 10.0
			result.Tsc2 /= 10.0
			Results = append(Results, result)
		}
		if resultJSON, err := json.Marshal(Results); err != nil {
			ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		} else {
			if _, err := fmt.Fprintf(w, string(resultJSON)); err != nil {
				log.Print(err)
			}
		}
	}
}

type HotTankValue struct {
	Logged string  `json:"logged"`
	Tsh0   float32 `json:"TSH0"`
	Tsh1   float32 `json:"TSH1"`
	Tsh2   float32 `json:"TSH2"`
	Mean   float32 `json:"mean"`
}

func getHotTankData(w http.ResponseWriter, r *http.Request) {
	var Results []*HotTankValue
	const DeviceString = "Hot Tank Data"

	start, end, err := GetTimeRange(r)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, false)
		return
	}

	if rows, err := pDB.Query(`select min(unix_timestamp(logged)) as logged, AVG(HotTankTop), AVG(HotTankBottom), AVG(HotTankMiddle)
					from temperatures where logged between ? and ? group by unix_timestamp(logged) DIV 60`, start, end); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		for rows.Next() {
			result := new(HotTankValue)
			if err := rows.Scan(&result.Logged, &result.Tsh0, &result.Tsh1, &result.Tsh2); err != nil {
				ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
				return
			}
			// Calculate the weighted mean temperature

			result.Mean = ((result.Tsh0 * 3) + result.Tsh1 + (result.Tsh2 * 2)) / 60.0
			result.Tsh0 /= 10.0
			result.Tsh1 /= 10.0
			result.Tsh2 /= 10.0
			Results = append(Results, result)
		}
		if resultJSON, err := json.Marshal(Results); err != nil {
			ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		} else {
			if _, err := fmt.Fprintf(w, string(resultJSON)); err != nil {
				log.Print(err)
			}
		}
	}
}

type ACValue struct {
	Logged             string  `json:"logged"`
	MatsInput          float32 `json:"MatsInput"`
	MatsOutput         float32 `json:"MatsOutput"`
	DehumidifierOutput float32 `json:"DehumidifierOutput"`
}

func getACTempData(w http.ResponseWriter, r *http.Request) {
	var Results []*ACValue
	const DeviceString = "Hot Tank Data"

	start, end, err := GetTimeRange(r)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, false)
		return
	}

	if rows, err := pDB.Query(`select min(unix_timestamp(logged)) as logged, AVG(MatsInput), AVG(MatsOutput), AVG(DehumidifierOutput)
					from temperatures where logged between ? and ? group by unix_timestamp(logged) DIV 60`, start, end); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		for rows.Next() {
			result := new(ACValue)
			if err := rows.Scan(&result.Logged, &result.MatsInput, &result.MatsOutput, &result.DehumidifierOutput); err != nil {
				ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
				return
			}
			// Calculate the weighted mean temperature

			result.MatsInput /= 10.0
			result.MatsOutput /= 10.0
			result.DehumidifierOutput /= 10.0
			Results = append(Results, result)
		}
		if resultJSON, err := json.Marshal(Results); err != nil {
			ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		} else {
			if _, err := fmt.Fprintf(w, string(resultJSON)); err != nil {
				log.Print(err)
			}
		}
	}
}

type SolarTempValue struct {
	Logged         string  `json:"logged"`
	SolarCollector float32 `json:"SolarCollector"`
	SolarInlet     float32 `json:"SolarInlet"`
	SolarOutlet    float32 `json:"SolarOutlet"`
}

func getSolarTempData(w http.ResponseWriter, r *http.Request) {
	var Results []*SolarTempValue
	const DeviceString = "Solar Temperature Data"

	start, end, err := GetTimeRange(r)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, false)
		return
	}

	if rows, err := pDB.Query(`select unix_timestamp(logged) as logged, SolarCollector, SolarInlet, SolarOutlet
								from temperatures where Logged between ? AND ?`, start, end); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		for rows.Next() {
			result := new(SolarTempValue)
			if err := rows.Scan(&result.Logged, &result.SolarCollector, &result.SolarInlet, &result.SolarOutlet); err != nil {
				ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
				return
			}
			// Calculate the weighted mean temperature

			result.SolarInlet /= 10.0
			result.SolarOutlet /= 10.0
			result.SolarCollector /= 10.0
			Results = append(Results, result)
		}
		if resultJSON, err := json.Marshal(Results); err != nil {
			ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		} else {
			if _, err := fmt.Fprintf(w, string(resultJSON)); err != nil {
				log.Print(err)
			}
		}
	}
}

type SolarFullData struct {
	Logged float64 `json:"logged"`
	AmpsA  float32 `json:"amps_a"`
	VoltsA float32 `json:"volts_a"`
	WattsA float32 `json:"watts_a"`
	AmpsB  float32 `json:"amps_b"`
	VoltsB float32 `json:"volts_b"`
	WattsB float32 `json:"watts_b"`
	AmpsC  float32 `json:"amps_c"`
	VoltsC float32 `json:"volts_c"`
	WattsC float32 `json:"watts_c"`
	AmpsD  float32 `json:"amps_d"`
	VoltsD float32 `json:"volts_d"`
	WattsD float32 `json:"watts_d"`
	AmpsE  float32 `json:"amps_e"`
	VoltsE float32 `json:"volts_e"`
	WattsE float32 `json:"watts_e"`
	AmpsF  float32 `json:"amps_f"`
	VoltsF float32 `json:"volts_f"`
	WattsF float32 `json:"watts_f"`
	AmpsG  float32 `json:"amps_g"`
	VoltsG float32 `json:"volts_g"`
	WattsG float32 `json:"watts_g"`
	AmpsH  float32 `json:"amps_h"`
	VoltsH float32 `json:"volts_h"`
	WattsH float32 `json:"watts_h"`
	AmpsI  float32 `json:"amps_i"`
	VoltsI float32 `json:"volts_i"`
	WattsI float32 `json:"watts_i"`
	AmpsJ  float32 `json:"amps_j"`
	VoltsJ float32 `json:"volts_j"`
	WattsJ float32 `json:"watts_j"`
	AmpsK  float32 `json:"amps_k"`
	VoltsK float32 `json:"volts_k"`
	WattsK float32 `json:"watts_k"`
}

func getSolar(w http.ResponseWriter, r *http.Request) {
	const SQLDirect = `select unix_timestamp(solar_production.logged) as logged,
		solar_production.amps_a, solar_production.volts_a, solar_production.watts_a,
		solar_production.amps_b, solar_production.volts_b, solar_production.watts_b,
		solar_production.amps_c, solar_production.volts_c, solar_production.watts_c,
		solar_production.amps_d, solar_production.volts_d, solar_production.watts_d,
		solar_production.amps_e, solar_production.volts_e, solar_production.watts_e,
		solar_production.amps_f, solar_production.volts_f, solar_production.watts_f,
		solar_production.amps_g, solar_production.volts_g, solar_production.watts_g,
		solar_production.amps_h, solar_production.volts_h, solar_production.watts_h,
		solar_production.amps_i, solar_production.volts_i, solar_production.watts_i,
		solar_production.amps_j, solar_production.volts_j, solar_production.watts_j,
		solar_production.amps_k, solar_production.volts_k, solar_production.watts_k
	from solar_production
	where logged between ? and ?`

	const SQLBy5Mins = `select max(logged) as logged,
    avg(amps_a) as amps_a, avg(volts_a) as volts_a, avg(watts_a) as watts_a,
    avg(amps_b) as amps_b, avg(volts_b) as volts_b, avg(watts_b) as watts_b,
    avg(amps_c) as amps_c, avg(volts_c) as volts_c, avg(watts_c) as watts_c,
    avg(amps_d) as amps_d, avg(volts_d) as volts_d, avg(watts_d) as watts_d,
    avg(amps_e) as amps_e, avg(volts_e) as volts_e, avg(watts_e) as watts_e,
    avg(amps_f) as amps_f, avg(volts_f) as volts_f, avg(watts_f) as watts_f,
    avg(amps_g) as amps_g, avg(volts_g) as volts_g, avg(watts_g) as watts_g,
    avg(amps_h) as amps_h, avg(volts_h) as volts_h, avg(watts_h) as watts_h,
    avg(amps_i) as amps_i, avg(volts_i) as volts_i, avg(watts_i) as watts_i,
    avg(amps_j) as amps_j, avg(volts_j) as volts_j, avg(watts_j) as watts_j,
    avg(amps_k) as amps_k, avg(volts_k) as volts_k, avg(watts_k) as watts_k
  from (
        select unix_timestamp(solar_production.logged) as logged,
            solar_production.amps_a, solar_production.volts_a, solar_production.watts_a,
            solar_production.amps_b, solar_production.volts_b, solar_production.watts_b,
            solar_production.amps_c, solar_production.volts_c, solar_production.watts_c,
            solar_production.amps_d, solar_production.volts_d, solar_production.watts_d,
            solar_production.amps_e, solar_production.volts_e, solar_production.watts_e,
            solar_production.amps_f, solar_production.volts_f, solar_production.watts_f,
            solar_production.amps_g, solar_production.volts_g, solar_production.watts_g,
            solar_production.amps_h, solar_production.volts_h, solar_production.watts_h,
            solar_production.amps_i, solar_production.volts_i, solar_production.watts_i,
            solar_production.amps_j, solar_production.volts_j, solar_production.watts_j,
            solar_production.amps_k, solar_production.volts_k, solar_production.watts_k
         from solar_production
        where logged between ? and ?) as solar
    group by logged DIV 300`
	var sql string
	var Results []*SolarFullData
	const DeviceString = "Solar Data"

	start, end, err := GetTimeRange(r)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, false)
		return
	}

	if end.Sub(start) > (time.Hour * 4) {
		sql = SQLBy5Mins
	} else {
		sql = SQLDirect
	}

	if rows, err := pDB.Query(sql, start, end); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		for rows.Next() {
			result := new(SolarFullData)
			if err := rows.Scan(&result.Logged,
				&result.AmpsA, &result.VoltsA, &result.WattsA,
				&result.AmpsB, &result.VoltsB, &result.WattsB,
				&result.AmpsC, &result.VoltsC, &result.WattsC,
				&result.AmpsD, &result.VoltsD, &result.WattsD,
				&result.AmpsE, &result.VoltsE, &result.WattsE,
				&result.AmpsF, &result.VoltsF, &result.WattsF,
				&result.AmpsG, &result.VoltsG, &result.WattsG,
				&result.AmpsH, &result.VoltsH, &result.WattsH,
				&result.AmpsI, &result.VoltsI, &result.WattsI,
				&result.AmpsJ, &result.VoltsJ, &result.WattsJ,
				&result.AmpsK, &result.VoltsK, &result.WattsK); err != nil {
				ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
				return
			}
			Results = append(Results, result)
		}
		if resultJSON, err := json.Marshal(Results); err != nil {
			ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		} else {
			if _, err := fmt.Fprintf(w, string(resultJSON)); err != nil {
				log.Print(err)
			}
		}
	}
}

type LoopTemps struct {
	Logged float64 `json:"logged"`
	TCHGI1 float64 `json:"TCHGI_1"`
	TCHGO1 float64 `json:"TCHGO_1"`
	TCHEI1 float64 `json:"TCHEI_1"`
	TCHEO1 float64 `json:"TCHEO_1"`
	TCHCI1 float64 `json:"TCHCI_1"`
	TCHCO1 float64 `json:"TCHCO_1"`
}

func getLoopTemps(w http.ResponseWriter, r *http.Request) {
	const SQLDirect = `SELECT UNIX_TIMESTAMP(Logged) AS Logged
            ,GeneratorIn / 10 AS GeneratorIn, GeneratorOut / 10 AS GeneratorOut
            ,EvaportatorIn / 10 AS EvaportatorIn, EvaporatorOut / 10 AS EvaporatorOut
            ,CondenserIn / 10 AS CondenserIn, CondenserOut / 10 AS CondenserOut
 		 FROM temperatures
 		WHERE logged BETWEEN ? AND ?`

	const SQLBy5Mins = `SELECT Logged
            ,AVG(GeneratorIn) / 10 AS GeneratorIn, AVG(GeneratorOut) / 10 AS GeneratorOut
            ,AVG(EvaportatorIn) / 10 AS EvaportatorIn, AVG(EvaporatorOut) / 10 AS EvaporatorOut
            ,AVG(CondenserIn) / 10 AS CondenserIn, AVG(CondenserOut) / 10 AS CondenserOut
	  FROM (
    	    SELECT UNIX_TIMESTAMP(Logged) AS Logged,
				GeneratorIn,GeneratorOut,
				EvaportatorIn, EvaporatorOut,
				CondenserIn, CondenserOut
		   FROM temperatures
          WHERE Logged BETWEEN ? AND ?) AS temps
	  GROUP BY Logged DIV 300`

	var sql string
	var Results []*LoopTemps
	const DeviceString = "Loop Temperature Data"

	start, end, err := GetTimeRange(r)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, false)
		return
	}

	if end.Sub(start) > (time.Hour * 4) {
		sql = SQLBy5Mins
	} else {
		sql = SQLDirect
	}

	if rows, err := pDB.Query(sql, start, end); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		for rows.Next() {
			result := new(LoopTemps)
			if err := rows.Scan(&result.Logged,
				&result.TCHGI1, &result.TCHGO1,
				&result.TCHEI1, &result.TCHEO1,
				&result.TCHCI1, &result.TCHCO1); err != nil {
				ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
				return
			}
			Results = append(Results, result)
		}
		if resultJSON, err := json.Marshal(Results); err != nil {
			ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		} else {
			if _, err := fmt.Fprintf(w, string(resultJSON)); err != nil {
				log.Print(err)
			}
		}
	}
}

type SolarPump struct {
	Logged float64 `json:"logged"`
	Power  float64 `json:"power"`
}

func getPumpData(w http.ResponseWriter, r *http.Request) {
	const SQLSolarDirect = `SELECT UNIX_TIMESTAMP(logged) AS logged, pump_power FROM solar_pump WHERE logged BETWEEN ? AND ?`

	var sql string
	var Results []*SolarPump

	const DeviceString = "Loop Temperature Data"

	start, end, err := GetTimeRange(r)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, false)
		return
	}

	sql = SQLSolarDirect

	if rows, err := pDB.Query(sql, start, end); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		for rows.Next() {
			result := new(SolarPump)
			if err := rows.Scan(&result.Logged, &result.Power); err != nil {
				ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
				return
			}
			Results = append(Results, result)
		}
	}

	if resultJSON, err := json.Marshal(Results); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
	} else {
		if _, err := fmt.Fprintf(w, string(resultJSON)); err != nil {
			log.Print(err)
		}
	}
}

type CurrentData struct {
	Logged    float64 `json:"logged"`
	Amps      float64 `json:"amps"`
	Volts     float64 `json:"volts"`
	SOC       float64 `json:"state_of_charge"`
	Hertz     float64 `json:"frequency"`
	VSetpoint float64 `json:"setpoint"`
}

func getCurrent(w http.ResponseWriter, r *http.Request) {
	const SQLDirect = `SELECT UNIX_TIMESTAMP(logged) AS logged
							, iBatt AS amps
							, vBatt AS volts
							, state_of_charge AS soc
							, frequency AS frequency
							, vSetpoint AS setpoint
						 FROM inverter_values
						WHERE logged BETWEEN ? AND ?
						ORDER BY logged DESC`

	const SQLBy5Mins = `SELECT MIN(UNIX_TIMESTAMP(logged)) AS logged
							, AVG(iBatt) AS amps
							, AVG(vBatt) AS volts
							, AVG(state_of_charge) AS soc
							, AVG(frequency) AS frequency
							, AVG(vSetpoint) AS setpoint
						 FROM inverter_values
						WHERE logged BETWEEN ? AND ?
						GROUP BY UNIX_TIMESTAMP(logged) DIV 300
						ORDER BY logged DESC`

	const SQLBy1hr = `SELECT MIN(UNIX_TIMESTAMP(logged)) AS logged
							, AVG(iBatt) AS amps
							, AVG(vBatt) AS volts
							, AVG(state_of_charge) AS soc
							, AVG(frequency) AS frequency
							, AVG(vSetpoint) AS setpoint
						 FROM inverter_values
						WHERE logged BETWEEN ? AND ?
						GROUP BY UNIX_TIMESTAMP(logged) DIV 3600
						ORDER BY logged DESC`

	var sql string
	var Results []*CurrentData
	const DeviceString = "Current Data"

	start, end, err := GetTimeRange(r)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, false)
		return
	}

	if end.Sub(start) > (time.Hour * 4) {
		if end.Sub(start) > (time.Hour * 47) {
			sql = SQLBy1hr
		} else {
			sql = SQLBy5Mins
		}
	} else {
		sql = SQLDirect
	}

	if rows, err := pDB.Query(sql, start, end); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		for rows.Next() {
			result := new(CurrentData)
			if err := rows.Scan(&result.Logged, &result.Amps, &result.Volts, &result.SOC, &result.Hertz, &result.VSetpoint); err != nil {
				ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
				return
			}
			Results = append(Results, result)
		}
		if resultJSON, err := json.Marshal(Results); err != nil {
			ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		} else {
			if _, err := fmt.Fprintf(w, string(resultJSON)); err != nil {
				log.Print(err)
			}
		}
	}
}

type TeslaData struct {
	Logged   float64 `json:"logged"`
	Setpoint uint8   `json:"iSetpoint"`
	Charging float64 `json:"iCharging"`
}

func getTeslaData(w http.ResponseWriter, r *http.Request) {
	var Results []*TeslaData
	const rqst = `select *
		from (select unix_timestamp(?) as logged
		, iSetpoint
		, iCharging
		from tesla_values
		where logged < ?
		order by tesla_values.logged desc limit 1) s1
		UNION
		select unix_timestamp(logged) as logged
			, iSetpoint
			, iCharging
			from tesla_values
			where logged between ? and ?`

	const DeviceString = "Tesla Data"

	start, end, err := GetTimeRange(r)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, false)
		return
	}

	if rows, err := pDB.Query(rqst, start, start, start, end); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		for rows.Next() {
			result := new(TeslaData)
			if err := rows.Scan(&result.Logged, &result.Setpoint, &result.Charging); err != nil {
				ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
				return
			}
			Results = append(Results, result)
		}
		if resultJSON, err := json.Marshal(Results); err != nil {
			ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		} else {
			if _, err := fmt.Fprintf(w, string(resultJSON)); err != nil {
				log.Print(err)
			}
		}
	}
}

type VoltageData struct {
	Logged    float64 `json:"logged"`
	VBatt     float64 `json:"volts"`
	VSetpoint float64 `json:"volts_setpoint"`
}

func getVoltageData(w http.ResponseWriter, r *http.Request) {
	var Results []*VoltageData
	const rqst = `select unix_timestamp(logged) as logged
                       , vBatt as volts
                       , vSetpoint as volts_setpoint
                   from inverter_values
                  where logged between ? and ?`

	const DeviceString = "Voltage Data"

	start, end, err := GetTimeRange(r)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, false)
		return
	}

	if rows, err := pDB.Query(rqst, start, end); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		for rows.Next() {
			result := new(VoltageData)
			if err := rows.Scan(&result.Logged, &result.VBatt, &result.VSetpoint); err != nil {
				ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
				return
			}
			Results = append(Results, result)
		}
		if resultJSON, err := json.Marshal(Results); err != nil {
			ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		} else {
			if _, err := fmt.Fprintf(w, string(resultJSON)); err != nil {
				log.Print(err)
			}
		}
	}
}

type HeaterData struct {
	Logged float64 `json:"logged"`
	Status string  `json:"status"`
}

func getHeaterData(w http.ResponseWriter, r *http.Request) {
	var Results []*HeaterData
	const rqst = `select unix_timestamp(logged) as logged, status from water_heater_operation where logged between ? and ?`

	const DeviceString = "Heater Data"

	start, end, err := GetTimeRange(r)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, false)
		return
	}

	if rows, err := pDB.Query(rqst, start, end); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		for rows.Next() {
			result := new(HeaterData)
			if err := rows.Scan(&result.Logged, &result.Status); err != nil {
				ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
				return
			}
			Results = append(Results, result)
		}
		if resultJSON, err := json.Marshal(Results); err != nil {
			ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		} else {
			if _, err := fmt.Fprintf(w, string(resultJSON)); err != nil {
				log.Print(err)
			}
		}
	}
}
