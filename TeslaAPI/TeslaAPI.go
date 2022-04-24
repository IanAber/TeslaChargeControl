package TeslaAPI

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/oauth2"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/smtp"
	"os"
	"time"
)

//const APIAUTHSCHEME = "https"
//const APIAUTHHOST = "auth.tesla.com"

//const APIAUTHROOT = APIAUTHSCHEME + "://" + APIAUTHHOST

//const APIAUTHCAPTCHAPATH = "/captcha"

//const APIAUTHCAPTCHAURL = APIAUTHROOT + APIAUTHCAPTCHAPATH

const APISCHEME = "https"
const APIHOST = "owner-api.teslamotors.com"
const APIROOT = APISCHEME + "://" + APIHOST

const APIEPVEHICLESPATH = "/api/1/vehicles"

const APIEPVEHICLESURL = APIROOT + APIEPVEHICLESPATH
const APIEPWAKEUP = APIEPVEHICLESURL + "/%d/wake_up"
const APIEPSTARTCHARGING = APIEPVEHICLESURL + "/%d/command/charge_start"
const APIEPSTOPCHARGING = APIEPVEHICLESURL + "/%d/command/charge_stop"

const CHARGINGLINKS = `<a href="/startCharging">Start Charging</a><br><a href="/stopCharging">Stop Charging</a>`

const APIKEYFILE = "/var/TeslaAPIKeys.txt"

type TokenExchangeParams struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

type TokenRequestParams struct {
	GrantType    string `json:"grant_type"`
	ClientID     string `json:"client_id"`
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	RedirectUri  string `json:"redirect_uri"`
	Scope        string `json:"scope"`
}

type TokenRead struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	State        string `json:"state"`
	TokenType    string `json:"token_type"`
	CreatedAt    int64  `json:"created_at"`
}

type vehicle struct {
	Id          uint64 `json:"id"`
	VehicleId   uint64 `json:"vehicle_id"`
	Vin         string `json:"vin"`
	DisplayName string `json:"display_name"`
	OptionCodes string `json:"option_codes"`
	Color       string `json:"color"`
}

type teslaVehicles struct {
	Response []vehicle `json:"response"`
	Count    int       `json:"count"`
}

type TeslaAPI struct {
	teslaToken     *oauth2.Token
	ctx            context.Context
	teslaClient    *http.Client
	oauthConfig    *oauth2.Config
	vId            uint64
	APIDisabled    bool
	commandHoldOff bool
	lastEmail      time.Time
	client         http.Client
	code           string
}

func New() (*TeslaAPI, error) {
	t := new(TeslaAPI)
	t.teslaToken = new(oauth2.Token)
	t.ctx = context.Background()
	bytes, err := ioutil.ReadFile(APIKEYFILE)
	t.lastEmail = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	jar, _ := cookiejar.New(nil)
	t.client = http.Client{Jar: jar, Transport: &http.Transport{}}
	t.oauthConfig = &oauth2.Config{
		ClientID:     "ownerapi",
		ClientSecret: "some secret",
		Endpoint: oauth2.Endpoint{
			AuthURL:   "https://auth.tesla.com/oauth2/v3/authorize",
			TokenURL:  "https://auth.tesla.com/oauth2/v3/token",
			AuthStyle: 0,
		},
		RedirectURL: "https://auth.tesla.com/void/callback",
		Scopes:      []string{"openid", "email", "offline_access"},
	}
	//	fmt.Println("Setting up the Tesla API client.")
	if err == nil {
		err = json.Unmarshal(bytes, &t.teslaToken)
		if err == nil {

			if t.teslaToken.AccessToken != "" {
				t.teslaToken.Expiry = time.Now()
				//			if t.teslaToken.Valid() {
				t.teslaClient = t.oauthConfig.Client(t.ctx, t.teslaToken)
				log.Println("Getting the vehicle ID")
				err = t.GetVehicleId()
				if err != nil {
					err = fmt.Errorf("TeslaAPI failed to get the Tesla vehicle ID - %s", err.Error())
					log.Println(err.Error())
				} else {
					log.Println("TeslaAPI Expires - ", t.teslaToken.Expiry)
				}

			} else {
				err = fmt.Errorf("AccessToken not found!--- %s", string(bytes))
				log.Println(err.Error())
			}
		}
	} else {
		log.Println("Error reading the API Key File - ", err.Error())
	}
	return t, err
}

func (api *TeslaAPI) cancelCommandHoldoff() {
	api.commandHoldOff = false
	log.Println("TeslaAPI - Holdoff cancelled")
}

// SendMail
// Send email to the administrator. Change the parameters for the email server etc. here.
func (api *TeslaAPI) SendMail(subject string, body string) error {
	//	fmt.Println("Sending mail to Ian : subject = ", subject, "\nbody : ", body)
	err := smtp.SendMail("smtp.titan.email:587",
		smtp.PlainAuth("", "pi@cedartechnology.com", "7444561", "smtp.titan.email"),
		"pi@cedartechnology.com", []string{"ian.abercrombie@cedartechnology.com"}, []byte(`From: Aberhome1
To: Ian.Abercrombie@cedartechnology.com
Subject: `+subject+`
`+body))
	return err
}

func (api *TeslaAPI) ShowGetTokensPage(w http.ResponseWriter, _ *http.Request) {
	_, err := fmt.Fprint(w, `<html><head><title>Tesla API Keys</title>
	</head>
	<body>
		<form action="/setTeslaKeys" method="POST">
			<label for="refresh_token">Refresh Token :</label>
			<input id="refresh_token" name="refresh_token" style="width:800px" value="" /><br>
			<label for="access_token">Access Token :</label>
			<input id="access_token" type="text" name="access_token" style="width:800px" value="" /><br>
			<button type="text" type="submit">log in to Tesla</button>
		</form>
	</body>
	</html>`)
	if err != nil {
		log.Println(err)
	}
}

func (api *TeslaAPI) loginCompletePage() string {
	return `<html><head><title>Tesla keys - Success</title></head><body><h1>Tesla login Successful</h1><br />The Tesla API keys have been retrieved and recorded.</body></html>`
}

/*
Sample token file
{
        "access_token":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6Ilg0RmNua0RCUVBUTnBrZTZiMnNuRi04YmdVUSJ9.eyJpc3MiOiJodHRwczovL2F1dGgudGVzbGEuY29tL29hdXRoMi92MyIsImF1ZCI6WyJodHRwczovL293bmVyLWFwaS50ZXNsYW1vdG9ycy5jb20vIiwiaHR0cHM6Ly9hdXRoLnRlc2xhLmNvbS9vYXV0aDIvdjMvdXNlcmluZm8iXSwiYXpwIjoib3duZXJhcGkiLCJzdWIiOiI2MjUwNDg3MS0yYzljLTQzNmEtOTg1Ni00YjJlNzVmZDI1M2EiLCJzY3AiOlsib3BlbmlkIiwiZW1haWwiLCJvZmZsaW5lX2FjY2VzcyJdLCJhbXIiOlsicHdkIl0sImV4cCI6MTYzNjM1MzA5NiwiaWF0IjoxNjM2MzI0Mjk2LCJhdXRoX3RpbWUiOjE2MzYzMjQyOTZ9.g36OJXdkFPHmsCDZwPiUINDA4jkfAt-WU4JyzlGhp0kJwjubvnXAI9IByjdtsCWjSGDG_JSkMMTsPbfKdXC15dkK5CtkUAMFBv3iMXQXuh-1rVZzYXRjgviHCUhkFQoAwgfcWGi1rcKs8l9B03CA82AovJUQRpUH9_827rLjaQuBMLHHsRZ3Gc3NrCr_TKnwxotXRqgm4cPY4vHe0UvZ7ePb8EY7JxVnHWhs5Eos743dahLyWl-ew2_JLgmzPVrLcl330_Ynt27oeRDVMXscd4pRVaS5uHAQCd3pQE9oidCRHpcgthrDAYcupQlNTMd-ftSnsTQYE9NTsNTjAQuypnvfM2cf-Lv0RVlMqN2Qe99CtrbALm8gFDX_UNK123ecEK0jjXFzMAbTGUCIJmzVleSRC_aJHjXb9Brsa_tCEpFHINJENg1sreLNFDdtKYPU1w49ycwtZWwr-v9N5Fz5g62-s-zS-ato6Mjewz0qLg9Uac-A0h-bXeVghPCYS-Wg3TpwRVoYtPSTRreF-C1K5Nd26TKW53fNnFWn5hGOp3BeA4hfDL8uuu34TNtqCnwXPc9Io-x1Uhh8wGWtCHklO6_gBp_QNWIBi-VN8HUTJ7bB5Osg0srhdbtcVAh6Pf2ekFXirfsTx8cM0MusM2TREnGdkgLPO8sPkVV8dBzzvuU",
		"id_token":"",
        "token_type":"bearer",
        "refresh_token":"eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCIsImtpZCI6Ilg0RmNua0RCUVBUTnBrZTZiMnNuRi04YmdVUSJ9.eyJpc3MiOiJodHRwczovL2F1dGgudGVzbGEuY29tL29hdXRoMi92MyIsImF1ZCI6Imh0dHBzOi8vYXV0aC50ZXNsYS5jb20vb2F1dGgyL3YzL3Rva2VuIiwiaWF0IjoxNjM2MzI0Mjk2LCJzY3AiOlsib3BlbmlkIiwib2ZmbGluZV9hY2Nlc3MiXSwiZGF0YSI6eyJ2IjoiMSIsImF1ZCI6Imh0dHBzOi8vb3duZXItYXBpLnRlc2xhbW90b3JzLmNvbS8iLCJzdWIiOiI2MjUwNDg3MS0yYzljLTQzNmEtOTg1Ni00YjJlNzVmZDI1M2EiLCJzY3AiOlsib3BlbmlkIiwiZW1haWwiLCJvZmZsaW5lX2FjY2VzcyJdLCJhenAiOiJvd25lcmFwaSIsImFtciI6WyJwd2QiXSwiYXV0aF90aW1lIjoxNjM2MzI0Mjk2fX0.lZUY07bmC8zD5rR0hI4uXar_9yuoHjWHuvs5IMw7yQPxeuztqwa8CKBpOlYphZCX9PmZhIkMJgiLZrfej2a78uYq_cbapBexYiadvbmg-NDK1Xcp_EWahsYBlx9LFxd7QTxOsEd1NEeR2GZf4yoLaTBzcOj7cQWXsQhH_Mfvt_AxE3IWIp_hwWLPpDr8UcFvTh_l5sNNRjrbLnuMIT81wuAyfQ0IPDzphxVvxyytVV3dq64X4EYFGnPRRNVn0iUhteyWo_PQHutBLoJrV5jXd4IOYHWEM-KQ5ZPG10TrsuEZY6ZqggPy6SE4rhLF6vjcIr8vaXgHy3oXA25lHZGbKzsG9H3gfyqcZldQgDvV9DzNFrXDNDQ0rsBbMdpzDUL4rHMDXBQf4bnEKb0LH4U02lI3wfibSUUIpNgDC5pQSDXrlPcxIorT9dzTtYQBKepBVRzVPkmyfuiNoXgLKEM4cNjo4uN5C69rbAUPvpDwjTkVWeclYM8AeuXsLll4Z1vo6-2pmLV0REAh6CZvbEJ0iAAoaSONZZiKX70FU2tVBjKGJkHehWDne3FF58m8QE8eVJOLQ6whFBreTVvt1MQUvIcOjuG9S7ydDvnTHhBfxSB-0Y5AproPmCZuknrIfhmryanbECUer3jX9oJ9MFArJFMoZQYDG8XsvU9EBL6zK0I",
        "expiry":"2021-11-08T00:00:00-05:00"
}
*/

// HandleSetTeslaTokens - Called when /setTeslaKeys is requested. Access and Refresh keys are provided in the form.
func (api *TeslaAPI) HandleSetTeslaTokens(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm() // Parse the form to get the keys
	if err != nil {
		log.Println(err)
		_, err := fmt.Fprint(w, "<html><title>Failed to get keys</title><head></head><body><h1>Failed to get the tokens</h1>", err.Error(), "</body></html>")
		if err != nil {
			log.Println(err)
		}
	}

	api.teslaToken.AccessToken = r.Form["access_token"][0]
	api.teslaToken.RefreshToken = r.Form["refresh_token"][0]
	api.teslaToken.TokenType = "bearer"
	api.teslaToken.Expiry = time.Now().Add(time.Hour * 8)
	//	api.teslaToken.IDToken = ""

	newToken, err2 := json.Marshal(api.teslaToken)
	if err2 != nil {
		fmt.Println(err2)
	} else {
		err = ioutil.WriteFile(APIKEYFILE, newToken, os.ModePerm)
		if err != nil {
			log.Println(err)
		}
	}

	api.teslaClient = api.oauthConfig.Client(api.ctx, api.teslaToken)

	err = api.GetVehicleId()
	if err != nil {
		_, err = fmt.Fprint(w, `<html><head><title>Tesla Credentials Updated</title></head><body><h1>The Tesla API credentials have been updated.</h1><br>Error retrieving the Vehicle ID : `, err.Error(), `</body></html>`)
	} else {
		_, err = fmt.Fprint(w, `<html><head><title>Tesla Credentials Updated</title></head><body><h1>The Tesla API credentials have been updated.</h1><br>Vehicle ID = `, api.vId, `<br />`, CHARGINGLINKS, `</body></html>`)
	}
	if err != nil {
		log.Println(err)
	}
}

func (api *TeslaAPI) postCarCommand(sCommand string) ([]byte, error) {
	response, err := api.teslaClient.Post(sCommand, "application/json", nil)
	if err != nil {
		log.Println(err)
	} else {
		defer func() {
			err := response.Body.Close()
			if err != nil {
				log.Println(err)
			}
		}()
		if response.StatusCode != 200 {
			return nil, fmt.Errorf("postCarCommand recieved %d - %s", response.StatusCode, response.Status)
		}
		body, err := ioutil.ReadAll(response.Body)
		errorClose := response.Body.Close()
		if errorClose != nil {
			log.Println(errorClose)
		}

		if err != nil {
			log.Println(err)
		} else {
			return body, nil
		}
	}
	return nil, err
}

type TeslaResponse struct {
	Response struct {
		Reason string `json:"reason"`
		Result bool   `json:"result"`
	} `json:"response"`
}

func (api *TeslaAPI) IsConfigured() bool {
	return api.teslaClient != nil
}

func (api *TeslaAPI) GetVehicleId() error {
	var vehicles teslaVehicles

	api.vId = 0
	log.Println("getting Tesla vehicle ID")
	if api.teslaClient == nil {
		if api.lastEmail.Before(time.Now().Add(0 - time.Hour)) {
			errorMail := api.SendMail("Tesla API Not Configured", "A call was made to GetVehicleId() but the Tesla API is not configured.")
			if errorMail != nil {
				log.Println(errorMail)
			}
			api.lastEmail = time.Now()
		}
		return fmt.Errorf("TeslaAPI is not configured")
	} else {
		response, err := api.teslaClient.Get(APIEPVEHICLESURL)
		if err != nil {
			log.Println(err)
			return err
		} else {
			defer func() {
				err := response.Body.Close()
				if err != nil {
					log.Println(err)
				}
			}()
			body, err := ioutil.ReadAll(response.Body)
			//			errorClose := response.Body.Close()
			//			if errorClose != nil {
			//				log.Println(errorClose)
			//			}
			if err != nil {
				return err
			} else {
				if response.StatusCode == 401 {
					log.Println("TeslaAPI returned unauthorised.")
					return fmt.Errorf("TeslaAPI returned unauthorised. Token = %s", api.teslaToken.AccessToken)
				}
				if response.StatusCode == 404 {
					return fmt.Errorf("TeslaAPI returned %s when looking for vehicle IDs", response.Status)
				}
				log.Println("TeslaAPI - Get Vehicles returned status : ", response.StatusCode, " : ", response.Status)
				err = json.Unmarshal(body, &vehicles)
				if err != nil {
					return err
				} else {
					api.vId = vehicles.Response[0].Id
					newToken, err2 := json.Marshal(api.teslaToken)
					if err2 != nil {
						log.Println(err2)
					} else {
						err = ioutil.WriteFile(APIKEYFILE, newToken, os.ModePerm)
						if err != nil {
							log.Println(err)
						} else {
							log.Println("Tesla key file rewritten.")
						}
					}
				}
			}
		}
	}
	return nil
}

func (api *TeslaAPI) WakeCar() bool {
	if api.teslaClient == nil {
		return false
	}
	var wakeResponse struct {
		Response struct {
			Id              uint64   `json:"id"`
			UserId          uint64   `json:"user_id"`
			VehicleId       uint64   `json:"vehicle_id"`
			Vin             string   `json:"vin"`
			DisplayName     string   `json:"display_name"`
			OptionCodes     string   `json:"option_codes"`
			Color           string   `json:"color"`
			Tokens          []string `json:"tokens"`
			State           string   `json:"state"`
			InService       bool     `json:"in_service"`
			Ids             string   `json:"ids"`
			CalendarEnabled bool     `json:"calendar_enabled"`
			ApiVersion      int      `json:"api_version"`
		}
	}
	if api.vId == 0 {
		err := api.GetVehicleId()
		if err != nil {
			log.Println(err)
		}
	}
	sCommand := fmt.Sprintf(APIEPWAKEUP, api.vId)
	for timeout := 0; timeout < 10; timeout++ {
		log.Println("TeslaAPI - Sending [", sCommand, "]")
		body, err := api.postCarCommand(sCommand)
		if err != nil {
			log.Println("TeslaAPI - Wakeup Car API Error - ", err)
			errMail := api.SendMail("Wake Car Error sending API call", err.Error())
			if errMail != nil {
				log.Println(errMail)
			}
		}
		if len(body) > 0 {
			if string(body) == "You have been temporarily blocked for making too many requests!" {
				// We need to hold off for at least 15 minutes
				log.Println("TeslaAPI - We have made too many API calls so we are being temporarily blocked by Tesla.")
				errMail := api.SendMail("Tesla API Blocked!", "The Tesla API has been blocked due to too many requests in a short time.")
				if errMail != nil {
					log.Println(errMail)
				}
				api.APIDisabled = true
				time.AfterFunc(time.Minute*15, func() { api.APIDisabled = false })
			} else {
				err = json.Unmarshal(body, &wakeResponse)
				if err != nil {
					log.Println("TeslaAPI - Wakeup Car Error reading response", err, " [", string(body), "]")
					errMail := api.SendMail("Wake Car Error reading response", err.Error()+" - "+string(body))
					if errMail != nil {
						log.Println(errMail)
					}
				}
			}
		}
		if err == nil {
			if wakeResponse.Response.State == "online" {
				log.Println("TeslaAPI - Zoe is awake.")
				time.Sleep(time.Second * 15)
				// Give it 15 seconds befoore issuing the next command.
				return true
			}
			log.Println("TeslaAPI - Waiting for Zoe to wake up. State = [", wakeResponse.Response.State, "]")
			// Wait 10 seconds before trying again if it is not awake
			time.Sleep(time.Second * 10)
		}
	}
	// Give up!
	log.Println("TeslaAPI - Timed out waiting for Zoe to wake up.")
	return false
}

func (api *TeslaAPI) IsHoldoff() bool {
	return api.commandHoldOff
}

func (api *TeslaAPI) StartCharging() error {

	if api.teslaClient == nil {
		if api.lastEmail.Before(time.Now().Add(0 - time.Hour)) {
			errorMail := api.SendMail("Tesla API not configured", "A call was made to StartCharging but the Tesla API is not configured.")
			if errorMail != nil {
				log.Println(errorMail)
			}
			api.lastEmail = time.Now()
		}
		return errors.New("StartCharging - TeslaAPI is not configured")
	}
	if api.APIDisabled {
		return errors.New("API is disabled")
	}

	if api.commandHoldOff {
		log.Println("It has been less than a minute since the last command was sent to Tesla. Ignoring request.")
		return nil
	}

	api.commandHoldOff = true
	time.AfterFunc(time.Minute, api.cancelCommandHoldoff)

	if !api.WakeCar() {
		return errors.New("TeslaAPI Failed to wake up the car before sending the start charging command")
	}
	sCommand := fmt.Sprintf(APIEPSTARTCHARGING, api.vId)
	body, err := api.postCarCommand(sCommand)
	var teslaResponse TeslaResponse
	err = json.Unmarshal(body, &teslaResponse)
	if err != nil {
		return errors.New(err.Error())
	} else {
		if !teslaResponse.Response.Result {
			return errors.New("TeslaAPI Failed! - " + teslaResponse.Response.Reason)
		}
	}

	errorMail := api.SendMail("Tesla Charging Started", "Started charging the Tesla.")
	if errorMail != nil {
		log.Println(errorMail)
	}
	return nil
}

func (api *TeslaAPI) StopCharging() error {
	if api.teslaClient == nil {
		errMail := api.SendMail("Tesla API Not Configured", "A call was made to StopCharging but the Tesla API is not configured.")
		if errMail != nil {
			log.Println(errMail)
		}
		return errors.New("StopCharging - TeslaAPI is not configured")
	}
	if api.APIDisabled {
		return errors.New(" Tesla API is disabled")
	}
	if api.commandHoldOff {
		log.Println("TeslaAPI - Holdoff")
		return nil
	}
	// Prevent spamming by holding off on another command for 2 minutes
	api.commandHoldOff = true
	time.AfterFunc(time.Minute, api.cancelCommandHoldoff)

	log.Println("TeslaAPI - Waking up Zoe")
	if !api.WakeCar() {
		errMail := api.SendMail("Tesla Charging Failed!", "Failed to stop the car from charging!!! HELP!!!!!")
		if errMail != nil {
			log.Println(errMail)
		}
		//		api.commandHoldOff = false;
		return errors.New("failed to wake up the car before sending the stop charging command")
	}
	sCommand := fmt.Sprintf(APIEPSTOPCHARGING, api.vId)
	log.Println("TeslaAPI - Sending STOP - [", sCommand, "]")
	body, err := api.postCarCommand(sCommand)

	var teslaResponse TeslaResponse
	err = json.Unmarshal(body, &teslaResponse)
	if err != nil {
		log.Println("TeslaAPI - Response from Tesla could not be unmarshalled. - ", err, " - ", string(body))
		errMail := api.SendMail("Response from Tesla could not be unmarshalled.", err.Error()+" - "+string(body))
		if errMail != nil {
			log.Println(errMail)
		}
		return errors.New(" TeslaAPI - Response from Tesla could not be unmarshalled.")
	} else {
		if !teslaResponse.Response.Result {
			errorMail := api.SendMail("Response.Result from Tesla was FALSE", "Reason = "+teslaResponse.Response.Reason+" - "+string(body))
			if errorMail != nil {
				log.Println(errorMail)
			}
			api.commandHoldOff = false
			return errors.New("TeslAPI - Failed - [" + teslaResponse.Response.Reason + "]")
		}
	}
	errorMail := api.SendMail("Tesla Charging Stopped", "Stopped charging the Tesla.")
	if errorMail != nil {
		log.Println(errorMail)
	}
	return nil
}
