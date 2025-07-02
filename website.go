package main

import (
	"SystemController/quinticFunction"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"
)

func setUpWebSite() {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/", getValues).Methods("GET")
	router.HandleFunc("/reloadChargingFunction", reloadChargingFunction).Methods("GET")
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
	router.HandleFunc("/realtime/getHeater", getHeaterData).Methods("GET")
	router.HandleFunc("/realtime/getVoltage", getVoltageData).Methods("GET")
	router.HandleFunc("/realtime/getactemps", getACTempData).Methods("GET")
	router.HandleFunc("/ChargeParams", showChargeParams).Methods("GET")
	router.HandleFunc("/ChargeParams", saveChargeParams).Methods("POST")

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

func showChargeParams(w http.ResponseWriter, _ *http.Request) {
	qf := iValues.GetQuinticFunction()
	_, err := fmt.Fprintf(w, `<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Charge Parameters</title>
    <link rel="stylesheet" href="../jqwidgets/styles/jqx.base.css" type="text/css" />
    <link rel="stylesheet" href="../jqwidgets/styles/jqx.web.css" type="text/css" />
    <script type="text/javascript" src="../scripts/jquery-3.6.0.min.js"></script>
    <script type="text/javascript" src="../jqwidgets/jqxcore.js"></script>
    <script type="text/javascript" src="../jqwidgets/jqxchart.core.js"></script>
    <script type="text/javascript" src="../jqwidgets/jqxdraw.js"></script>
    <script type="text/javascript" src="../jqwidgets/jqxdata.js"></script>
    <script type="text/javascript">
        var data = [
            {"SOC":100,"Min":0,"Max":0},
            {"SOC":95,"Min":0,"Max":0},
            {"SOC":90,"Min":0,"Max":0},
            {"SOC":85,"Min":0,"Max":0},
            {"SOC":80,"Min":0,"Max":0},
            {"SOC":75,"Min":0,"Max":0},
            {"SOC":70,"Min":0,"Max":0},
            {"SOC":65,"Min":0,"Max":0},
            {"SOC":60,"Min":0,"Max":0},
            {"SOC":55,"Min":0,"Max":0},
            {"SOC":50,"Min":0,"Max":0},
            {"SOC":45,"Min":0,"Max":0},
            {"SOC":40,"Min":0,"Max":0},
            {"SOC":35,"Min":0,"Max":0},
            {"SOC":30,"Min":0,"Max":0},
            {"SOC":25,"Min":0,"Max":0},
            {"SOC":20,"Min":0,"Max":0},
            {"SOC":15,"Min":0,"Max":0},
            {"SOC":10,"Min":0,"Max":0},
            {"SOC":5,"Min":0,"Max":0},
            {"SOC":0,"Min":0,"Max":0}]

        var params = {
   			"min": {"a":0.0, "b":0.0, "c":0.0, "d":0.0, "e":0.0, "f":0.0},
   			"max": {"a":0.0, "b":0.0, "c":0.0, "d":0.0, "e":0.0, "f":0.0}
        }

        function calculateValues() {
            params.min.a = document.getElementById("min_a").value;
            params.min.b = document.getElementById("min_b").value;
            params.min.c = document.getElementById("min_c").value;
            params.min.d = document.getElementById("min_d").value;
            params.min.e = document.getElementById("min_e").value;
            params.min.f = document.getElementById("min_f").value;
            params.max.a = document.getElementById("max_a").value;
            params.max.b = document.getElementById("max_b").value;
            params.max.c = document.getElementById("max_c").value;
            params.max.d = document.getElementById("max_d").value;
            params.max.e = document.getElementById("max_e").value;
            params.max.f = document.getElementById("max_f").value;
            data = [];
            for(soc = 0; soc <= 100; soc++) {
                data.push({"SOC":soc,
                    "Min": (0 - params.min.a)
                        + (params.min.b * soc)
                        - (params.min.c * Math.pow(soc, 2))
                        + (params.min.d * Math.pow(soc, 3))
                        - (params.min.e * Math.pow(soc, 4))
                        + (params.min.f * Math.pow(soc, 5)),
                    "Max":(0 -  params.max.a)
                        + (params.max.b * soc)
                        - (params.max.c * Math.pow(soc, 2))
                        + (params.max.d * Math.pow(soc, 3))
                        - (params.max.e * Math.pow(soc, 4))
                        + (params.max.f * Math.pow(soc, 5))})
            }
            console.log(data);
            $('#ChartContainer').jqxChart({'source':data});
            $('#ChartContainer').jqxChart('update');
        }

        $(document).ready(function () {
            // prepare jqxChart settings
            var settings = {
                title: "Charging Control",
                description: "Charging control curve",
                enableAnimations: false,
                animationDuration: 1000,
                enableAxisTextAnimation: true,
                showLegend: false,
                enableAnimations: true,
                padding: { left: 5, top: 5, right: 5, bottom: 5 },
                titlePadding: { left: 90, top: 0, right: 0, bottom: 10 },
                colorScheme: 'scheme04',
                categoryAxis: {
                    dataField: 'SOC',
                    showGridLines: false,
                    textRotationAngle: 270,
                    unitInterval: 10,
                    formatFunction: function (value) { return value },
                    title: {
                        visible: true,
                        text: "Battery State of Charge (percent)",
                    }
                },
                colorScheme: 'scheme01',
                seriesGroups: [{
                    type: 'splinearea',
                    valueAxis: {
                        position: 'right',
                        unitInterval: 0.1,
                        tickMarks: {
                            visible: true,
                            step: 0.2,
                        },
                        gridLines: {
                            visible: false,
                            step: 0.1,
                        },
                        labels: {
                            formatSettings: {
                                decimalPlaces: 0,
                            },
                            visible: true,
                            step: 0.1,
                        },
                        minValue: 0,
                        maxValue: 6,
                        description: 'Delta between Actual and Setpoint (Volts)',
                    },
                    series: [{
                        dataField: 'Max',
                        displayText: 'Maximum Delta V',
                        lineColor: '#C00000',
                        fillColor: '#10FF10',
                    },
                        {
                            dataField: 'Min',
                            displayText: 'Minimum Delta V',
                            lineColor: '#C00000',
                            fillColor: 'white'
                        }]
                }]
            }
            // select the chartContainer DIV element and render the chart.
			let chart = $('#ChartContainer');
			chart.jqxChart(settings);
            calculateValues();
        });

        function goBack() {
            window.history.back();
        }

    </script>
    <style>
        input.const{
            width:10em;
        }
    </style>
</head>
<body>
<body style="background:white;">
    <form action="ChargeParams" method="post">
        <div style="float:left">
            <table>
                <tr>
                    <th>Minimum</th>
                    <th />
                    <th>Maximum</th>
                </tr>
                <tr>
                    <td><input type="number" pattern="[0-9]*" class=const step="0.05" id="min_a" name="min_a" value="%0.2f" onchange="calculateValues()" tabindex=1/></td>
                    <th>A</th>
                    <td><input type="number" pattern="[0-9]*" class=const step="0.05" id="max_a" name="max_a" value="%0.2f" onchange="calculateValues()" tabindex=7 /></td>
                </tr>
                <tr>
                    <td><input type="number" pattern="[0-9]*" class=const step="0.0005" id="min_b" name="min_b" value="%0.4f" onchange="calculateValues()" tabindex=2 /></td>
                    <th>B</th>
                    <td><input type="number" pattern="[0-9]*" class=const step="0.0005" id="max_b" name="max_b" value="%0.4f" onchange="calculateValues()" tabindex=8 /></td>
                </tr>
                <tr>
                    <td><input type="number" pattern="[0-9]*" class=const step="0.000005" id="min_c" name="min_c" value="%0.6f" onchange="calculateValues()" tabindex=3 /></td>
                    <th>C</th>
                    <td><input type="number" pattern="[0-9]*" class=const step="0.000005" id="max_c" name="max_c" value="%0.6f" onchange="calculateValues()" tabindex=9 /></td>
                </tr>
                <tr>
                    <td><input type="number" pattern="[0-9]*" class=const step="0.00000005" id="min_d" name="min_d" value="%0.8f" onchange="calculateValues()" tabindex=4 /></td>
                    <th>D</th>
                    <td><input type="number" pattern="[0-9]*" class=const step="0.00000005" id="max_d" name="max_d" value="%0.8f" onchange="calculateValues()" tabindex=10 /></td>
                </tr>
                <tr>
                    <td><input type="number" pattern="[0-9]*" class=const step="0.0000000005" id="min_e" name="min_e" value="%0.10f" onchange="calculateValues()" tabindex=5 /></td>
                    <th>E</th>
                    <td><input type="number" pattern="[0-9]*" class=const step="0.0000000005" id="max_e" name="max_e" value="%0.10f" onchange="calculateValues()" tabindex=11 /></td>
                </tr>
                <tr>
                    <td><input type="number" pattern="[0-9]*" class=const step="0.000000000005" id="min_f" name="min_f" value="%0.12f" onchange="calculateValues()" tabindex=6 /></td>
                    <th>F</th>
                    <td><input type="number" pattern="[0-9]*" class=const step="0.000000000005" id="max_f" name="max_f" value="%0.12f" onchange="calculateValues()" tabindex=12 /></td>
                </tr>
            </table>
        </div>
        <div style="overflow:hidden; margin-left:20em; margin-right:20em">
            <h2 style="text-align:center">Charging will try to maintain the delta between setpoint and actual battery voltage in the green area by controlling the Tesla charging current and the auxilliary heater power.</h2>
            <h3 style="text-align:center">-A+(B*x)-(C*x<sup>2</sup>)+(D*x<sup>3</sup>)-(E*x<sup>4</sup>)+(F*x<sup>5</sup>)</h3>
            <div style="text-align:center">
                <button type="button" onclick="window.history.back()" tabindex=14 style="font-size:1.5em;border-radius: 10px">&lt;&lt;&lt; Back</button>&nbsp;
                <button type="submit" onclick="calculateValues()" tabindex=13  style="font-size:1.5em;border-radius: 10px">Update Settings</button>
            </div>
        </div>
    </form>
    <div id='ChartContainer' style="width:98%%; height: 80%%" ></div>
</body>

</body>
</html>`, qf.Min.A, qf.Max.A, qf.Min.B, qf.Max.B, qf.Min.C, qf.Max.C, qf.Min.D, qf.Max.D, qf.Min.E, qf.Max.E, qf.Min.F, qf.Max.F)
	if err != nil {
		ReturnJSONError(w, "Charging Parameters", err, http.StatusInternalServerError, true)
	}
}

func saveChargeParams(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		_, errFmt := fmt.Fprintf(w, err.Error())
		if errFmt != nil {
			log.Println(errFmt)
		}
		ReturnJSONError(w, "Save Charge Parameters", err, http.StatusBadRequest, true)
		return
	}
	minA := r.Form.Get("minA")
	maxA := r.Form.Get("maxA")
	minB := r.Form.Get("minB")
	maxB := r.Form.Get("maxB")
	minC := r.Form.Get("minC")
	maxC := r.Form.Get("maxC")
	minD := r.Form.Get("minD")
	maxD := r.Form.Get("maxD")
	minE := r.Form.Get("minE")
	maxE := r.Form.Get("maxE")
	minF := r.Form.Get("minF")
	maxF := r.Form.Get("maxF")

	var qf quinticFunction.QuinticFunction

	const DeviceString = "Save Charge Params"
	qf.Min.A, err = strconv.ParseFloat(minA, 10)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	}
	qf.Max.A, err = strconv.ParseFloat(maxA, 10)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	}
	qf.Min.B, err = strconv.ParseFloat(minB, 10)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	}
	qf.Max.B, err = strconv.ParseFloat(maxB, 10)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	}
	qf.Min.C, err = strconv.ParseFloat(minC, 10)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	}
	qf.Max.C, err = strconv.ParseFloat(maxC, 10)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	}
	qf.Min.D, err = strconv.ParseFloat(minD, 10)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	}
	qf.Max.D, err = strconv.ParseFloat(maxD, 10)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	}
	qf.Min.E, err = strconv.ParseFloat(minE, 10)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	}
	qf.Max.E, err = strconv.ParseFloat(maxE, 10)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	}
	qf.Min.F, err = strconv.ParseFloat(minF, 10)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	}
	qf.Max.F, err = strconv.ParseFloat(maxF, 10)
	if err != nil {
		ReturnJSONError(w, DeviceString, err, http.StatusBadRequest, true)
		return
	}
	if err := qf.SaveConstants(ChargingConstantsFile); err != nil {
		log.Println(err)
	}
	if err := iValues.qf.LoadConstants(ChargingConstantsFile); err != nil {
		log.Println(err)
	}
	showChargeParams(w, r)
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
		Hot  float32 `json:"hot"`
		Cold float32 `json:"cold"`
	}
	var err = pDB.QueryRow(`SELECT (HotTankTop + HotTankBottom + HotTankMiddle) / 30 AS hotTemp,
										(BufferTankTop + BufferTankMiddle + BufferTankBottom) / 30 As coldTemp
									FROM temperatures
									WHERE Logged > date_add(now(), INTERVAL -5 MINUTE)
									ORDER BY Logged DESC LIMIT 1;`).Scan(&temps.Hot, &temps.Cold)
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
		"vBattDeltaMin":%0.2f,
		"vBattDeltaMax":%0.2f,
%s
	}
}`, iValues.GetFrequency(), iValues.GetSetPoint(),
		iValues.GetVolts(), iValues.GetAmps(), iValues.GetSOC(),
		iValues.GetAvgAmps(),
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
