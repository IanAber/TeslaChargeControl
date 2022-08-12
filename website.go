package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"path/filepath"
	"time"
)

func setUpWebSite() {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/", getValues).Methods("GET")
	router.HandleFunc("/getTeslaKeys", ServeGetTeslaTokens).Methods("GET")
	router.HandleFunc("/setTeslaKeys", ServeSetTeslaTokens).Methods("POST")
	router.HandleFunc("/disableHeater", disableHeater).Methods("GET")
	router.HandleFunc("/enableHeater", enableHeater).Methods("GET")
	router.HandleFunc("/reloadChargingFunction", reloadChargingFunction).Methods("GET")
	router.HandleFunc("/version", getVersion).Methods("GET")
	router.HandleFunc("/enableLogging", enableLogging).Methods("GET")
	router.HandleFunc("/disableLogging", disableLogging).Methods("GET")
	router.HandleFunc("/waterTemps", getWaterTemps).Methods("GET")
	router.HandleFunc("/mail", sendMail).Methods("POST")
	router.HandleFunc("/mail", mailForm).Methods("GET")
	router.HandleFunc("/startCharging", handleStartCharging)
	router.HandleFunc("/stopCharging", handleStopCharging)
	router.HandleFunc("/enableElectrolyser", enableElectrolyser).Methods("GET", "POST")
	router.HandleFunc("/disableElectrolyser", disableElectrolyser).Methods("GET", "POST")

	router.HandleFunc("/realtime/getdata", getData).Methods("GET")
	router.HandleFunc("/realtime/getcoldtank", getColdTankData).Methods("GET")
	router.HandleFunc("/realtime/gethottank", getHotTankData).Methods("GET")
	router.HandleFunc("/realtime/getsolartemps", getSolarTempData).Methods("GET")

	fileServer := http.FileServer(neuteredFileSystem{http.Dir("/var/www/html")})
	router.PathPrefix("/").Handler(http.StripPrefix("/", fileServer))

	log.Fatal(http.ListenAndServe(":8080", router))
}

func sendMail(w http.ResponseWriter, r *http.Request) {
	if API == nil {
		log.Println("Cannot send mail. Tesla API is null.")
	}
	err := r.ParseForm()
	if err != nil {
		_, errFmt := fmt.Fprintf(w, err.Error())
		if errFmt != nil {
			log.Println(errFmt)
		}
		return
	}
	log.Println("Sending mail to ian.")
	err = API.SendMail(r.Form.Get("subject"), r.Form.Get("body"))
	if err != nil {
		_, errFmt := fmt.Fprintf(w, "<html><head><title>Send Email Error</title></head><body><h1>%s</h1><br/>", err.Error())
		if errFmt != nil {
			log.Println(errFmt)
		}
	} else {
		_, errFmt := fmt.Fprintf(w, "<html><head><title>Sent</title></head><body><h1>Message Sent!</h1><br/")
		if errFmt != nil {
			log.Println(errFmt)
		}
	}
	mailForm(w, r)
}

func ServeSetTeslaTokens(w http.ResponseWriter, r *http.Request) {
	if API == nil {
		log.Println("API is nil")
	}
	API.HandleSetTeslaTokens(w, r)
}

func ServeGetTeslaTokens(w http.ResponseWriter, r *http.Request) {
	if API == nil {
		_, errFmt := fmt.Println("API is nil")
		if errFmt != nil {
			log.Println(errFmt)
		}
	}
	API.ShowGetTokensPage(w, r)
}

type neuteredFileSystem struct {
	fs http.FileSystem
}

func (nfs neuteredFileSystem) Open(path string) (http.File, error) {
	f, err := nfs.fs.Open(path)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
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

	return f, nil
}

func handleStartCharging(w http.ResponseWriter, _ *http.Request) {
	err := API.StartCharging()
	if err == nil {
		_, errFmt := fmt.Fprint(w, "<html><head><title>Tesla Control</title></head><body><h1>Charging started</h1>", CHARGINGLINKS, "</body></html>")
		if errFmt != nil {
			log.Println(errFmt)
		}
	} else {
		_, errFmt := fmt.Fprint(w, "<html><head><title>Tesla Control</title></head><body><h1>Charging failed to start</h1><br />", err.Error(), "<br />", CHARGINGLINKS, "</body></html>")
		if errFmt != nil {
			log.Println(errFmt)
		}
	}
}

func handleStopCharging(w http.ResponseWriter, _ *http.Request) {
	err := API.StopCharging()
	if err == nil {
		_, errFmt := fmt.Fprint(w, "<html><head><title>Tesla Control</title></head><body><h1>Charging stopped</h1>", CHARGINGLINKS, "</body></html>")
		if errFmt != nil {
			log.Println(errFmt)
		}
	} else {
		_, errFmt := fmt.Fprint(w, "<html><head><title>Tesla Control</title></head><body><h1>Charging failed to stop</h1><br />", err.Error(), "<br />", CHARGINGLINKS, "</body></html>")
		if errFmt != nil {
			log.Println(errFmt)
		}
	}
}

func getWaterTemps(w http.ResponseWriter, _ *http.Request) {
	// Get the hot tank temperature
	var temps struct {
		Hot  float32 `json:"hot"`
		Cold float32 `json:"cold"`
	}
	var err = pDB.QueryRow("select greatest(`TSH0`, `TSH1`, `TSH2`) / 10 as `hotTemp`, least(`TSC0`, `TSC1`, `TSC2`) / 10 as `coldTemp` from `chillii_analogue_input` where `logged` > date_add(now(), interval -5 minute) order by `logged` desc limit 1;").Scan(&temps.Hot, &temps.Cold)
	if err != nil {
		http.Error(w, "Failed to get the hot tank temperatures.", http.StatusInternalServerError)
		log.Printf("Error fetching tank temperatures from the database - %s", err)
		return
	}
	str, err := json.Marshal(temps)
	if err != nil {
		http.Error(w, "Failed to marshal the tempratures into a JSON object", http.StatusInternalServerError)
		log.Printf("Error marshalling tank temperatures - %s", err)
		return
	}
	_, err = fmt.Fprint(w, string(str))
	if err != nil {
		log.Println("getWaterTemps() - ", err)
	}
}

func enableLogging(w http.ResponseWriter, r *http.Request) {
	iValues.Log = true
	getValues(w, r)
}

func disableLogging(w http.ResponseWriter, r *http.Request) {
	iValues.Log = false
	getValues(w, r)
}

func enableHeater(w http.ResponseWriter, r *http.Request) {
	Heater.SetEnabled(true)
	getValues(w, r)
}

func disableHeater(w http.ResponseWriter, r *http.Request) {
	Heater.SetEnabled(false)
	Heater.SetHeater(0)
	getValues(w, r)
}

func enableElectrolyser(w http.ResponseWriter, r *http.Request) {
	Electrolyser.SetEnabled(true)
	getValues(w, r)
}

func disableElectrolyser(w http.ResponseWriter, r *http.Request) {
	Electrolyser.SetEnabled(false)
	getValues(w, r)
}

func reloadChargingFunction(w http.ResponseWriter, _ *http.Request) {
	err := iValues.LoadFunctionConstants("/var/www/html/params/charge_params.json")
	if err != nil {
		log.Println(err)
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	jsonData, err := json.Marshal(iValues.GetQuinticFunction())
	if err != nil {
		log.Println(err)
	} else {
		_, err = w.Write(jsonData)
		if err != nil {
			log.Println(err)
		}
	}
}

func getVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_, err := fmt.Fprint(w, `<html>
  <head>
    <Cedar Technology System Manager>
  </head>
  <body>
    <h1>Cedar Technology System Manager</h1>
    <h2>Version 2.0 - October 29th 2019</h2>
  </body>
</html>`)
	if err != nil {
		log.Println(err)
	}
}

// getValues /*
// Return a set of values as a JSON object for consumption by a WEB page dashboard.
func getValues(w http.ResponseWriter, _ *http.Request) {
	_, fMaxAmps := TeslaParameters.GetValues()
	var sPump string

	w.Header().Set("Access-Control-Allow-Origin", "*")
	if Heater.GetPump() {
		sPump = "ON"
	} else {
		sPump = "OFF"
	}
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
	"electrolyser":{
		"enabled":%t,
		"level":%d
	},
	"heater":{
		"setting":%d,
		"pump":"%s",
		"enabled":"%s"
	},
	"inverter":{
		"frequency":%0.2f,
		"vSetpoint":%0.2f,
		"vBatt":%0.2f,
		"iBatt":%0.2f,
		"soc":%0.2f,
		"vBattDeltaMin":%0.2f,
		"vBattDeltaMax":%0.2f,
%s
	}
}`, Electrolyser.IsEnabled(), Electrolyser.GetRate(),
		Heater.GetSetting(), sPump, Heater.GetEnabled(), iValues.GetFrequency(), iValues.GetSetPoint(),
		iValues.GetVolts(), iValues.GetAmps(), iValues.GetSOC(),
		iValues.GetVBattDeltaMin(), iValues.GetVBattDeltaMax(), iValues.GetFlags())
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
