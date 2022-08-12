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
	Logged string  `json:"logged"`
	TOU    float32 `json:"TOU"`
	Tin1   float32 `json:"TIN_1"`
	Tin2   float32 `json:"TIN_2"`
	Tin3   float32 `json:"TIN_3"`
	Tin4   float32 `json:"TIN_4"`
	Tchci1 float32 `json:"TCHCI_1"`
	Tchco1 float32 `json:"TCHCO_1"`
	Tchgi1 float32 `json:"TCHGI_1"`
	Tchgo1 float32 `json:"TCHGO_1"`
	Tchei1 float32 `json:"TCHEI_1"`
	Tcheo1 float32 `json:"TCHEO_1"`
	Tsh0   float32 `json:"TSH_0"`
	Tsh1   float32 `json:"TSH_1"`
	Tsh2   float32 `json:"TSH_2"`
	Tsc0   float32 `json:"TSC_0"`
	Tsc1   float32 `json:"TSC_1"`
	Tsc2   float32 `json:"TSC_2"`
	Tsoc1  float32 `json:"TSOC_1"`
	Tsopi  float32 `json:"TSOPI"`
	Tsopo  float32 `json:"TSOPO"`
	Tsos   float32 `json:"TSOS"`
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

/**
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

/**
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
	//if rows, err := pDB.Query("select logged, watts_a, watts_b, watts_c, watts_d, watts_e, watts_f, watts_g, watts_h, watts_i, watts_j, watts_k from solar_production order by logged desc limit 1"); err != nil {
	//	ReturnJSONError(w, "Solar Data", err, http.StatusInternalServerError, true)
	//	return
	//} else {
	//	defer func() {
	//		if err := rows.Close(); err != nil {
	//			log.Print(err)
	//		}
	//	}()
	//	if rows.Next() {
	//		if err := rows.Scan(&result.Solar.Logged, &result.Solar.A, &result.Solar.B, &result.Solar.C, &result.Solar.D, &result.Solar.E, &result.Solar.F, &result.Solar.G, &result.Solar.H, &result.Solar.I, &result.Solar.J, &result.Solar.K); err != nil {
	//			ReturnJSONError(w, "Solar Data", err, http.StatusInternalServerError, true)
	//			return
	//		}
	//	}
	//}

	if rows, err := pDB.Query("select logged, TOU, ifnull(TIN_1, 0), ifnull(TIN_2, 0), ifnull(TIN_3, 0), ifnull(TIN_4, 0), TCHCI_1, TCHCO_1, TCHGI_1, TCHGO_1, TCHEI_1, TCHEO_1, TSH0, TSH1, TSH2, TSC0, TSC1, TSC2, TSOC_1, TSOPI, TSOPO, TSOS FROM chillii_analogue_input ORDER BY logged DESC LIMIT 1"); err != nil {
		ReturnJSONError(w, "Temperature Data", err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		if rows.Next() {
			if err := rows.Scan(&result.Temps.Logged, &result.Temps.TOU, &result.Temps.Tin1, &result.Temps.Tin2, &result.Temps.Tin3, &result.Temps.Tin4, &result.Temps.Tchci1, &result.Temps.Tchco1, &result.Temps.Tchgi1, &result.Temps.Tchgo1, &result.Temps.Tchei1, &result.Temps.Tcheo1, &result.Temps.Tsh0, &result.Temps.Tsh1, &result.Temps.Tsh2,
				&result.Temps.Tsc0, &result.Temps.Tsc1, &result.Temps.Tsc2, &result.Temps.Tsoc1, &result.Temps.Tsopi, &result.Temps.Tsopo, &result.Temps.Tsos); err != nil {
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

type ColdTankValue struct {
	Logged string  `json:"logged"`
	Tsc0   float32 `json:"TSC0"`
	Tsc1   float32 `json:"TSC1"`
	Tsc2   float32 `json:"TSC2"`
}

//getColdTankData returns the set of cold tank values between the provided start and end times as a JSON array
//{
//	"logged":string
//	"TSC0":float
//	"TSC1":float
//	"TSC2":float
//}
func getColdTankData(w http.ResponseWriter, r *http.Request) {
	var Results []*ColdTankValue
	var start time.Time
	var end time.Time
	const DeviceString = "Cold Tank Data"

	params := r.URL.Query()
	values := params["start"]
	if len(values) != 1 {
		ReturnJSONErrorString(w, DeviceString, "Exactly one 'start=' value must be supplied for start time", http.StatusBadRequest, false)
		return
	}
	if timeVal, err := time.Parse("2006-1-2 15:4", values[0]); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	} else {
		start = timeVal
	}

	values = params["end"]
	if len(values) != 1 {
		ReturnJSONErrorString(w, DeviceString, "Exactly one 'start=' value must be supplied for start time", http.StatusBadRequest, false)
		return
	}
	if timeVal, err := time.Parse("2006-1-2 15:4", values[0]); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	} else {
		end = timeVal
	}

	if rows, err := pDB.Query("select min(unix_timestamp(logged)) as logged, avg(TSC0), avg(TSC1), avg(TSC2) FROM chillii_analogue_input WHERE logged BETWEEN ? AND ? GROUP BY unix_timestamp(logged) DIV 60", start, end); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
		return
	} else {
		defer func() {
			if err := rows.Close(); err != nil {
				log.Print(err)
			}
		}()
		for rows.Next() {
			result := new(ColdTankValue)
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

//getHotTankData returns the set of hot tank values between the provided start and end times as a JSON array
//{
//	"logged":string
//	"TSH0":float
//	"TSH1":float
//	"TSH2":float
//	"mean":float
//}
func getHotTankData(w http.ResponseWriter, r *http.Request) {
	var Results []*HotTankValue
	var start time.Time
	var end time.Time
	const DeviceString = "Hot Tank Data"

	params := r.URL.Query()
	values := params["start"]
	if len(values) != 1 {
		ReturnJSONErrorString(w, "Cold Tank Data", "Exactly one 'start=' value must be supplied for start time", http.StatusBadRequest, false)
		return
	}
	if timeVal, err := time.Parse("2006-1-2 15:4", values[0]); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	} else {
		start = timeVal
	}

	values = params["end"]
	if len(values) != 1 {
		ReturnJSONErrorString(w, DeviceString, "Exactly one 'start=' value must be supplied for start time", http.StatusBadRequest, false)
		return
	}
	if timeVal, err := time.Parse("2006-1-2 15:4", values[0]); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	} else {
		end = timeVal
	}

	if rows, err := pDB.Query(`select min(unix_timestamp(logged)) as logged, AVG(TSH0), AVG(TSH1), AVG(TSH2) from chillii_analogue_input where logged between ? and ? group by unix_timestamp(logged) DIV 60`, start, end); err != nil {
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

//select unix_timestamp(`timestamp`) as `logged`,`TSOC_1` / 10 as TSOC_1,`TSOPI` / 10 as TSOPI,`TSOPO` / 10 as TSOPO,`TSOS` / 10 as TSOS
//from `chillii_analogue_input`
//where `timestamp` between

type SolarTempValue struct {
	Logged string  `json:"logged"`
	Tsoc1  float32 `json:"TSOC_1"`
	Tsopi  float32 `json:"TSOPI"`
	Tsopo  float32 `json:"TSOPO"`
	Tsos   float32 `json:"TSOS"`
}

//getHotTankData returns the set of hot tank values between the provided start and end times as a JSON array
//{
//	"logged":string
//	"TSOC_1":float
//	"TSOPI":float
//	"TSOPO":float
//	"TSOS":float
//}
func getSolarTempData(w http.ResponseWriter, r *http.Request) {
	var Results []*SolarTempValue
	var start time.Time
	var end time.Time
	const DeviceString = "Solar Temperature Data"

	params := r.URL.Query()
	values := params["start"]
	if len(values) != 1 {
		ReturnJSONErrorString(w, DeviceString, "Exactly one 'start=' value must be supplied for start time", http.StatusBadRequest, false)
		return
	}
	if timeVal, err := time.Parse("2006-1-2 15:4", values[0]); err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	} else {
		start = timeVal
	}

	values = params["end"]
	if len(values) != 1 {
		ReturnJSONErrorString(w, DeviceString, "Exactly one 'start=' value must be supplied for start time", http.StatusBadRequest, false)
		return
	}
	if timeVal, err := time.Parse("2006-1-2 15:4", values[0]); err != nil {
		ReturnJSONError(w, "Cold Tank Values", err, http.StatusBadRequest, true)
		return
	} else {
		end = timeVal
	}

	if rows, err := pDB.Query(`select unix_timestamp(logged) as logged,TSOC_1 as TSOC_1,TSOPI as TSOPI,TSOPO as TSOPO,TSOS as TSOS from chillii_analogue_input where logged between ? AND ?`, start, end); err != nil {
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
			if err := rows.Scan(&result.Logged, &result.Tsoc1, &result.Tsopi, &result.Tsopo, &result.Tsos); err != nil {
				ReturnJSONError(w, DeviceString, err, http.StatusInternalServerError, true)
				return
			}
			// Calculate the weighted mean temperature

			result.Tsoc1 /= 10.0
			result.Tsopi /= 10.0
			result.Tsopo /= 10.0
			result.Tsos /= 10.0
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
