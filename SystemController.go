package main

import (
	"SystemController/Params"
	"SystemController/TeslaAPI"
	"SystemController/twcMessage"
	"SystemController/twcSlave"
	_ "crypto/aes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/IanAber/SMACanMessages"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/brutella/can"
	_ "github.com/go-sql-driver/mysql"
	"github.com/goburrow/serial"
	"github.com/gorilla/mux"
)

// Version 2 makes parameters editable via the WEB interface

const CHARGINGLINKS = `<a href="/startCharging">Start Charging</a><br><a href="/stopCharging">Stop Charging</a>`
const maxGasPressure = 34.0 // Pressure above which we do not increase the electrolyser output

var (
	address          string
	baudrate         int
	databits         int
	stopbits         int
	parity           string
	verbose          bool
	apiPort          uint
	databaseServer   string
	databasePort     string
	databaseName     string
	databaseLogin    string
	databasePassword string
	masterAddress    uint
	port             serial.Port
	TeslaParameters  Params.Params
	Heater           *HeaterSetting
	Electrolyser     ElectrolyserSetting
	iValues          InverterValues
	slaves           []twcSlave.Slave
	pDB              *sql.DB
	API              *TeslaAPI.TeslaAPI
)

func findSlave(slaves []twcSlave.Slave, address uint16) int {
	for i := range slaves {
		if slaves[i].GetAddress() == address {
			return i
		}
	}
	return -1
}

func logData(msg twcMessage.TwcMessage, slaves *[]twcSlave.Slave) {

	i := findSlave(*slaves, msg.GetFromAddress())
	if i >= 0 {
		(*slaves)[i].UpdateValues(&msg)
	} else {
		s := twcSlave.New(msg.GetFromAddress(), verbose, port)
		s.UpdateValues(&msg)
		*slaves = append(*slaves, s)
	}
}

// If we don't already have the slave, add it to the list
func processSlaveLinkReady(msg twcMessage.TwcMessage, slaves *[]twcSlave.Slave) {
	i := findSlave(*slaves, msg.GetFromAddress())
	if i < 0 {
		s := twcSlave.New(msg.GetFromAddress(), verbose, port)
		s.UpdateValues(&msg)
		*slaves = append(*slaves, s)
		log.Printf("Slave added [%04x]", msg.GetFromAddress())
	}
}

func checkSlaveTimeouts(slaves []twcSlave.Slave) []twcSlave.Slave {
	for i := range slaves {
		s := &slaves[i]
		if s.TimeSinceLastHeartbeat() > (10 * time.Second) {
			if s.GetAllowed() > 599 {
				log.Printf("=======> Slave %04x has gone away! Time span = %d > 10 seconds (%d). <=======\n", s.GetAddress(), s.TimeSinceLastHeartbeat(), time.Second*10)
				slaves[i] = slaves[len(slaves)-1]
				return slaves[:len(slaves)-1]
			} else {
				s.SetCurrent(0)
			}
		}
	}
	return slaves
}

// Heartbeat status to the slave
// 00 = no change
// 05 = Tell slave to change its setpoint

func sendHearbeatsToSlaves(slaves []twcSlave.Slave, masterAddress uint16) {
	for i := range slaves {
		slaves[i].SendMasterHeartbeat(masterAddress, API)
	}
}

func divideMaxAmpsAmongstSlaves(slaves []twcSlave.Slave, maxAmps uint16) {
	var activeCars uint16 = 0

	// Find out how many cars are waiting to charge, actively charging or starting to charge
	for i := range slaves {
		// Count how many cars are trying to charge.
		if slaves[i].RequestCharge() {
			activeCars++
		}
	}
	//	fmt.Println(activeCars, " cars are active")
	// If there is at least one car then divide the current between them equally
	if activeCars > 0 {
		maxAmps = maxAmps / activeCars
	}
	// Tesla can only accept charging currents from 5 amps upwards. We are trying to set a current of less
	// than 6 amps fix it to 5 amps unless the battery state of charge is less than 85%
	// If we end up with less than 5 amps for each car, stop charging until we have more available
	if maxAmps < 500 {
		if iValues.GetSOC() > 85 {
			maxAmps = 500
		} else {
			maxAmps = 0
		}
	}
	// Share out the current amongst the cars waiting to charge or actively charging
	for i := range slaves {
		if slaves[i].RequestCharge() {
			slaves[i].SetCurrent(maxAmps)
		} else {
			slaves[i].SetCurrent(2500)
		}
	}
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

	fileServer := http.FileServer(neuteredFileSystem{http.Dir("./web")})
	router.PathPrefix("/").Handler(http.StripPrefix("/", fileServer))

	log.Fatal(http.ListenAndServe(":8080", router))
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
	var err = pDB.QueryRow("select greatest(`TSH0`, `TSH1`, `TSH2`) / 10 as `hotTemp`, least(`TSC0`, `TSC1`, `TSC2`) / 10 as `coldTemp` from `chillii_analogue_input` where `TIMESTAMP` > date_add(now(), interval -5 minute) order by `TIMESTAMP` desc limit 1;").Scan(&temps.Hot, &temps.Cold)
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

func handleCANFrame(frm can.Frame) {
	switch frm.ID {
	case 0x305: // Battery voltage, current and state of charge
		c305 := SMACanMessages.NewCan305(frm.Data[0:])
		iValues.SetVolts(c305.VBatt())
		iValues.SetAmps(c305.IBatt())
		iValues.SetSOC(c305.SocBatt())
		API.AllowStart = (c305.IBatt() < -30)
		//		log.Printf("V = %f, I = %f, DOC = %f\n", c305.VBatt(), c305.IBatt(), c305.SocBatt())

	case 0x306: // Charge procedure, Operating state, Active error, Charge set point
		c306 := SMACanMessages.NewCan306(frm.Data[0:])
		iValues.SetSetPoint(c306.ChargeSetPoint())

	case 0x010: // Frequency
		c010 := SMACanMessages.NewCan010(frm.Data[0:])
		iValues.SetFrequency(c010.Frequency())
		//		log.Printf("Frequency = %f\n", c010.Frequency())

	case 0x307: // Relays and status
		c307 := SMACanMessages.NewCan307(frm.Data[0:])
		iValues.GnRun = c307.GnRun()
		iValues.OnRelay1 = c307.Relay1Master()
		iValues.OnRelay2 = c307.Relay2Master()
		iValues.OnRelay1Slave1 = c307.Relay1Slave1()
		iValues.OnRelay2Slave1 = c307.Relay2Slave1()
		iValues.OnRelay1Slave2 = c307.Relay1Slave2()
		iValues.OnRelay2Slave2 = c307.Relay2Slave2()
		iValues.GnRun = c307.GnRun()
		iValues.GnRunSlave1 = c307.GnRunSlave1()
		iValues.GnRunSlave2 = c307.GnRunSlave2()
		iValues.AutoGn = c307.AutoGn()
		iValues.AutoLodExt = c307.AutoLodExt()
		iValues.AutoLodSoc = c307.AutoLodSoc()
		iValues.Tm1 = c307.Tm1()
		iValues.Tm2 = c307.Tm2()
		iValues.ExtPwrDer = c307.ExtPwrDer()
		iValues.ExtVfOk = c307.ExtVfOk()
		iValues.GdOn = c307.GdOn()
		iValues.Errror = c307.Error()
		iValues.Run = c307.Run()
		iValues.BatFan = c307.BatFan()
		iValues.AcdCir = c307.AcdCir()
		iValues.MccBatFan = c307.MccBatFan()
		iValues.MccAutoLod = c307.MccAutoLod()
		iValues.Chp = c307.Chp()
		iValues.ChpAdd = c307.ChpAdd()
		iValues.SiComRemote = c307.SiComRemote()
		iValues.OverLoad = c307.Overload()
		iValues.ExtSrcConn = c307.ExtSrcConn()
		iValues.Silent = c307.Silent()
		iValues.Current = c307.Current()
		iValues.FeedSelfC = c307.FeedSelfC()
		iValues.Esave = c307.Esave()
	}
}

func processCANFrames() {
	bus, err := can.NewBusForInterfaceWithName("can0")
	if err != nil {
		log.Fatalf("Error starting CAN interface - %s -\nSorry, I am giving up", err)
	} else {
		log.Println("Connected to CAN bus - monitoring the inverters.")
	}
	bus.SubscribeFunc(handleCANFrame)
	err = bus.ConnectAndPublish()
	if err != nil {
		log.Printf("ConnectAndPublish failed - %s", err)
		os.Exit(-1)
	}
}

func connectToDatabase() (*sql.DB, error) {
	if pDB != nil {
		_ = pDB.Close()
		pDB = nil
	}
	var sConnectionString = databaseLogin + ":" + databasePassword + "@tcp(" + databaseServer + ":" + databasePort + ")/" + databaseName

	fmt.Println("Connecting to [", sConnectionString, "]")
	db, err := sql.Open("mysql", sConnectionString)
	if err != nil {
		return nil, err
	}
	err = db.Ping()
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, err
}

func init() {

	// Set up logging
	//	logwriter, e := syslog.New(syslog.LOG_NOTICE, "myprog")
	//	if e == nil {
	//		log.SetOutput(logwriter)
	//	}

	// Get the settings
	flag.StringVar(&address, "teslaPort", "/dev/serial/by-path/platform-3f980000.usb-usb-0:1.2:1.0-port0", "Serial port address")
	flag.IntVar(&baudrate, "teslaBaud", 9600, "Serial port baud rate")
	flag.IntVar(&databits, "teslaBits", 8, "Serial port data bits")
	flag.IntVar(&stopbits, "teslaStop", 1, "Serial port stop bits")
	flag.StringVar(&parity, "teslaParity", "N", "Serial port parity (N/E/O)")
	flag.UintVar(&masterAddress, "teslaMaster", 0x7777, "Master TWC address")
	flag.UintVar(&apiPort, "apiPort", 0x8080, "WEB port to listen on for API connections")
	flag.StringVar(&databaseServer, "sqlServer", "127.0.0.1", "MySQL Server")
	flag.StringVar(&databaseName, "database", "logging", "Database name")
	flag.StringVar(&databaseLogin, "dbUser", "logger", "Database login user name")
	flag.StringVar(&databasePassword, "dbPassword", "logger", "Database user password")
	flag.StringVar(&databasePort, "dbPort", "3306", "Database port")
	flag.BoolVar(&verbose, "verbose", false, "Enable verbose mode to trace information to STDOUT.")
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	// Initialise the current values
	TeslaParameters.Reset()
	Heater = NewHeaterSetting()
	// Set up the quintic function to control charging
	err := iValues.LoadFunctionConstants("/var/www/html/params/charge_params.json")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Verbose = ", verbose)

	// Set up the API WEBSite
	go setUpWebSite()

	config := serial.Config{
		Address:  address,
		BaudRate: baudrate,
		DataBits: databits,
		StopBits: stopbits,
		Parity:   parity,
		Timeout:  1, //30 * time.Second,
	}

	p, err := serial.Open(&config)
	if err != nil {
		log.Fatalf("ERROR - %s - Cannot connect to the Tesla RS485 port.\nSorry. I am givng up!", err)
	} else {
		log.Println("Connecting to Tesla Wall Charger via ", config.Address)
	}
	port = p

	// Set up the database connection
	pDB, err = connectToDatabase()
	if err != nil {
		log.Fatalf("Failed to connect to to the database - %s - Sorry, I am giving up.", err)
	} else {
		log.Println("Connected to the database")
	}

	API, err = TeslaAPI.New()
	if err != nil {
		log.Println(err)
	} else {
		log.Println("Tesla API setup and ready.")
	}

	log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime)
	Electrolyser.Enabled = true
	Electrolyser.gasPressure = maxGasPressure
	// Start handling incoming CAN bus messages
	go processCANFrames()
}

// Ping the API for the vehicle ID every 2 hours. This should update the token as required
func teslaKeepAlive() {
	defer func() {
		stackSlice := make([]byte, 512)
		s := runtime.Stack(stackSlice, false)
		log.Printf("\n%s", stackSlice[0:s])
	}()
	for {
		if API.IsConfigured() {
			err := API.GetVehicleId()
			if err != nil {
				log.Println(err)
			}
		} else {
			log.Println("tesla keep alive - API is not configured")
			err := API.SendMail("Keep Alive Failed", "Tesla Keep Alive Failed - Not Configured")
			if err != nil {
				log.Println(err)
			}
		}
		time.Sleep(time.Hour * 2)
	}
}

// Make sure the heater is turned down if we are discharging
func killHeaterOnDischarge() {
	for {
		// If discharging at more than 1 amp then decrease the heater if it is on
		// This makes recovery from discharge quite fast.
		if (iValues.GetAmps() > 1) && (Heater.GetSetting() > 0) {
			Heater.Decrease(true)
		}
		time.Sleep(time.Second)
	}
}

// This function will look at the various inverter parameters and work out if there is power available for car charging or water heating
// It bases this calculation on the current battery state of charge, the battery charging current and the difference between the setpoint
// and the actual battery voltage
func calculatePowerAvailable() {
	//	var iBatt float32
	//	var soc float32
	//	var frequency float64
	var delta int16
	lastPowerState := 0
	//	var step int16

	for {

		powerState := iValues.GetChargeLevel()
		// If we get to a point where we are inside the ideal window before midday then preheat the electrolysers ready to produce hydrogen.
		if powerState == 0 && lastPowerState == -1 {
			if time.Now().Hour() < 12 {
				Electrolyser.preHeat()
			}
		}
		lastPowerState = powerState

		// Set the total car charging current for all cars charging
		carCurrent := float32(0.0)
		for i := range slaves {
			carCurrent += float32(slaves[i].GetCurrent()) / 100.0
		}
		TeslaParameters.SetCurrent(carCurrent)

		if iValues.AutoGn {
			// If the generator is running turn off the Tesla and the auxiliary heater
			TeslaParameters.SetMaxAmps(0)
			Electrolyser.ChangeRate(-100)
			if Heater.GetSetting() > 0 {
				Heater.SetHeater(0)
			}
		} else if powerState == 1 { // If the delta is less than the minimum we can take more power
			log.Println("Increasing consumption")
			// Inverter current is at 48V so approx. 5 times car current. We should push it up in small stages
			delta = 0 - int16(iValues.GetAmps()/10) // Charging shows as a negative inverter current
			log.Println("Inverter current =", iValues.GetAmps(), "A - (negative = charging)")
			if Electrolyser.currentSetting < 100 {
				// If the electrolysers are running below 100% then limit the car to 20Amps
				TeslaParameters.SetSystemAmps(20)
			} else {
				// Electrolysers are running at 100% so allow the car to run up to the full 48amps
				TeslaParameters.SetSystemAmps(48)
			}
			log.Println("Tesla set to", TeslaParameters.GetMaxAmps(), "Amps Max.")
			if carCurrent > 1 {
				// Car is charging so try and increase the charge rate
				log.Println("Change Tesla", delta, "A")
				if !TeslaParameters.ChangeCurrent(delta) {
					// Charge rate increase was not accepted so turn up the electrolyser
					log.Println("Increase Electrolyser ", delta, "%")
					if !Electrolyser.ChangeRate(delta) {
						Heater.Increase(iValues.GetFrequency())
					}
				}
			} else {
				// No car charging requested so set the available current to 15.0 amps and turn up the auxiliary heater
				log.Println("Tesla not charging to default to 15A and increase electrolyser")
				TeslaParameters.SetMaxAmps(15.0)
				// If the frequency is over 60.9 the solar inverters are throttled so we should whack the electrolyser up to full immediately
				if iValues.frequency > 60.9 {
					delta = 100
				}
				if !Electrolyser.ChangeRate(delta) {
					// Electrolyser did not increase so we should turn the heaters up.
					log.Println("Electrolyser did not increase so increasing water heater")
					Heater.Increase(iValues.GetFrequency())
				}
			}
		} else if powerState == -1 { // If the delta is more than the max we need to reduce the load to give the battery chance to charge up
			// Turn the water heat down first
			log.Println("Reducing consumption")
			if !Heater.Decrease(false) {
				delta = 0 - int16(iValues.GetAmps()/5) // Inverter current is at 48V so 5 times the 240 car current
				// Delta is negative here
				log.Println("Inverter current =", iValues.GetAmps(), "A Discharging - Delta set to", delta)
				// if the heater is already off and the Tesla is above 20A then reduce the Tesla
				if carCurrent > 20 {
					// Drop the car current
					log.Println("Car > 20A so change it", delta, "A")
					TeslaParameters.ChangeCurrent(delta)
				} else {
					// Car is not above 20A so derease the Electrolysers first
					log.Println("Car < 20A so changing Electrolysers", delta, "%")
					if !Electrolyser.ChangeRate(delta) {
						// Electrolyser did not decrease, perhaps because it is already zero, so drop the car rate
						log.Println("Electrolyser is 0 so changing the Tesla ", delta, "A")
						TeslaParameters.ChangeCurrent(delta)
					}
				}
			}
		}
		time.Sleep(time.Second * 2)
	}
}

func CloseDB() {
	_ = pDB.Close()
}

func logToDatabase() {
	defer CloseDB()

	lastFrequency := iValues.GetFrequency()
	lastVsetpoint := iValues.GetSetPoint()
	lastVbatt := iValues.GetVolts()
	lastIbatt := iValues.GetAmps()
	lastSoc := iValues.GetSOC()
	lastIavailable := TeslaParameters.GetMaxAmps()
	lastIused := TeslaParameters.GetCurrent()
	lastHeatersetting := Heater.GetSetting()
	lastHeaterpump := Heater.GetPump()
	var err error
	hotTankTemp := int16(1000)

	for {
		newFrequency := iValues.GetFrequency()
		newVsetpoint := iValues.GetSetPoint()
		newVbatt := iValues.GetVolts()
		newIbatt := iValues.GetAmps()
		newSoc := iValues.GetSOC()
		newIavailable := TeslaParameters.GetMaxAmps()
		newIused := TeslaParameters.GetCurrent()
		newHeatersetting := Heater.GetSetting()
		newHeaterpump := Heater.GetPump()

		if pDB == nil {
			pDB, err = connectToDatabase()
			if err != nil {
				log.Println("Error opening the database ", err)
				pDB = nil
				time.Sleep(time.Second)
				continue
			}
		}

		if (newFrequency != lastFrequency) || (newVsetpoint != lastVsetpoint) || (newVbatt != lastVbatt) || (newIbatt != lastIbatt) || (newSoc != lastSoc) {
			lastFrequency = newFrequency
			lastVsetpoint = newVsetpoint
			lastVbatt = newVbatt
			lastIbatt = newIbatt
			lastSoc = newSoc
			var _, err = pDB.Exec("call log_inverter_values(?, ?, ?, ?, ?)", newFrequency, newVsetpoint, newVbatt, newIbatt, newSoc)
			if err != nil {
				log.Printf("Error writing inverter values to the database - %s", err)
				_ = pDB.Close()
				pDB = nil
				time.Sleep(time.Second)
				continue
			}
		}
		if (newIavailable != lastIavailable) || (newIused != lastIused) {
			lastIavailable = newIavailable
			lastIused = newIused
			_, err := pDB.Exec("call log_tesla_values(?, ?)", newIavailable, newIused)
			if err != nil {
				log.Printf("Error writing Tesla values to the database - %s", err)
				_ = pDB.Close()
				pDB = nil
				time.Sleep(time.Second)
				continue
			}
		}
		if (newHeatersetting != lastHeatersetting) || (newHeaterpump != lastHeaterpump) {
			lastHeatersetting = newHeatersetting
			lastHeaterpump = newHeaterpump
			lastIused = newIused
			_, err := pDB.Exec("call log_heater_values(?, ?)", newHeatersetting, newHeaterpump)
			if err != nil {
				log.Printf("Error writing heater values to the database - %s", err)
				_ = pDB.Close()
				pDB = nil
				time.Sleep(time.Second)
				continue
			}
		}
		// Get the hot tank temperature
		var err = pDB.QueryRow("select greatest(`TSH0`, `TSH1`, `TSH2`) as maxtemp from `chillii_analogue_input` where `TIMESTAMP` > date_add(now(), interval -5 minute) order by `TIMESTAMP` desc limit 1;").Scan(&hotTankTemp)
		if err != nil {
			Heater.SetHotTankTemp(1000) // Be safe. If we can't get the temperature assume it is boiling to shut down the heater.
			log.Printf("Error fetching hot tank temperature from the database - %s", err)
			err = pDB.Close()
			pDB = nil
			time.Sleep(time.Second)
			continue
		}
		Heater.SetHotTankTemp(hotTankTemp)
		time.Sleep(time.Second)
	}
}

func main() {
	var buf [1]byte
	var linkReadyNum int
	var err interface{}

	defer func() {
		err = port.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()
	linkReadyNum = 10

	msg := twcMessage.New(port, verbose)
	t := time.Now()

	// Start the Tesla Keep Alive loop
	go teslaKeepAlive()

	// Start the power management loop
	go calculatePowerAvailable()

	// Start the heater kill loop to ensure the heater is not drawing from the battery.
	go killHeaterOnDischarge()

	go logToDatabase()

	for {
		if time.Since(t) > time.Second {
			if linkReadyNum > 5 {
				msg.SendMasterLinkReady1(uint16(masterAddress))
				linkReadyNum--
			} else if linkReadyNum > 0 {
				msg.SendMasterLinkReady2(uint16(masterAddress))
				linkReadyNum--
			}
			if len(slaves) > 0 {
				divideMaxAmpsAmongstSlaves(slaves, uint16(TeslaParameters.GetMaxAmps()*100))
				sendHearbeatsToSlaves(slaves, uint16(masterAddress))

				linkReadyNum = 0
			}
			if linkReadyNum < 0 {
				linkReadyNum = 0
			}
			t = time.Now()
		}
		for {
			_, err := port.Read(buf[:])
			if err != nil {
				if err != serial.ErrTimeout {
					fmt.Println(err)
				}
				break
			} else {
				//				fmt.Printf("-%02x", buf[0])
				bGotMessage, err := msg.AddByte(buf[0])
				if err != nil {
					log.Print(err)
					break
				}
				if bGotMessage {
					//					msg.Print()
					switch msg.GetCode() {
					//						case 0xfbe0 : fmt.Printf("To Slave %04x | Status = %02x | SetPoint = %0.2f | = %0.2f\n", msg.GetToAddress(), msg.GetStatus(), float32(msg.GetSetPoint()) / 100, float32(msg.GetCurrent()) / 100)
					case 0xfde0:
						logData(msg, &slaves)
					case 0xfde2:
						processSlaveLinkReady(msg, &slaves)
					default:
						log.Printf("Unknown message code %02x\n", msg.GetCode())
					}
					msg.Reset()
				}
			}
		}
		s := checkSlaveTimeouts(slaves)
		if s != nil {
			slaves = s
		}
		time.Sleep(100 * time.Millisecond)
	}
}
