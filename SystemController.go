package main

import (
	"SystemController/Params"
	"SystemController/TeslaAPI"
	"SystemController/twcMessage"
	"SystemController/twcSlave"
	_ "crypto/aes"
	"database/sql"
	"flag"
	"fmt"
	"github.com/IanAber/SMACanMessages"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/brutella/can"
	_ "github.com/go-sql-driver/mysql"
	"github.com/goburrow/serial"
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
	SolarProduction  struct {
		power  float32
		logged time.Time
	}
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

func handleCANFrame(frm can.Frame) {
	switch frm.ID {
	case 0x305: // Battery voltage, current and state of charge
		c305 := SMACanMessages.NewCan305(frm.Data[0:])
		iValues.SetVolts(c305.VBatt())
		iValues.SetAmps(c305.IBatt())
		iValues.SetSOC(c305.SocBatt())
		API.AllowStart = c305.IBatt() < -30
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
		log.Println("To enable CAN on the Rspbreey Pi follow this article - https://projects-raspberry.com/how-to-connect-raspberry-pi-to-can-bus/")
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

	log.Println("Connecting to [", sConnectionString, "]")
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

	// The Tesla serial port should be set using UDEV rules to /dev/ttyTesla
	flag.StringVar(&address, "teslaPort", "/dev/ttyTesla", "Serial port address")
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
		log.Fatalf("ERROR - %s - Cannot connect to the Tesla RS485 port at %s\nSorry. I am givng up!", err, config.Address)
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

	powerTicker := time.NewTicker(time.Second * 5)
	for range powerTicker.C {
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
			//			log.Println("Increasing consumption")
			// Inverter current is at 48V so approx. 5 times car current. We should push it up in small stages
			delta = 0 - int16(iValues.GetAmps()/10) // Charging shows as a negative inverter current
			//			log.Println("Inverter current =", iValues.GetAmps(), "A - (negative = charging)")
			if Electrolyser.currentSetting < 100 {
				// If the electrolysers are running below 100% then limit the car to 20Amps
				TeslaParameters.SetSystemAmps(10)
			} else {
				// Electrolysers are running at 100% so allow the car to run up to the full 48amps
				TeslaParameters.SetSystemAmps(48)
			}
			//			log.Println("Tesla set to", TeslaParameters.GetMaxAmps(), "Amps Max.")
			if carCurrent > 1 {
				// Car is charging so try and increase the charge rate
				//				log.Println("Change Tesla", delta, "A")
				if !TeslaParameters.ChangeCurrent(delta) {
					// Charge rate increase was not accepted so turn up the electrolyser
					//					log.Println("Increase Electrolyser ", delta, "%")
					if !Electrolyser.ChangeRate(1) {
						Heater.Increase(iValues.GetFrequency())
					}
				}
			} else {
				// No car charging requested so set the available current to 15.0 amps and turn up the auxiliary heater
				//				log.Println("Tesla not charging to default to 15A and increase electrolyser")
				TeslaParameters.SetMaxAmps(15.0)
				// If the frequency is over 60.9 the solar inverters are throttled, so we should whack the electrolyser up to full immediately
				if iValues.frequency > 60.9 {
					delta = 100
				} else {
					delta = 1
				}
				if !Electrolyser.ChangeRate(delta) {
					// Electrolyser did not increase, so we should turn the heaters up.
					//					log.Println("Electrolyser did not increase so increasing water heater")
					Heater.Increase(iValues.GetFrequency())
				}
			}
		} else if powerState == -1 { // If the delta is more than the max we need to reduce the load to give the battery chance to charge up
			// Turn the water heat down first
			//			log.Println("Reducing consumption")
			if !Heater.Decrease(false) {
				delta = 0 - int16(iValues.GetAmps()/5) // Inverter current is at 48V so 5 times the 240 car current
				// Delta is negative here
				// log.Println("Inverter current =", iValues.GetAmps(), "A Discharging - Delta set to", delta)

				// if the heater is already off and the Tesla is above 20A then reduce the Tesla
				if carCurrent > 10 {
					// Drop the car current
					//					log.Println("Car > 10A so change it", delta, "A")
					TeslaParameters.ChangeCurrent(delta)
				} else {
					// Car is not above 10A so derease the Electrolysers first
					//					log.Println("Car < 20A so changing Electrolysers", delta, "%")
					if !Electrolyser.ChangeRate(delta) {
						// Electrolyser did not decrease, perhaps because it is already zero, so drop the car rate
						//						log.Println("Electrolyser is 0 so changing the Tesla ", delta, "A")
						TeslaParameters.ChangeCurrent(delta)
					}
				}
			}
		}
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
	lastSolarPump := Heater.GetSolarPump()
	var err error
	hotTankTemp := int16(1000)

	loggingTicker := time.NewTicker(time.Second)
	for range loggingTicker.C {
		newFrequency := iValues.GetFrequency()
		newVsetpoint := iValues.GetSetPoint()
		newVbatt := iValues.GetVolts()
		newIbatt := iValues.GetAmps()
		newSoc := iValues.GetSOC()
		newIavailable := TeslaParameters.GetMaxAmps()
		newIused := TeslaParameters.GetCurrent()
		newHeatersetting := Heater.GetSetting()
		newHeaterpump := Heater.GetPump()
		newSolarPump := Heater.GetSolarPump()

		if pDB == nil {
			pDB, err = connectToDatabase()
			if err != nil {
				log.Println("Error opening the database ", err)
				pDB = nil
				continue
			}
		}

		if lastSolarPump != newSolarPump {
			// Log the new solar pump value if we changed it
			if _, err := pDB.Exec("INSERT INTO solar_pump (pump_power) values(?)", newSolarPump); err != nil {
				log.Println(err)
			}
		}
		lastSolarPump = newSolarPump
		if (newFrequency != lastFrequency) || (newVsetpoint != lastVsetpoint) || (newVbatt != lastVbatt) || (newIbatt != lastIbatt) || (newSoc != lastSoc) {
			lastFrequency = newFrequency
			lastVsetpoint = newVsetpoint
			lastVbatt = newVbatt
			lastIbatt = newIbatt
			lastSoc = newSoc
			var _, err = pDB.Exec("insert into inverter_values (frequency, vSetpoint, vBatt, iBatt, state_of_charge) values (?, ?, ?, ?, ?)",
				newFrequency, newVsetpoint, newVbatt, newIbatt, newSoc)
			if err != nil {
				log.Printf("Error writing inverter values to the database - %s", err)
				_ = pDB.Close()
				pDB = nil
				continue
			}
		}
		if (newIavailable != lastIavailable) || (newIused != lastIused) {
			lastIavailable = newIavailable
			lastIused = newIused
			_, err := pDB.Exec("insert into tesla_values(iSetpoint, iCharging) values(?, ?)", newIavailable, newIused)
			if err != nil {
				log.Printf("Error writing Tesla values to the database - %s", err)
				_ = pDB.Close()
				pDB = nil
				continue
			}
		}
		if (newHeatersetting != lastHeatersetting) || (newHeaterpump != lastHeaterpump) {
			lastHeatersetting = newHeatersetting
			lastHeaterpump = newHeaterpump
			lastIused = newIused
			_, err := pDB.Exec("insert into water_heater_operation(status, pump) values(?, ?)", newHeatersetting, newHeaterpump)
			if err != nil {
				log.Printf("Error writing heater values to the database - %s", err)
				_ = pDB.Close()
				pDB = nil
				continue
			}
		}
		// Get the hot tank temperature
		var err = pDB.QueryRow("select greatest(`TSH0`, `TSH1`, `TSH2`) as maxtemp from `chillii_analogue_input` where `logged` > date_add(now(), interval -5 minute) order by `logged` desc limit 1;").Scan(&hotTankTemp)
		if err != nil {
			Heater.SetHotTankTemp(1000) // Be safe. If we can't get the temperature assume it is boiling to shut down the heater.
			if err != sql.ErrNoRows {
				log.Printf("Error fetching hot tank temperature from the database - %s", err)
				err = pDB.Close()
				pDB = nil
				continue
			}
		}
		Heater.SetHotTankTemp(hotTankTemp)
	}
}

func GetTemperatures() {
	log.Println("GetTemperatures...")
	var temperatures struct {
		TSOC1 int16
		TSOPI int16
		TSOPO int16
		TSH0  int16
		TSH1  int16
		TSH2  int16

		TSC0 int16
		TSC1 int16
		TSC2 int16
		TOU  int16
		TSOS int16
		TIN1 int16

		TCHCI1 int16
		TCHCO1 int16
		TCHEI1 int16
		TCHEO1 int16
		TCHGI1 int16
		TCHGO1 int16
	}
	esp1 := NewESPTemperature("http://ESPTEMP1.home")
	esp2 := NewESPTemperature("http://ESPTEMP2.home")
	esp3 := NewESPTemperature("http://ESPTEMP3.home")
	go esp1.readTemperatures()
	go esp2.readTemperatures()
	go esp3.readTemperatures()

	tempTicker := time.NewTicker(time.Second * 5)
	for range tempTicker.C {
		esp1.readTemperatures()
		temps := esp1.getTemperatures()
		temperatures.TSOC1 = int16(temps[0] * 10)
		temperatures.TSOPI = int16(temps[1] * 10)
		temperatures.TSOPO = int16(temps[2] * 10)
		temperatures.TSH0 = int16(temps[3] * 10)
		temperatures.TSH1 = int16(temps[4] * 10)
		temperatures.TSH2 = int16(temps[5] * 10)

		esp2.readTemperatures()
		temps = esp2.getTemperatures()
		temperatures.TSC0 = int16(temps[0] * 10)
		temperatures.TSC1 = int16(temps[1] * 10)
		temperatures.TSC2 = int16(temps[2] * 10)
		temperatures.TOU = int16(temps[3] * 10)
		temperatures.TSOS = int16(temps[4] * 10)
		temperatures.TIN1 = int16(temps[5] * 10)

		esp3.readTemperatures()
		temps = esp3.getTemperatures()
		temperatures.TCHCI1 = int16(temps[0] * 10)
		temperatures.TCHCO1 = int16(temps[1] * 10)
		temperatures.TCHEI1 = int16(temps[2] * 10)
		temperatures.TCHEO1 = int16(temps[3] * 10)
		temperatures.TCHGI1 = int16(temps[4] * 10)
		temperatures.TCHGO1 = int16(temps[5] * 10)

		// Signal the solar pump controller that we have new values
		solarTemps := new(SolarTemps)
		solarTemps.collector = temperatures.TSOC1
		solarTemps.input = temperatures.TSOPI
		solarTemps.output = temperatures.TSOPO
		solarTemps.tankTop = temperatures.TSH0
		solarTemps.tankMid = temperatures.TSH1
		solarTemps.tankBottom = temperatures.TSH2

		if tempUpdate != nil {
			//			log.Print("Signal solarTemps")
			tempUpdate <- solarTemps
		}

		//		log.Println("Write chillii analog inputs")
		if pDB == nil {
			if dbPtr, err := connectToDatabase(); err != nil {
				log.Println(err)
				continue
			} else {
				pDB = dbPtr
			}
		}
		if _, err := pDB.Exec(`INSERT INTO logging.chillii_analogue_input (TSOC_1, TSOPI, TSOPO, TSH0, TSH1, TSH2,
		                                   TSC0, TSC1, TSC2, TOU, TSOS, TIN_1,
		                                   TCHCI_1, TCHCO_1, TCHEI_1, TCHEO_1, TCHGI_1, TCHGO_1)
										VALUES (?,?,?,?,?,?,
										        ?,?,?,?,?,?,
										        ?,?,?,?,?,?)`,
			temperatures.TSOC1, temperatures.TSOPI, temperatures.TSOPO, temperatures.TSH0, temperatures.TSH1, temperatures.TSH2,
			temperatures.TSC0, temperatures.TSC1, temperatures.TSC2, temperatures.TOU, temperatures.TSOS, temperatures.TIN1,
			temperatures.TCHCI1, temperatures.TCHCO1, temperatures.TCHEI1, temperatures.TCHEO1, temperatures.TCHGI1, temperatures.TCHGO1); err != nil {
			log.Print(err)
		}
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

	log.Println("********** Tesla Keep Alive started. *********")

	// Start the power management loop
	go calculatePowerAvailable()
	log.Println("********** Calculate Power started. *********")

	// Start the heater kill loop to ensure the heater is not drawing from the battery.
	go killHeaterOnDischarge()
	log.Println("********** Kill heater on discharge started. *********")

	go logToDatabase()
	log.Println("********** Database logger started started. *********")

	go GetTemperatures()
	log.Println("********** TGet Temperatures started. *********")

	go ManageSolarPump()
	log.Println("********** Solar Pump started. *********")

	go ManageCpuTemp()
	log.Println("********** CPU Temperature manager started. *********")

	//	go LogPumps()

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
