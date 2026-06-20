package main

import (
	"SystemController/Params"
	"SystemController/twcMessage"
	"SystemController/twcSlave"
	_ "crypto/aes"
	"database/sql"
	"errors"
	"flag"
	"log"
	"os"
	"time"

	"github.com/IanAber/SMACanMessages"

	"github.com/brutella/can"
	_ "github.com/go-sql-driver/mysql"
	"github.com/goburrow/serial"
)

// Version 2 makes parameters editable via the WEB interface

//const CHARGINGLINKS = `<a href="/startCharging">Start Charging</a><br><a href="/stopCharging">Stop Charging</a>`

//const maxGasPressure = 34.0 // Pressure above which we do not increase the electrolyser output

var (
	address               string
	baudrate              int
	databits              int
	stopbits              int
	parity                string
	verbose               bool
	apiPort               uint
	databaseServer        string
	databasePort          string
	databaseName          string
	databaseLogin         string
	databasePassword      string
	ChargingConstantsFile string
	masterAddress         uint
	port                  serial.Port
	TeslaParameters       Params.Params
	Heater                *HeaterSetting
	//	Electrolyser          ElectrolyserSetting
	iValues         InverterValues
	slaves          []twcSlave.Slave
	pDB             *sql.DB
	SolarProduction struct {
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
		slaves[i].SendMasterHeartbeat(masterAddress)
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
	// Tesla can only accept charging currents from 6 amps upwards. We are trying to set a current of less
	// than 6 amps fix it to 6 amps unless the battery state of charge is less than 50%
	// If we end up with less than 6 amps for each car, stop charging until we have more available
	if maxAmps < 600 && maxAmps > 0 {
		if iValues.GetSOC() >= 50 {
			maxAmps = 600
		} else {
			maxAmps = 0
		}
	}
	if TeslaParameters.IsLogging() {
		log.Printf("Setting maximum car current to %fA", float64(maxAmps)/100)
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
		//		API.AllowStart = c305.IBatt() < -30
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
	case 0x308:
		c308 := NewCan308(frm.Data[0:])
		iValues.inverterPower = c308.LoadPwr()
	case 0x351:
		c351 := NewBMS351(frm.Data[0:])
		iValues.bmsChargeVolts = c351.ChargeVolts()
		iValues.bmsDischargeVolts = c351.DischargeVoltage()
		iValues.bmsChargeCurrentMax = c351.ChargeCurrentLimit()
		iValues.bmsDichargeCurrentMax = c351.DischargeCurrentLimit()
	case 0x355:
		c355 := NewBMS355(frm.Data[0:])
		iValues.bmsSOC = c355.SOC()
		iValues.bmsSOH = c355.SOH()
	case 0x356:
		c356 := NewBMS356(frm.Data[0:])
		iValues.SetBmsVolts(c356.VBatt())
		iValues.SetBmsAmps(c356.IBatt())
		iValues.bmsTBat = c356.TBatt()
		//		log.Printf("%fV : %fA : %fT", c356.VBatt(), c356.IBatt(), c356.TBatt())
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
	// Set the time zone to Local to correctly record times
	var sConnectionString = databaseLogin + ":" + databasePassword + "@tcp(" + databaseServer + ":" + databasePort + ")/" + databaseName + "?loc=Local"

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
	flag.StringVar(&ChargingConstantsFile, "constantsFile", "/var/www/html/params/charge_params.json", "Path of the file in which the charging constants are stored")
	flag.Parse()
	log.SetFlags(log.Lshortfile | log.LstdFlags)

	// Initialise the current values
	TeslaParameters.Reset()
	Heater = NewHeaterSetting()
	// Set up the quintic function to control charging
	err := iValues.LoadFunctionConstants(ChargingConstantsFile)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("Verbose = ", verbose)

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

	log.SetFlags(log.Lshortfile | log.Ldate | log.Ltime)
	//	Electrolyser.Enabled = true
	//	Electrolyser.gasPressure = maxGasPressure
	// Start handling incoming CAN bus messages
	go processCANFrames()
}

func CloseDB() {
	_ = pDB.Close()
}

func logToDatabase() {
	defer CloseDB()

	lastFrequency := iValues.GetFrequency()
	lastVsetpoint := iValues.GetSetPoint()
	lastVbatt := iValues.GetVolts()
	lastSoc := iValues.GetSOC()
	lastPower := iValues.GetPower()
	lastIavailable := TeslaParameters.GetMaxAmps()
	lastIused := TeslaParameters.GetCurrent()
	//lastHeatersetting := Heater.GetSetting()
	//lastHeaterpump := Heater.GetPump()
	//	lastSolarPump := Heater.GetSolarPump()
	lastSolarPump := uint8(255)
	var err error
	hotTankTemp := int16(1000)
	lastbmsVBat := 0.0
	lastbmsIBat := 0.0
	lastbmsTBat := 0.0
	lastbmsChargeVolts := 0.0
	lastbmsDischargeVolts := 0.0
	lastbmsChargeCurrentMax := 0.0
	lastbmsDischargeCurrentMax := 0.0
	lastbmsSOC := uint16(0)
	lastbmsSOH := uint16(0)

	loggingTicker := time.NewTicker(time.Second)
	for range loggingTicker.C {
		iValues.mu.Lock()
		newbmsVBat := iValues.bmsVBat
		newbmsIBat := iValues.bmsIBat
		newbmsTBat := iValues.bmsTBat
		newbmsChargeVolts := iValues.bmsChargeVolts
		newbmsDischargeVolts := iValues.bmsDischargeVolts
		newbmsChargeCurrentMax := iValues.bmsChargeCurrentMax
		newbmsDischargeCurrentMax := iValues.bmsDichargeCurrentMax
		newbmsSOC := iValues.bmsSOC
		newbmsSOH := iValues.bmsSOH
		iValues.mu.Unlock()

		newFrequency := iValues.GetFrequency()
		newVsetpoint := iValues.GetSetPoint()
		newVbatt := iValues.GetVolts()
		newSoc := iValues.GetSOC()
		newIavailable := TeslaParameters.GetMaxAmps()
		newIused := TeslaParameters.GetCurrent()
		newPower := iValues.GetPower()
		//newHeatersetting := Heater.GetSetting()
		//newHeaterpump := Heater.GetPump()
		newSolarPump := Heater.GetSolarPump()

		if pDB == nil {
			pDB, err = connectToDatabase()
			if err != nil {
				log.Println("Error opening the database ", err)
				pDB = nil
				continue
			}
		}

		if lastbmsChargeCurrentMax != newbmsChargeCurrentMax || lastbmsChargeVolts != newbmsChargeVolts || lastbmsDischargeCurrentMax != newbmsDischargeCurrentMax ||
			lastbmsDischargeVolts != newbmsDischargeVolts || lastbmsIBat != newbmsIBat || lastbmsSOC != newbmsSOC || lastbmsSOH != newbmsSOH ||
			lastbmsVBat != newbmsVBat || lastbmsTBat != newbmsTBat {
			if _, err := pDB.Exec("INSERT INTO logging.bms_values (vbat, ibat, tbat, chargeVolts, dischargeVolts, chargeAmpsMax, dischargeAmpsMax, soc, soh) VALUES(?,?,?,?,?,?,?,?,?)", newbmsVBat, newbmsIBat, newbmsTBat, newbmsChargeVolts, newbmsDischargeVolts, newbmsChargeCurrentMax, newbmsDischargeCurrentMax,
				newbmsSOC, newbmsSOH); err != nil {
				log.Println(err)
			}
			lastbmsChargeCurrentMax = newbmsChargeCurrentMax
			lastbmsChargeVolts = newbmsChargeVolts
			lastbmsDischargeCurrentMax = newbmsDischargeCurrentMax
			lastbmsDischargeVolts = newbmsDischargeVolts
			//lastbmsIBat = newbmsIBat
			lastbmsSOC = newbmsSOC
			lastbmsSOH = newbmsSOH
			lastbmsVBat = newbmsVBat
			lastbmsTBat = newbmsTBat
		}
		if lastSolarPump != newSolarPump {
			// Log the new solar pump value if we changed it
			if _, err := pDB.Exec("INSERT INTO solar_pump (pump_power) values(?)", newSolarPump); err != nil {
				log.Println(err)
			}
		}
		lastSolarPump = newSolarPump
		if (newFrequency != lastFrequency) || (newVsetpoint != lastVsetpoint) || (newVbatt != lastVbatt) || (newbmsIBat != lastbmsIBat) || (newSoc != lastSoc) || (lastPower != newPower) {
			lastFrequency = newFrequency
			lastVsetpoint = newVsetpoint
			lastVbatt = newVbatt
			//			lastIbatt = newIbatt
			lastbmsIBat = newbmsIBat
			lastSoc = newSoc
			var _, err = pDB.Exec("insert into inverter_values (frequency, vSetpoint, vBatt, iBatt, state_of_charge, power) values (?, ?, ?, ?, ?, ?)",
				newFrequency, newVsetpoint, newVbatt, newbmsIBat, newSoc, newPower)
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
		//if (newHeatersetting != lastHeatersetting) || (newHeaterpump != lastHeaterpump) {
		//	lastHeatersetting = newHeatersetting
		//	lastHeaterpump = newHeaterpump
		//	lastIused = newIused
		//	_, err := pDB.Exec("insert into water_heater_operation(status, pump) values(?, ?)", newHeatersetting, newHeaterpump)
		//	if err != nil {
		//		log.Printf("Error writing heater values to the database - %s", err)
		//		_ = pDB.Close()
		//		pDB = nil
		//		continue
		//	}
		//}
		// Get the hot tank temperature
		var err = pDB.QueryRow("select greatest(`HotTankTop`, `HotTankMiddle`, `HotTankBottom`) as maxtemp from `temperatures` where `logged` > date_add(now(), interval -5 minute) order by `logged` desc limit 1;").Scan(&hotTankTemp)
		if err != nil {
			Heater.SetHotTankTemp(1000) // Be safe. If we can't get the temperature assume it is boiling to shut down the heater.
			if !errors.Is(err, sql.ErrNoRows) {
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
		SolarCollector int16
		SolarInlet     int16
		SolarOutlet    int16
		HotTankTop     int16
		HotTankMiddle  int16
		HotTankBottom  int16

		ColdTankBottom     int16
		ColdTankTop        int16
		ColdTankMiddle     int16
		MatsInput          int16
		MatsOutput         int16
		DehumidifierOutput int16
		//		AmbientOutside int16 //TOU
		//		SolarExchanger int16 //TSOS
		//		Bedroom        int16 //TIN1

		CondenserIn   int16
		CondenserOut  int16
		EvaporatorIn  int16
		EvaporatorOut int16
		GeneratorIn   int16
		GeneratorOut  int16
	}
	esp1 := NewESPTemperature("http://ESPTEMP1")
	esp2 := NewESPTemperature("http://ESPTEMP2")
	esp3 := NewESPTemperature("http://ESPTEMP3")
	esp1.readTemperatures()
	esp2.readTemperatures()
	esp3.readTemperatures()

	tempTicker := time.NewTicker(time.Second * 5)
	for range tempTicker.C {
		//		esp1.readTemperatures()
		temps := esp1.getTemperatures()
		temperatures.SolarCollector = int16(temps[0] * 10)
		temperatures.SolarInlet = int16(temps[1] * 10)
		temperatures.SolarOutlet = int16(temps[2] * 10)
		temperatures.HotTankTop = int16(temps[3] * 10)
		temperatures.HotTankMiddle = int16(temps[4] * 10)
		temperatures.HotTankBottom = int16(temps[5] * 10)

		//		esp2.readTemperatures()
		temps = esp2.getTemperatures()
		temperatures.ColdTankBottom = int16(temps[0] * 10)
		temperatures.ColdTankTop = int16(temps[1] * 10)
		temperatures.ColdTankMiddle = int16(temps[2] * 10)
		temperatures.MatsInput = int16(temps[3] * 10)
		temperatures.MatsOutput = int16(temps[4] * 10)
		temperatures.DehumidifierOutput = int16(temps[5] * 10)

		//		esp3.readTemperatures()
		temps = esp3.getTemperatures()
		temperatures.GeneratorIn = int16(temps[0] * 10)
		temperatures.GeneratorOut = int16(temps[1] * 10)
		temperatures.EvaporatorIn = int16(temps[2] * 10)
		temperatures.EvaporatorOut = int16(temps[3] * 10)
		temperatures.CondenserIn = int16(temps[4] * 10)
		temperatures.CondenserOut = int16(temps[5] * 10)

		// Signal the solar pump controller that we have new values
		solarTemps := new(SolarTemps)
		solarTemps.collector = temperatures.SolarCollector
		solarTemps.input = temperatures.SolarInlet
		solarTemps.output = temperatures.SolarOutlet
		solarTemps.tankTop = temperatures.HotTankTop
		solarTemps.tankMid = temperatures.HotTankMiddle
		solarTemps.tankBottom = temperatures.HotTankBottom
		//		solarTemps.exchanger = temperatures.SolarExchanger

		if tempUpdate != nil {

			// Force temperatures in case of failure.
			//log.Print("Signal solarTemps", solarTemps)
			//solarTemps.collector = 1000
			//solarTemps.input = 600
			//solarTemps.exchanger = 950
			//solarTemps.output = 950
			//solarTemps.tankMid = 60
			//solarTemps.tankTop = 60
			//solarTemps.tankBottom = 60
			tempUpdate <- solarTemps
		}

		if pDB == nil {
			if dbPtr, err := connectToDatabase(); err != nil {
				log.Println(err)
				continue
			} else {
				pDB = dbPtr
			}
		}
		if _, err := pDB.Exec(`INSERT INTO logging.temperatures (HotTankTop, HotTankMiddle, HotTankBottom,
                                  						BufferTankTop, BufferTankMiddle, BufferTankBottom, 
                                  						GeneratorIn, GeneratorOut, EvaportatorIn, EvaporatorOut, CondenserIn, CondenserOut, 
                                  						SolarCollector, SolarInlet, SolarOutlet, MatsInput, 
                                  						MatsOutput, DehumidifierOutput)
										VALUES (?,?,?,?,?,?,
										        ?,?,?,?,?,?,
										        ?,?,?,?,?,?)`,
			temperatures.HotTankTop, temperatures.HotTankMiddle, temperatures.HotTankBottom,
			temperatures.ColdTankTop, temperatures.ColdTankMiddle, temperatures.ColdTankBottom,
			temperatures.GeneratorIn, temperatures.GeneratorOut, temperatures.EvaporatorIn, temperatures.EvaporatorOut, temperatures.CondenserIn, temperatures.CondenserOut,
			temperatures.SolarCollector, temperatures.SolarInlet, temperatures.SolarOutlet, temperatures.MatsInput,
			temperatures.MatsOutput, temperatures.DehumidifierOutput); err != nil {
			log.Print(err)
		}
		go esp1.readTemperatures()
		go esp2.readTemperatures()
		go esp3.readTemperatures()

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

	log.Println("********** Tesla Keep Alive started. *********")

	// Start the power management loop
	go calculatePowerAvailable()
	log.Println("********** Calculate Power started. *********")

	//// Start the heater kill loop to ensure the heater is not drawing from the battery.
	//go killHeaterOnDischarge()
	//log.Println("********** Kill heater on discharge started. *********")

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
				if !errors.Is(err, serial.ErrTimeout) {
					log.Println(err)
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
					//					log.Print(msg.GetDetails())
					switch msg.GetCode() {
					//						case 0xfbe0: fmt.Printf("To Slave %04x | Status = %02x | SetPoint = %0.2f | = %0.2f\n", msg.GetToAddress(), msg.GetStatus(), float32(msg.GetSetPoint()) / 100, float32(msg.GetCurrent()) / 100)
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
		//		log.Println("Tesla loop")
		time.Sleep(100 * time.Millisecond)
	}
}
