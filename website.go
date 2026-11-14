package main

import (
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/gorilla/mux"
)

func setUpWebSite() {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/", getValues).Methods("GET")
	router.HandleFunc("/version", getVersion).Methods("GET")

	router.HandleFunc("/waterTemps", getWaterTemps).Methods("GET")
	router.HandleFunc("/mail", mailForm).Methods("GET")

	router.HandleFunc("/realtime/getdata", getData).Methods("GET")
	router.HandleFunc("/realtime/getbuffertank", getBufferTankData).Methods("GET")
	router.HandleFunc("/realtime/gethottank", getHotTankData).Methods("GET")
	router.HandleFunc("/realtime/getsolartemps", getSolarTempData).Methods("GET")
	router.HandleFunc("/realtime/getSolar", getSolar).Methods("GET")
	router.HandleFunc("/realtime/getLoopTemps", getLoopTemps).Methods("GET")
	router.HandleFunc("/realtime/getpumps", getPumpData).Methods("GET")
	router.HandleFunc("/realtime/getCurrent", getCurrent).Methods("GET")
	router.HandleFunc("/realtime/getTesla", getTeslaData).Methods("GET")
	router.HandleFunc("/realtime/getVoltage", getVoltageData).Methods("GET")
	router.HandleFunc("/realtime/getactemps", getACTempData).Methods("GET")
	router.HandleFunc("/realtime/getEG4", getEG4).Methods("GET")
	router.HandleFunc("/realtime/getEG4History", getEG4History).Methods("GET")
	router.HandleFunc("/realtime/eg4Events", eg4Events).Methods("GET")
	router.HandleFunc("/realtime/statusEvents", statusEvents).Methods("GET")
	router.HandleFunc("/realtime/getStatus", getStatus).Methods("GET")

	router.HandleFunc("/logging", showLoggingSelection).Methods("GET")
	router.HandleFunc("/enableLogging/iValues", enableIvalueLogging).Methods("GET")
	router.HandleFunc("/disableLogging/iValues", disableIvalueLogging).Methods("GET")
	router.HandleFunc("/enableLogging/Tesla", enableTeslaLogging).Methods("GET")
	router.HandleFunc("/disableLogging/Tesla", disableTeslaLogging).Methods("GET")
	router.HandleFunc("/enableLogging/solar", enableSolarLogging).Methods("GET")
	router.HandleFunc("/disableLogging/solar", disableSolarLogging).Methods("GET")

	fileServer := http.FileServer(neuteredFileSystem{http.Dir("/var/www/html")})
	router.PathPrefix("/").Handler(http.StripPrefix("/", fileServer))

	log.Fatal(http.ListenAndServe(":8080", router))
}

type neuteredFileSystem struct {
	fs http.FileSystem
}

func (nfs neuteredFileSystem) Open(path string) (http.File, error) {
	f, err := nfs.fs.Open(path)
	if err != nil {
		return nil, err
	}

	if s, err := f.Stat(); err != nil {
		log.Println(err)
	} else {
		if s.IsDir() {
			index := filepath.Join(path, "index.html")
			if _, err := nfs.fs.Open(index); err != nil {
				closeErr := f.Close()
				if closeErr != nil {
					return nil, closeErr
				}
				return nil, err
			}
		}
	}
	return f, nil
}

func getWaterTemps(w http.ResponseWriter, _ *http.Request) {
	// Get the hot tank temperature
	var temps struct {
		Hot          float32 `json:"hot"`
		Cold         float32 `json:"cold"`
		Dehumidifier float32 `json:"dehumidifier"`
		MatsInput    float32 `json:"matsInput"`
		MatsOutput   float32 `json:"matsOutput"`
	}
	var err = pDB.QueryRow(`SELECT (HotTankTop + HotTankBottom + HotTankMiddle) / 30 AS hotTemp,
										(BufferTankTop + BufferTankMiddle + BufferTankBottom) / 30 As coldTemp,
										DehumidifierOutput / 10 as dehumidifierOutput,
										MatsInput / 10 as matsInput,
										MatsOutput/ 10 as matsOutput 
									FROM temperatures
									WHERE Logged > date_add(now(), INTERVAL -5 MINUTE)
									ORDER BY Logged DESC LIMIT 1;`).Scan(&temps.Hot, &temps.Cold, &temps.Dehumidifier, &temps.MatsInput, &temps.MatsOutput)
	if err != nil {
		http.Error(w, "Failed to get the temperatures.", http.StatusInternalServerError)
		log.Printf("Error fetching tank temperatures from the database - %s", err)
		return
	}
	str, err := json.Marshal(temps)
	if err != nil {
		http.Error(w, "Failed to marshal the tempratures into a JSON object", http.StatusInternalServerError)
		log.Printf("Error marshalling temperatures - %s", err)
		return
	}
	_, err = fmt.Fprint(w, string(str))
	if err != nil {
		log.Println("getWaterTemps() - ", err)
	}
}

func enableTeslaLogging(w http.ResponseWriter, r *http.Request) {
	TeslaParameters.EnableLogging(true)
	for slaveIdx := range slaves {
		slaves[slaveIdx].EnableLogging(true)
	}
	showLoggingSelection(w, r)
}

func disableTeslaLogging(w http.ResponseWriter, r *http.Request) {
	TeslaParameters.EnableLogging(false)
	for slaveIdx := range slaves {
		slaves[slaveIdx].EnableLogging(false)
	}
	showLoggingSelection(w, r)
}

func enableIvalueLogging(w http.ResponseWriter, r *http.Request) {
	iValues.Log = true
	showLoggingSelection(w, r)
}

func disableIvalueLogging(w http.ResponseWriter, r *http.Request) {
	iValues.Log = false
	showLoggingSelection(w, r)
}

func enableSolarLogging(w http.ResponseWriter, r *http.Request) {
	logSolar = true
	showLoggingSelection(w, r)
}

func disableSolarLogging(w http.ResponseWriter, r *http.Request) {
	logSolar = false
	showLoggingSelection(w, r)
}

func getVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	b := getBuildInfo()
	_, err := fmt.Fprintf(w, `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <title>Cedar Technology System Manager</title>
  </head>
  <body>
    <h1>Cedar Technology System Manager</h1>
    <h2>Version %s</h2>
    <p>Commit %s</p>
    <p>Built with %s. Running since %s.</p>
  </body>
</html>`, html.EscapeString(b.String()), html.EscapeString(b.Revision), html.EscapeString(b.GoVersion),
		startTime.Format("2006-01-02 15:04:05 MST"))
	if err != nil {
		log.Println(err)
	}
}

// getValues /*
// Return a set of values as a JSON object for consumption by a WEB page dashboard.
func getValues(w http.ResponseWriter, _ *http.Request) {
	_, fMaxAmps := TeslaParameters.GetValues()
	//	var sPump string

	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, _ = fmt.Fprintf(w, `{
	"time":"%s",
	"tesla":{
		"maxAmps":%02f,
		"cars":[`, time.Now().String(), fMaxAmps)
	for i := range slaves {
		if i > 0 {
			_, _ = fmt.Fprint(w, ',')
		}
		stopped := ""
		if slaves[i].GetStopped() {
			stopped = " stopped"
		}
		_, _ = fmt.Fprintf(w, `
			{
				"Current":%0.2f,
				"maxAmps":%0.2f,
				"status":"%s%s"
			}`, float32(slaves[i].GetCurrent())/100, float32(slaves[i].GetAllowed())/100, slaves[i].GetStatus(), stopped)
	}
	_, _ = fmt.Fprintf(w, `
		]},
	"inverter":{
		"frequency":%0.2f,
		"vSetpoint":%0.2f,
		"vBatt":%0.2f,
		"iBatt":%0.2f,
		"soc":%0.2f,
		"iBattAvg":%0.2f,
		"bmsVBat":%0.2f,
		"bmsIBat":%0.2f,
		"bmsTBat":%0.2f,
		"bmsChargeVolts":%0.2f,
		"bmsDischargeVolts":%0.2f,
		"bmsChargeCurrentMax":%0.2f,
		"bmsDichargeCurrentMax":%0.2f,
		"bmsSOC":%d,
		"bmsSOH":%d,
		"power":%0.1f,
%s
	}
}`, iValues.GetFrequency(), iValues.GetSetPoint(),
		iValues.GetVolts(), iValues.GetAmps(), iValues.GetSOC(),
		iValues.GetAvgAmps(),
		iValues.bmsVBat, iValues.bmsIBat, iValues.bmsTBat,
		iValues.bmsChargeVolts, iValues.bmsDischargeVolts,
		iValues.bmsChargeCurrentMax, iValues.bmsDichargeCurrentMax,
		iValues.bmsSOC, iValues.bmsSOH, iValues.inverterPower,
		iValues.GetFlags())
}

func mailForm(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		_, err := fmt.Fprintf(w, `<html><head><title>Email Tester</title></head><body>`)
		if err != nil {
			log.Println(err)
		}
	}
	_, err := fmt.Fprintf(w, `<br/>Send email to Ian@CedarTechnology.com<br/>
			<form action="/mail" method="post">
				<label for="subject">Subject :</label><input name="subject" id="subject" value="%s" style="width:300px;border:solid 1px" /><br/>
				<label for="body">Body :</label><textarea name="body" id="body" rows="25" cols="80">%s</textarea><br />
				<input type="submit" value="Send">
			</form>
		</body>
	</html>`, r.Form.Get("subject"), r.Form.Get("body"))
	if err != nil {
		log.Println(err)
	}
}

func showLoggingSelection(w http.ResponseWriter, _ *http.Request) {
	if _, err := fmt.Fprint(w, `<!DOCTYPE html><html lang="en"><head><meta charset="UTF-8"><title>System Controller Logging Control</title></head><body><div><h1>Logging</h1></div><div><ul>`); err != nil {
		log.Println(err)
	}
	if TeslaParameters.IsLogging() {
		if _, err := fmt.Fprint(w, `<li><a href="/disableLogging/Tesla">Disable Tesla Logging</a></li>`); err != nil {
			log.Println(err)
		}
	} else {
		if _, err := fmt.Fprint(w, `<li><a href="/enableLogging/Tesla">Enable Tesla Logging</a></li>`); err != nil {
			log.Println(err)
		}
	}
	if iValues.Log {
		if _, err := fmt.Fprint(w, `<li><a href="/disableLogging/iValues">Disable iValue Logging</a></li>`); err != nil {
			log.Println(err)
		}
	} else {
		if _, err := fmt.Fprint(w, `<li><a href="/enableLogging/iValues">Enable iValue Logging</a></li>`); err != nil {
			log.Println(err)
		}
	}
	if logSolar {
		if _, err := fmt.Fprint(w, `<li><a href="/disableLogging/solar">Disable Solar Logging</a></li>`); err != nil {
			log.Println(err)
		}
	} else {
		if _, err := fmt.Fprint(w, `<li><a href="/enableLogging/solar">Enable Solar Logging</a></li>`); err != nil {
			log.Println(err)
		}
	}
	if _, err := fmt.Fprint(w, `</ul></div><div><a href="/ac_status.html">Status Page</a></div></body></html>`); err != nil {
		log.Println(err)
	}
}
