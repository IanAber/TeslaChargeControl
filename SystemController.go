package main

import (
	"CanMessages/CAN_010"
	"CanMessages/CAN_305"
	"CanMessages/CAN_306"
	"CanMessages/CAN_307"
	"SystemController/InverterValues"
	"SystemController/Params"
	"SystemController/TeslaAPI"
	"SystemController/heaterSetting"
	"SystemController/twcMessage"
	"SystemController/twcSlave"
	_ "crypto/aes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/brutella/can"
	_ "github.com/go-sql-driver/mysql"
	"github.com/goburrow/serial"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"time"
)

// Version 2 makes parameters editable via the WEB interface

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
	Heater           *heaterSetting.HeaterSetting
	iValues          InverterValues.InverterValues
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
	// If we end up with less than 5 amps for each car, stop charging until we have more available
	if maxAmps < 500 {
		maxAmps = 0
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

func ServeHandleTeslaLogin(w http.ResponseWriter, r *http.Request) {
	if API == nil {
		fmt.Println("API is nil")
	}
	API.HandleTeslaLogin(w, r)
}

func ServeShowLoginPage(w http.ResponseWriter, r *http.Request) {
	if API == nil {
		fmt.Println("API is nil")
	}
	API.ShowLoginPage(w, r)
}

func setUpWebSite() {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/", getValues).Methods("GET")
	router.HandleFunc("/TeslaLogin", ServeShowLoginPage).Methods("GET")
	router.HandleFunc("/getTeslaKeys", ServeHandleTeslaLogin).Methods("POST")
	router.HandleFunc("/disableHeater", disableHeater).Methods("GET")
	router.HandleFunc("/enableHeater", enableHeater).Methods("GET")
	router.HandleFunc("/reloadChargingFunction", reloadChargingFunction).Methods("GET")
	router.HandleFunc("/version", getVersion).Methods("GET")
	router.HandleFunc("/enableLogging", enableLogging).Methods("GET")
	router.HandleFunc("/disableLogging", disableLogging).Methods("GET")
	router.HandleFunc("/waterTemps", getWaterTemps).Methods("GET")
	log.Fatal(http.ListenAndServe(":8080", router))
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
}`, Heater.GetSetting(), sPump, Heater.GetEnabled(), iValues.GetFrequency(), iValues.GetSetPoint(),
		iValues.GetVolts(), iValues.GetAmps(), iValues.GetSOC(),
		iValues.GetVBattDeltaMin(), iValues.GetVBattDeltaMax(), iValues.GetFlags())
}

func handleCANFrame(frm can.Frame) {
	switch frm.ID {
	case 0x305: // Battery voltage, current and state of charge
		c305 := CAN_305.New(frm.Data[0:])

		iValues.SetVolts(c305.VBatt())
		iValues.SetAmps(c305.IBatt())
		iValues.SetSOC(c305.SocBatt())

	case 0x306: // Charge procedure, Operating state, Active error, Charge set point
		c306 := CAN_306.New(frm.Data[0:])
		iValues.SetSetPoint(c306.ChargeSetPoint())

	case 0x010: // Frequency
		c010 := CAN_010.New(frm.Data[0:])
		iValues.SetFrequency(c010.Frequency())

	case 0x307: // Relays and status
		c307 := CAN_307.New(frm.Data[0:])
		iValues.GnRun = c307.GnRun()
		iValues.OnRelay1 = c307.Relay1_Master()
		iValues.OnRelay2 = c307.Relay2_Master()
		iValues.OnRelay1Slave1 = c307.Relay1_Slave1()
		iValues.OnRelay2Slave1 = c307.Relay2_Slave1()
		iValues.OnRelay1Slave2 = c307.Relay1_Slave2()
		iValues.OnRelay2Slave2 = c307.Relay2_Slave2()
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
	// Initialise the current values

	TeslaParameters.Reset()
	Heater = heaterSetting.New()

	// Set up logging
	//	logwriter, e := syslog.New(syslog.LOG_NOTICE, "myprog")
	//	if e == nil {
	//		log.SetOutput(logwriter)
	//	}

	// Set up the quintic function to control charging
	err := iValues.LoadFunctionConstants("/var/www/html/params/charge_params.json")
	if err != nil {
		log.Fatal(err)
	}

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

	fmt.Println("Verbose = ", verbose)

	// Set up the API WEB Site
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

	API, err = TeslaAPI.New(verbose)
	if err != nil {
		log.Println(err)
	} else {
		log.Println("Tesla API setup and ready.")
	}

	// Start handling incoming CAN messages
	go processCANFrames()
}

/**
Every 6 hours we should hit the Tesla API to keep our token valid.
*/
func teslaKeepAlive() {
	var err error

	keepAliveTicker := time.NewTicker(24 * time.Hour)

	for {
		select {
		case <-keepAliveTicker.C:
			log.Println("TeslaAPI - Keep Alive")
			reportAPIError := func() {
				log.Println(err)
				err2 := smtp.SendMail("mail.cedartechnology.com:587",
					smtp.PlainAuth("", "pi@cedartechnology.com", "7444561", "mail.cedartechnology.com"),
					"pi@cedartechnology.com", []string{"ian.abercrombie@cedartechnology.com"}, []byte("From: Aberhome1\r\nTo: Ian.Abercrombie@CedarTechnology.com\r\nSubject: Tesla Keep Alive Failed!\r\n\r\n"+err.Error()))
				if err2 != nil {
					log.Println("Failed to send email about the error above. ", err2)
				}
			}

			if API == nil {
				API, err = TeslaAPI.New(verbose)
				if err != nil {
					log.Println("teslaKeepAlive - failed to create a new API object")
					reportAPIError()
				}
			}
			if API != nil {
				err = API.GetVehicleId()
				if err != nil {
					log.Println("teslaKeepAlive - Failed to get the vehicle id.")
					reportAPIError()
					API, err = TeslaAPI.New(verbose)
					if err != nil {
						reportAPIError()
					}
				}
			}
		}
	}
}

// This function will look at the various inverter parameters and work out if there is power available for car charging or water heating
// It bases this calculation on the current battery state of charge, the battery charging current and the difference between the setpoint
// and the actual battery voltage
func calculatePowerAvailable() {
	//	var iBatt float32
	var soc float32
	//	var frequency float64
	var delta int16

	for {
		//		soc = iValues.GetSOC()
		//		frequency = iValues.GetFrequency()

		powerState := iValues.GetChargeLevel()

		// Set the total car charging current for all cars charging
		carCurrent := float32(0.0)
		for i := range slaves {
			carCurrent += float32(slaves[i].GetCurrent()) / 100.0
		}
		TeslaParameters.SetCurrent(carCurrent)

		//		fmt.Printf("soc = %f\nDelta = %f\nMaxDelta = %f\nMinDelta = %f\n", soc, vBattDelta, vBattDeltaMax, vBattDeltaMin)

		//		fmt.Printf ("F = %0.2fHz : Cars = %0.2fA : available = %0.2fA : SOC = %0.2f%%: setpoint = %0.2fV : vBatt = %0.2fV", frequency, carCurrent, TeslaParameters.GetMaxAmps(), soc, vSetpoint, vBatt)

		if (carCurrent > 6) && (Heater.GetSetting() > 0) {
			// If the car is trying to charge and the heater is on, turn the heater off
			Heater.SetHeater(0)
		}
		if iValues.AutoGn {
			// If the generator is running turn off the Tesla and the auxiliary heater
			TeslaParameters.SetMaxAmps(0)
			if Heater.GetSetting() > 0 {
				Heater.SetHeater(0)
			}
		} else if powerState == 1 { // If the delta is less then the minimum we can take more power
			if carCurrent > 1 {
				// Car is charging so try and increase the charge rate
				delta = 0 - int16(int(iValues.GetAmps())/5)
				//				delta = 1
				if !TeslaParameters.ChangeCurrent(delta) {
					// Charge rate increase was not accepted so turn up the auxiliary heater
					Heater.Increase(iValues.GetFrequency())
				} else {
					// Tesla accepted the increase so we should drop the heater a bit ignoring hold time setting
					Heater.Decrease(true)
				}
			} else {
				// No car charging requested so set the available current to 25.0 amps and turn up the auxiliary heater
				TeslaParameters.SetMaxAmps(25.0)
				Heater.Increase(iValues.GetFrequency())
			}
		} else if powerState == -1 { // If the delta is more than the max we need to reduce the load to give the battery chance to charge up
			if !Heater.Decrease(false) {
				// if the heater is already off and the car is charging then reduce the car charge rate
				if carCurrent > 1 {
					// If we are not discharging and the car is not charging at 6 amps or above then leave it alone
					if (iValues.GetAmps() > 1) || (carCurrent > 6) {
						// If the state of charge is 95% or more don't let the car current fall below 8 amps
						if (soc < 95.0) || (carCurrent > 8) {
							switch {
							case iValues.GetAmps() > 35:
								TeslaParameters.ChangeCurrent(0 - int16(iValues.GetAmps()/7))
							case carCurrent > 30.0:
								TeslaParameters.ChangeCurrent(-3)
							case carCurrent > 20.0:
								TeslaParameters.ChangeCurrent(-2)
							default:
								TeslaParameters.ChangeCurrent(-1)
							}
						}
					}
				}
			}
		}
		time.Sleep(time.Second * 15)
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
