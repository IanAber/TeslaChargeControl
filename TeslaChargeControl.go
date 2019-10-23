package main

import (
	"CanMessages/CAN_010"
	"CanMessages/CAN_305"
	"CanMessages/CAN_306"
	"CanMessages/CAN_307"
	"TeslaChargeControl/InverterValues"
	"TeslaChargeControl/Params"
	"TeslaChargeControl/heaterSetting"
	"TeslaChargeControl/twcMessage"
	"TeslaChargeControl/twcSlave"
	"database/sql"
	"flag"
	"fmt"
	"github.com/brutella/can"
	"github.com/goburrow/serial"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"log"
	"log/syslog"
	"net/http"
	"os"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

var (
	address          string
	baudrate         int
	databits         int
	stopbits         int
	parity           string
	listenMode       bool
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

//	hotTankTemp			int16
)

func findSlave(slaves []twcSlave.Slave, address uint) int {
	for i, s := range slaves {
		if s.GetAddress() == address {
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
		s := twcSlave.New(msg.GetFromAddress(), listenMode, port)
		s.UpdateValues(&msg)
		*slaves = append(*slaves, s)
	}
}

// If we don't already have the slave, add it to the list
func processSlaveLinkReady(msg twcMessage.TwcMessage, slaves *[]twcSlave.Slave) {
	i := findSlave(*slaves, msg.GetFromAddress())
	if i < 0 {
		s := twcSlave.New(msg.GetFromAddress(), listenMode, port)
		s.UpdateValues(&msg)
		*slaves = append(*slaves, s)
		glog.Info("Slave added [%04x]", msg.GetFromAddress())
	}
}

func checkSlaveTimeouts(slaves []twcSlave.Slave) []twcSlave.Slave {
	for i, s := range slaves {
		if s.TimeSinceLastHeartbeat() > (10 * time.Second) {
			glog.Infof("=======> Slave %04x has gone away! Time span = %d > 10 seconds (%d). <=======\n", s.GetAddress(), s.TimeSinceLastHeartbeat(), time.Second*10)
			glog.Flush()
			slaves[i] = slaves[len(slaves)-1]
			return slaves[:len(slaves)-1]
		}
	}
	return slaves
}

// Heartbeat status to the slave
// 00 = no change
// 05 = Tell slave to change its setpoint

func sendHearbeatsToSlaves(slaves []twcSlave.Slave, masterAddress uint) {
	for _, s := range slaves {
		s.SendMasterHeartbeat(masterAddress, listenMode)
	}
}

func divideMaxAmpsAmongstSlaves(slaves []twcSlave.Slave, maxAmps int) {
	activeCars := 0

	// Find out how many cars are waiting to charge, actively charging or starting to charge
	for _, s := range slaves {
		// Count how many cars are trying to charge.
		if s.RequestCharge() {
			activeCars++
		}
	}
	// If there is at least one car then divide the current between them equally
	if activeCars > 0 {
		maxAmps = maxAmps / activeCars
	}
	// If we end up with less than 5 amps for each car, stop charging until we have more available
	if maxAmps < 500 {
		maxAmps = 0
	}
	// Share out the current amongst the cars waiting to charge or actively charging
	for i, s := range slaves {
		if s.RequestCharge() {
			slaves[i].SetCurrent(maxAmps)
		}
	}
}

func setUpWebSite() {
	router := mux.NewRouter().StrictSlash(true)
	router.HandleFunc("/", getValues).Methods("GET")
	router.HandleFunc("/disableHeater", disableHeater).Methods("GET")
	router.HandleFunc("/enableHeater", enableHeater).Methods("GET")
	log.Fatal(http.ListenAndServe(":8080", router))
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
	for i, s := range slaves {
		if i > 0 {
			_, _ = fmt.Fprint(w, ',')
		}
		_, _ = fmt.Fprintf(w, `
			{
				"Current":%0.2f,
				"maxAmps":%0.2f,
				"status":"%s"
			}`, float32(s.GetCurrent())/100, float32(s.GetAllowed())/100, s.GetStatus())
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
%s
	}
}`, Heater.GetSetting(), sPump, Heater.GetEnabled(), iValues.GetFrequency(), iValues.GetSetPoint(), iValues.GetVolts(), iValues.GetAmps(), iValues.GetSOC(), iValues.GetFlags())
}

func handleCANFrame(frm can.Frame) {
	switch frm.ID {
	case 0x305: // Battery voltage, current and state of charge
		c305 := CAN_305.New([]byte(frm.Data[0:]))

		iValues.SetVolts(c305.VBatt())
		iValues.SetAmps(c305.IBatt())
		iValues.SetSOC(c305.SocBatt())

	case 0x306: // Charge procedure, Operating state, Active error, Charge set point
		c306 := CAN_306.New([]byte(frm.Data[0:]))
		iValues.SetSetPoint(c306.ChargeSetPoint())

	case 0x010: // Frequency
		c010 := CAN_010.New([]byte(frm.Data[0:]))
		iValues.SetFrequency(c010.Frequency())

	case 0x307: // Relays and status
		c307 := CAN_307.New([]byte(frm.Data[0:]))
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

func processCANFrames(bus *can.Bus) {
	bus.SubscribeFunc(handleCANFrame)
	err := bus.ConnectAndPublish()
	if err != nil {
		glog.Errorf("ConnectAndPublish failed - %s", err)
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

func usage() {
	_, _ = fmt.Fprintf(os.Stderr, "usage: example -stderrthreshold=[INFO|WARN|FATAL] -log_dir=[string]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func init() {
	// Initialise the current values

	TeslaParameters.Reset()
	Heater = heaterSetting.New()

	flag.Usage = usage
	_ = flag.Set("log_dir", "/var/log")
	_ = flag.Set("stderrthreshold", "INFO")
	// NOTE: This next line is key you have to call flag.Parse() for the command line
	// options or "flags" that are defined in the glog module to be picked up.
	flag.Parse()

	// Set up logging
	logwriter, e := syslog.New(syslog.LOG_NOTICE, "myprog")
	if e == nil {
		log.SetOutput(logwriter)
	}

	// Get the settings
	flag.StringVar(&address, "a", "/dev/serial/by-path/platform-3f980000.usb-usb-0:1.2:1.0-port0", "Serial port address")
	flag.IntVar(&baudrate, "b", 9600, "Serial port baud rate")
	flag.IntVar(&databits, "d", 8, "Serial port data bits")
	flag.IntVar(&stopbits, "s", 1, "Serial port stop bits")
	flag.StringVar(&parity, "p", "N", "Serial port parity (N/E/O)")
	flag.UintVar(&masterAddress, "m", 0x7777, "Master TWC address")
	flag.UintVar(&apiPort, "i", 0x8080, "WEB port to listen on for API connections")
	flag.StringVar(&databaseServer, "q", "127.0.0.1", "MySQL Server")
	flag.StringVar(&databaseName, "n", "logging", "Database name")
	flag.StringVar(&databaseLogin, "u", "logger", "Database login user name")
	flag.StringVar(&databasePassword, "w", "logger", "Database user password")
	flag.StringVar(&databasePort, "o", "3306", "Database port")
	flag.BoolVar(&listenMode, "l", false, "Listen Mode prints output to stdout instead of sending over the wire.")
	flag.Parse()

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
		glog.Fatalf("ERROR - %s - Cannot connect to the Tesla RS485 port.\nSorry. I am givng up!", err)
	} else {
		glog.Info("Connected to Tesla Wall Charger.")
	}
	glog.Flush()
	port = p

	bus, err := can.NewBusForInterfaceWithName("can0")
	if err != nil {
		glog.Fatalf("Error starting CAN interface - %s -\nSorry, I am giving up", err)
	} else {
		glog.Info("Connected to CAN bus - monitoring the inverters.")
	}
	glog.Flush()

	// Set up the database connection
	pDB, err = connectToDatabase()
	if err != nil {
		glog.Fatalf("Failed to connec to to the database - %s - Sorry, I am giving up.", err)
	} else {
		glog.Info("Connected to the database")
	}
	glog.Flush()

	// Start handling incoming CAN messages
	go processCANFrames(bus)
}

// This function will look at the various inverter parameters and work out if there is power available for car charging or water heating
// It bases this calculation on the current battery state of charge, the battery charging current and the difference between the setpoint
// and the actual batter voltage
func calculatePowerAvailable() {
	var vSetpoint float32
	var vBatt float32
	var iBatt float32
	var soc float32
	var frequency float64
	var delta int16

	for {
		vSetpoint = iValues.GetSetPoint()
		vBatt = iValues.GetVolts()
		iBatt = iValues.GetAmps()
		soc = iValues.GetSOC()
		frequency = iValues.GetFrequency()

		// Set the total car charging current for all cars charging
		carCurrent := float32(0.0)
		for _, s := range slaves {
			carCurrent += float32(s.GetCurrent()) / 100.0
		}
		TeslaParameters.SetCurrent(carCurrent)

		//		fmt.Printf ("F = %0.2fHz : Cars = %0.2fA : available = %0.2fA : SOC = %0.2f%%: setpoint = %0.2fV : vBatt = %0.2fV", frequency, carCurrent, TeslaParameters.GetMaxAmps(), soc, vSetpoint, vBatt)
		if iValues.AutoGn {
			// If the generator is running turn off the Tesla and the auxiliary heater
			TeslaParameters.SetMaxAmps(0)
			Heater.SetHeater(0)
		} else if (frequency > 60.8) && (iBatt < 10) {
			// If the frequency is above 60.8 hertz we are getting more solar power than we are consuming so the first thing to do is check the car
			// to see if it could use more. If it is charging but at the allowed rate and that rate is less than 48 amps then push it up a bit.
			if carCurrent > 1 {
				// Car is charging so try and increase the charge rate
				if carCurrent > 40 {
					delta = 1
				} else if carCurrent > 35 {
					delta = 2
				} else if carCurrent > 30 {
					delta = 3
				} else if carCurrent > 25 {
					delta = 4
				} else if carCurrent > 20 {
					delta = 5
				} else if carCurrent > 15 {
					delta = 6
				} else if carCurrent > 10 {
					delta = 8
				} else {
					delta = 10
				}
				if !TeslaParameters.ChangeCurrent(delta) {
					// Charge rate increase was not accepted so turn up the auxiliary heater
					Heater.Increase(frequency)
				} else {
					// Tesla accepted the increase so we should drop the heater a bit ignoring and hold time set
					Heater.Decrease(true)
				}
			} else {
				// No car charging requested so set the available current to 10.0 amps and turn up the auxiliary heater
				//				fmt.Println("Set car current to 10A and increase heater")
				TeslaParameters.SetMaxAmps(10.0)
				Heater.Increase(frequency)
			}
		} else if frequency > 60.8 {
			if !TeslaParameters.ChangeCurrent(-1) {
				Heater.Decrease(true)
			}
		} else if frequency < 58 {
			// If frequency is this low we must be on generator power so stop the Tesla and Heaters
			if !Heater.Decrease(true) {
				if carCurrent > 1 {
					TeslaParameters.ChangeCurrent(int16(0 - carCurrent))
				}
			}

		} else if frequency < 59.2 {
			//			fmt.Println(" f < 59.2Hz - Decrease heater", )
			// The frequency is low so the Sunny Island is looking for more grid power to fulfill the load requirements.
			// We should dial back the heater and/or car a bit if the battery is less than 95% and not charging or
			// if we are discharging at more than 10 Amps
			if (soc < 95.0 && iBatt > 0) || (iBatt > 10) {
				if !Heater.Decrease(false) {
					//				fmt.Println("Heater is off so decrease car current")
					// if the heater is already off and the car is charging then reduce the car charge rate
					if carCurrent > 1 {
						// If the state of charge is 90% or more don't let the car current fall below 8 amps
						if (soc < 90.0) || (carCurrent > 8) {
							if carCurrent > 35.0 {
								TeslaParameters.ChangeCurrent(-8)
							} else if carCurrent > 30.0 {
								TeslaParameters.ChangeCurrent(-6)
							} else if carCurrent > 25.0 {
								TeslaParameters.ChangeCurrent(-5)
							} else if carCurrent > 20.0 {
								TeslaParameters.ChangeCurrent(-4)
							} else if carCurrent > 15.0 {
								TeslaParameters.ChangeCurrent(-3)
							} else if carCurrent > 10.0 {
								TeslaParameters.ChangeCurrent(-2)
							} else {
								TeslaParameters.ChangeCurrent(-1)
							}
						}
					}
				}
			}
		} else {
			// We are right around 60Hz so we should make sure that the battery is getting what it needs.
			if soc < 95.0 {
				// If the battery is less than 95% then ensure that the battery voltage is withing 5 volts of the setpoint
				if ((vSetpoint - vBatt) > 5) && (iBatt > -40) {
					//					fmt.Println ("battery charge voltage is low so decrease heater.")
					// We are at least 5v below the setpoint so drop the car current or heater rate
					if !Heater.Decrease(false) {
						// Heater is off so drop the charge rate available if there is a car charging to keep at least
						// 5 amps going into the battery
						if (carCurrent > 1.0) && (iBatt > -5) {
							//							fmt.Println("Heater is off and car is charging so decrease car.
							TeslaParameters.ChangeCurrent(-2)
						}
					}
				} else if ((vSetpoint - vBatt) < 1) || (iBatt < -80) {
					// We are within 1v of the setpoint so we can push the charge rate or heater up a bit
					if carCurrent > 1.0 {
						//						fmt.Println("Car is charging so increase current.")
						if !TeslaParameters.ChangeCurrent(+1) {
							//							fmt.Println("Car current = max so increase heater")
							Heater.Increase(frequency)
						} else {
							Heater.Decrease(true)
						}
					} else {
						if TeslaParameters.GetMaxAmps() < 10 {
							//							Set Tesla charge current to 10 Amps minimum to make sure the car gets a chance to charge if it needs it
							TeslaParameters.ChangeCurrent(10 - int16(TeslaParameters.GetMaxAmps()))
						} else {
							//							Car is not charging so increase heater.
							Heater.Increase(frequency)
						}
					}
				} else {
					// If the car is trying to charge give it at least 44 amps before allowing the heaters to run
					if (carCurrent > 1.0) && (Heater.GetSetting() > 0) && (carCurrent < 44) {
						Heater.Decrease(false)
						TeslaParameters.ChangeCurrent(+2)
					}
				}
			} else {
				// Battery is almost full so make sure we are not discharging
				if iBatt < 0.0 {
					//					fmt.Println(" - Battery is full so push the car current up")
					if carCurrent > 1 {
						if !TeslaParameters.ChangeCurrent(1) {
							//							fmt.Println("Car is maxed out so add in heaters")
							Heater.Increase(frequency)
						} else {
							// Give priority to the car if it is charging
							Heater.Decrease(false)
						}
					}
				} else if iBatt > 15 {
					//					fmt.Println("Battery is discharging more than 15 Amps so decrease heaters")
					if !Heater.Decrease(false) {
						if carCurrent > 1 {
							//							fmt.Println("Heaters off and cars are charging so drop the rate if the car current
							//							is more than 8 amps or the state of charge is below 90%")
							if (soc < 95.0) || (carCurrent > 8) {
								TeslaParameters.ChangeCurrent(-2)
							}
						}
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

	last_frequency := iValues.GetFrequency()
	last_vSetpoint := iValues.GetSetPoint()
	last_vBatt := iValues.GetVolts()
	last_iBatt := iValues.GetAmps()
	last_soc := iValues.GetSOC()
	last_iAvailable := TeslaParameters.GetMaxAmps()
	last_iUsed := TeslaParameters.GetCurrent()
	last_heaterSetting := Heater.GetSetting()
	last_heaterPump := Heater.GetPump()
	var err error
	hotTankTemp := int16(1000)

	for {
		new_frequency := iValues.GetFrequency()
		new_vSetpoint := iValues.GetSetPoint()
		new_vBatt := iValues.GetVolts()
		new_iBatt := iValues.GetAmps()
		new_soc := iValues.GetSOC()
		new_iAvailable := TeslaParameters.GetMaxAmps()
		new_iUsed := TeslaParameters.GetCurrent()
		new_heaterSetting := Heater.GetSetting()
		new_heaterPump := Heater.GetPump()

		if pDB == nil {
			pDB, err = connectToDatabase()
			if err != nil {
				glog.Errorf("Error opening the database ", err)
				glog.Flush()
				pDB = nil
				time.Sleep(time.Second)
				continue
			}
		}

		if (new_frequency != last_frequency) || (new_vSetpoint != last_vSetpoint) || (new_vBatt != last_vBatt) || (new_iBatt != last_iBatt) || (new_soc != last_soc) {
			last_frequency = new_frequency
			last_vSetpoint = new_vSetpoint
			last_vBatt = new_vBatt
			last_iBatt = new_iBatt
			last_soc = new_soc
			var _, err = pDB.Exec("call log_inverter_values(?, ?, ?, ?, ?)", new_frequency, new_vSetpoint, new_vBatt, new_iBatt, new_soc)
			if err != nil {
				glog.Errorf("Error writing inverter values to the database - %s", err)
				glog.Flush()
				_ = pDB.Close()
				pDB = nil
				time.Sleep(time.Second)
				continue
			}
		}
		if (new_iAvailable != last_iAvailable) || (new_iUsed != last_iUsed) {
			last_iAvailable = new_iAvailable
			last_iUsed = new_iUsed
			_, err := pDB.Exec("call log_tesla_values(?, ?)", new_iAvailable, new_iUsed)
			if err != nil {
				glog.Errorf("Error writing Tesla values to the database - %s", err)
				glog.Flush()
				_ = pDB.Close()
				pDB = nil
				time.Sleep(time.Second)
				continue
			}
		}
		if (new_heaterSetting != last_heaterSetting) || (new_heaterPump != last_heaterPump) {
			last_heaterSetting = new_heaterSetting
			last_heaterPump = new_heaterPump
			last_iUsed = new_iUsed
			_, err := pDB.Exec("call log_heater_values(?, ?)", new_heaterSetting, new_heaterPump)
			if err != nil {
				glog.Errorf("Error writing heater values to the database - %s", err)
				glog.Flush()
				_ = pDB.Close()
				pDB = nil
				time.Sleep(time.Second)
				continue
			}
		}
		// Get the hot tank temperature
		var err = pDB.QueryRow("select greatest(TSH0, TSH1, TSH2) as maxtemp from chillii_analogue_input where TIMESTAMP > date_add(now(), interval -5 minute) order by TIMESTAMP desc limit 1").Scan(&hotTankTemp)
		if err != nil {
			Heater.SetHotTankTemp(1000) // Be safe. If we can't get the temperature assume it is boiling to shut down the heater.
			glog.Errorf("Error fetching hot tank temperature from the database - %s", err)
			glog.Flush()
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
	var chars int
	var lastchar byte
	var linkReadyNum int
	var err interface{}

	defer func() {
		err = port.Close()
		if err != nil {
			glog.Fatal(err)
		}
	}()

	linkReadyNum = 10

	msg := twcMessage.New(port, listenMode)

	chars = 0
	lastchar = 0
	t := time.Now()

	// Start the power management loop
	go calculatePowerAvailable()

	go logToDatabase()

	for {
		if time.Since(t) > time.Second {
			if linkReadyNum > 5 {
				msg.SendMasterLinkReady1(masterAddress)
				linkReadyNum--
			} else if linkReadyNum > 0 {
				msg.SendMasterLinkReady2(masterAddress)
				linkReadyNum--
			}
			if len(slaves) > 0 {
				divideMaxAmpsAmongstSlaves(slaves, int(TeslaParameters.GetMaxAmps()*100))
				sendHearbeatsToSlaves(slaves, masterAddress)
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
				msg.AddByte(buf[0])
				chars++
				if chars == 32 {
					chars = 0
				}
				if buf[0] == 0xfe && lastchar == 0xc0 {
					chars = 0
				}
				lastchar = buf[0]
			}
			if msg.IsComplete() {
				if !msg.IsValid() {
					glog.Errorln("Invalid message received!")
					glog.Flush()
				} else {
					switch msg.GetCode() {
					//						case 0xfbe0 : fmt.Printf("To Slave %04x | Status = %02x | SetPoint = %0.2f | = %0.2f\n", msg.GetToAddress(), msg.GetStatus(), float32(msg.GetSetPoint()) / 100, float32(msg.GetCurrent()) / 100)
					case 0xfde0:
						logData(msg, &slaves)
					case 0xfde2:
						processSlaveLinkReady(msg, &slaves)
						//						case 0xfce1 : fmt.Println("Master Link Ready 1 received")
						//						case 0xfbe2 : fmt.Println("Master Link Ready 2 received")
					default:
						glog.Errorf("Unknown message code %04x\n", msg.GetCode())
						glog.Flush()
						msg.Print()
					}
				}
				msg.Reset()
			}
		}
		s := checkSlaveTimeouts(slaves)
		if s != nil {
			slaves = s
		}
		time.Sleep(100 * time.Millisecond)
	}
}
