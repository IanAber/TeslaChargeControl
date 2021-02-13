package TeslaAPI

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/PuerkitoBio/goquery"
	"golang.org/x/oauth2"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strings"
	"time"
)

const APIKeyFile = "/var/TeslaAPIKeys.txt"
const EPVehicles = "https://owner-api.teslamotors.com/api/1/vehicles"
const EPWakeUp = EPVehicles + "/%d/wake_up"
const EPStartCharging = EPVehicles + "/%d/command/charge_start"
const EPStopCharging = EPVehicles + "/%d/command/charge_stop"

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
	verbose        bool
	APIDisabled    bool
	commandHoldOff bool
}

func New(verbose bool) (*TeslaAPI, error) {
	t := new(TeslaAPI)
	t.verbose = verbose
	t.teslaToken = new(oauth2.Token)
	bytes, err := ioutil.ReadFile(APIKeyFile)
	if err == nil {
		err = json.Unmarshal(bytes, &t.teslaToken)
		if err == nil {
			if t.teslaToken.Valid() {
				//			if t.teslaToken.AccessToken != "" {
				t.teslaClient = t.oauthConfig.Client(t.ctx, t.teslaToken)
				log.Println("TeslaAPI Expires - ", t.teslaToken.Expiry)
				err = t.GetVehicleId()
				if err != nil {
					err = fmt.Errorf("TeslaAPI failed to get the Tesla vehicle ID - %s", err.Error())
				}
			} else {
				err = fmt.Errorf("AccessToken not found!--- %s", string(bytes))
			}
		}
	}
	return t, err
}

func (api *TeslaAPI) cancelCommandHoldoff() {
	api.commandHoldOff = false
	log.Println("TeslaAPI - Holdoff cancelled")
}

/**
Send email to the administrator. Change the parameters for the email server etc. here.
*/
func (api *TeslaAPI) SendMail(subject string, body string) {
	err := smtp.SendMail("mail.cedartechnology.com:587",
		smtp.PlainAuth("", "pi@cedartechnology.com", "7444561", "mail.cedartechnology.com"),
		"pi@cedartechnology.com", []string{"ian.abercrombie@cedartechnology.com"}, []byte(`From: Aberhome1
To: Ian.Abercrombie@CedarTechnology.com
Subject: `+subject+`
`+body))
	if err != nil {
		log.Println("Send email failed - ", err)
	}
}

func (api *TeslaAPI) ShowLoginPage(w http.ResponseWriter, _ *http.Request) {
	_, err := fmt.Fprint(w, `<html><head><title>Tesla API Login</title>
</head>
<body>
	<form action="/getTeslaKeys" method="POST">
		<label for="email">Email :</label><input id="email" type="text" name="email" style="width:300px" value="Tesla@CedarTechnology.com" /><br>
		<label for="password">Password :</label><input id="password" type="password" name="password" style="width:300px" value="" /><br>
		<button type="text" type="submit">Log in to Tesla</button>
	</form>
</body>
</html>`)
	if err != nil {
		log.Println(err)
	}
}

func (api *TeslaAPI) loginCompletePage() string {
	return `<html><head><title>Tesla Login Success</title></head><body><h1>Tesla Login Successful</h1><br />The Tesla API keys have been retrieved and recorded.</body></html>`
}

func randomString(len int) string {

	bytes := make([]byte, len)

	for i := 0; i < len; i++ {
		bytes[i] = byte(randInt(97, 122))
	}

	return string(bytes)
}

func randInt(min int, max int) int {

	return min + rand.Intn(max-min)
}

func Base64Encode(src []byte) string {
	return base64.RawURLEncoding.EncodeToString(src)
}

func (api *TeslaAPI) HandleTeslaLogin(w http.ResponseWriter, r *http.Request) {
	// "access_token":"qts-2a42f25867b9b18b2104c89ae57ab85f53dec8433118e97f8de8e789016e7a01"
	// "token_type":"bearer"
	// "expires_in":3888000
	// "refresh_token":"2ce3912596858fe28db4859b49a15c7b4441be61e515fe5703ca95f61090f4ce"
	// "created_at":1606874992

	//var token struct {
	//	AccessToken  string `json:"access_token"`
	//	TokenType    string `json:"bearer"`
	//	ExpiresIn    int64  `json:"expires_in"`
	//	RefreshToken string `json:"refresh_token"`
	//	CreatedAt    int64  `json:"created_at"`
	//}
	var token oauth2.Token

	loginFailureMessage := func(w http.ResponseWriter, err error) {
		_, err = fmt.Fprint(w, `<html><head><title>Tesla API Login Failure</title><head><body>`, err, `</body></html>`)
		if err != nil {
			log.Println(err)
		}
	}
	err := r.ParseForm()
	if err != nil {
		log.Println(err)
	}
	email := r.Form["email"][0]
	password := r.Form["password"][0]
	if api.verbose {
		fmt.Println("Email = ", email)
		fmt.Println("Password = ", password)
	}

	rand.Seed(time.Now().UTC().UnixNano())
	codeVerifier := randomString(86)
	hash := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := Base64Encode(hash[:])
	state := randomString(10)
	strUri := fmt.Sprintf("https://auth.tesla.com/oauth2/v3/authorize?client_id=ownerapi&code_challenge=%s&code_challenge_method=S256&redirect_uri=https://auth.tesla.com/void/callback&response_type=code&scope=openid%%20emai%%20offline_access&state=%s&login_hint=tesla@cedartechnology.com",
		codeChallenge, state)
	getResponse, err := http.Get(strUri)
	defer func() {
		err := getResponse.Body.Close()
		if err != nil {
			log.Println(err)
		}
	}()
	if err != nil {
		loginFailureMessage(w, err)
		return
	}
	if getResponse.StatusCode != 200 {
		body, err2 := ioutil.ReadAll(getResponse.Body)
		if err2 != nil {
			loginFailureMessage(w, err2)
			return
		}

		loginFailureMessage(w, fmt.Errorf("Tesla login page returned %s\n", string(body)))
		return
	}

	var sessionCookie *http.Cookie
	for _, cookie := range getResponse.Cookies() {
		log.Println("Cookie - ", cookie.Name, " : ", cookie.Value)
		if cookie.Name == "tesla-auth.sid" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		loginFailureMessage(w, fmt.Errorf("TeslaAPI The session cookie was missing"))
		return
	} else {
		log.Println("Cookie = ", sessionCookie)
	}

	strUri = fmt.Sprintf("https://auth.tesla.com/oauth2/v3/authorize?client_id=ownerapi&code_challenge=%s&code_challenge_method=S256&"+
		"redirect_uri=https://auth.tesla.com/void/callback&"+
		"response_type=code&"+
		"scope=openid%%20email%%20offline_access&"+
		"state=%s",
		codeChallenge, state)
	form := url.Values{"identity": {email},
		"credential": {password}}
	// Load the HTML document
	doc, err := goquery.NewDocumentFromReader(getResponse.Body)

	if err != nil {
		loginFailureMessage(w, err)
		return
	}

	doc.Find("input").Each(func(i int, s *goquery.Selection) {
		// For each item found, get the band and title

		inputName, foundName := s.Attr("name")
		inputValue, _ := s.Attr("value")
		inputType, foundType := s.Attr("type")
		if foundType && foundName && (inputType == "hidden") {
			form.Add(inputName, inputValue)
		}
	})

	//	http.SetCookie(w, sessionCookie)
	log.Println("Sending login form.")

	request, err := http.NewRequest("POST", strUri, strings.NewReader(form.Encode()))
	if err != nil {
		loginFailureMessage(w, err)
		return
	}
	request.AddCookie(sessionCookie)
	request.Header.Add("content-type", "application/x-www-form-urlencoded")

	// Need to do a RoundTrip so we don't automatically redirect. We should get 302 status
	response, err := http.DefaultTransport.RoundTrip(request)
	defer func() {
		err := response.Body.Close()
		if err != nil {
			loginFailureMessage(w, err)
			return
		}
	}()

	if err != nil {
		loginFailureMessage(w, err)
		return
	}

	if response.StatusCode != 302 {
		loginFailureMessage(w, fmt.Errorf(" Tesla login returned %d - %s", response.StatusCode, response.Status))
		return
	}

	locationUri, err := response.Location()
	if err != nil {
		loginFailureMessage(w, err)
		return
	}

	authorisationCode := locationUri.Query().Get("code")
	strUri = "https://auth.tesla.com/oauth2/v3/token"
	var requestParams struct {
		GrantType    string `json:"grant_type"`
		ClientID     string `json:"client_id"`
		Code         string `json:"code"`
		CodeVerifier string `json:"code_verifier"`
		RedirectUri  string `json:"redirect_uri"`
	}

	requestParams.GrantType = "authorization_code"
	requestParams.ClientID = "ownerapi"
	requestParams.Code = authorisationCode
	requestParams.CodeVerifier = codeVerifier
	requestParams.RedirectUri = "https://auth.tesla.com/void/callback"

	requestBody, err := json.Marshal(requestParams)
	if err != nil {
		loginFailureMessage(w, err)
		return
	}

	response, err = http.Post(strUri, "application/json", strings.NewReader(string(requestBody)))
	if err != nil {
		loginFailureMessage(w, err)
		return
	}
	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		loginFailureMessage(w, err)
		return
	}

	//	log.Println("TeslaAPI response. Unmarshalling... ", string(body))
	err = json.Unmarshal(body, &token)
	if err != nil {
		loginFailureMessage(w, err)
		return
	}
	api.teslaToken.AccessToken = token.AccessToken
	api.teslaToken.RefreshToken = token.RefreshToken
	api.teslaToken.TokenType = token.TokenType
	api.teslaToken.Expiry = token.Expiry

	newToken, err2 := json.Marshal(api.teslaToken)
	if err2 != nil {
		log.Println(err2)
	} else {
		err = ioutil.WriteFile(APIKeyFile, newToken, os.ModePerm)
		if err != nil {
			log.Println(err)
		} else {
			log.Println("TeslaAPI wrote to key file : ", string(newToken))
		}
	}
	api.teslaClient = api.oauthConfig.Client(api.ctx, api.teslaToken)
	err = api.GetVehicleId()
	if err != nil {
		_, err = fmt.Fprint(w, `<html><head><title>Tesla Credentials Updated</title></head><body><h1>The Tesla API credentials have been updated.</h1><br>Error retrieving the Vehicle ID : `, err.Error(), `</body></html>`)
	} else {
		_, err = fmt.Fprint(w, `<html><head><title>Tesla Credentials Updated</title></head><body><h1>The Tesla API credentials have been updated.</h1><br>Vehicle ID = `, api.vId, `</body></html>`)
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
		if err != nil {
			log.Println(err)
		} else {
			if api.verbose {
				fmt.Println(string(body))
			}
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

func (api *TeslaAPI) GetVehicleId() error {
	var vehicles teslaVehicles

	api.vId = 0
	if api.teslaClient == nil {
		if api.verbose {
			fmt.Println("Waiting...")
		}
		return fmt.Errorf("TeslaAPI is not configured")
	} else {
		if api.verbose {
			fmt.Println("Getting vehicle ID")
		}
		response, err := api.teslaClient.Get(EPVehicles)
		if err != nil {
			return err
		} else {
			defer func() {
				err := response.Body.Close()
				if err != nil {
					log.Println(err)
				}
			}()
			if api.verbose {
				fmt.Println("Tesla api status = ", response.Status)
			}
			body, err := ioutil.ReadAll(response.Body)
			if err != nil {
				return err
			} else {
				if api.verbose {
					fmt.Println("Response = ", string(body))
				}
				if response.StatusCode == 401 {
					return fmt.Errorf("TeslaAPI returned unauthorised. Token = %s", api.teslaToken.AccessToken)
				}
				log.Println("TeslaAPI - Get Vehicles returned status : ", response.StatusCode, " : ", response.Status)
				err = json.Unmarshal(body, &vehicles)
				if err != nil {
					return err
				} else {
					if api.verbose {
						fmt.Println("Vehicle = ", vehicles.Response[0].DisplayName)
					}
					api.vId = vehicles.Response[0].Id
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
	sCommand := fmt.Sprintf(EPWakeUp, api.vId)
	if api.verbose {
		fmt.Println(sCommand)
	}
	for timeout := 0; timeout < 10; timeout++ {
		log.Println("TeslaAPI - Sending [", sCommand, "]")
		body, err := api.postCarCommand(sCommand)
		if err != nil {
			log.Println("TeslaAPI - Wakeup Car API Error - ", err)
			api.SendMail("Wake Car Error sending API call", err.Error())
		}
		if len(body) > 0 {
			if string(body) == "You have been temporarily blocked for making too many requests!" {
				// We need to hold off for at least 15 minutes
				log.Println("TeslaAPI - We have made too many API calls so we are being temporarily blocked by Tesla.")
				api.SendMail("Tesla API Blocked!", "The Tesla API has been blocked due to too many requests in a short time.")
				api.APIDisabled = true
				time.AfterFunc(time.Minute*15, func() { api.APIDisabled = false })
			} else {
				err = json.Unmarshal(body, &wakeResponse)
				if err != nil {
					log.Println("TeslaAPI - Wakeup Car Error reading response", err, " [", string(body), "]")
					api.SendMail("Wake Car Error reading response", err.Error()+" - "+string(body))
				}
			}
		}
		if err == nil {
			if wakeResponse.Response.State == "online" {
				if api.verbose {
					fmt.Println("Car is awake.")
				}
				log.Println("TeslaAPI - Zoe is awake.")
				// Record when this command was sent. We do not want to send commands too often.
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

func (api *TeslaAPI) StartCharging() {

	if api.verbose {
		fmt.Println("Starting to charge...")
	}
	if api.teslaClient == nil {
		log.Println("StartCharging - TeslaAPI is not configured")
		return
	}
	if api.APIDisabled {
		return
	}

	if api.commandHoldOff {
		log.Println("It has been less than a minute since the last command was sent to Tesla. Ignoring request.")
		return
	}

	api.commandHoldOff = true
	time.AfterFunc(time.Minute*2, api.cancelCommandHoldoff)

	if !api.WakeCar() {
		log.Println("TeslaAPI Failed to wake up the car before sending the start charging command")
		return
	}
	sCommand := fmt.Sprintf(EPStartCharging, api.vId)
	if api.verbose {
		fmt.Println(sCommand)
	}
	body, err := api.postCarCommand(sCommand)
	var teslaResponse TeslaResponse
	err = json.Unmarshal(body, &teslaResponse)
	if err != nil {
		log.Println(err)
		return
	} else {
		if !teslaResponse.Response.Result {
			log.Printf("TeslaAPI Failed! - %s\n", teslaResponse.Response.Reason)
			return
		}
	}

	api.SendMail("Tesla Charging Started", "Started charging the Tesla.")
	return
}

func (api *TeslaAPI) StopCharging() {
	if api.verbose {
		fmt.Println("Stopping charging")
	}
	if api.teslaClient == nil {
		log.Println("StopCharging - TeslaAPI is not configured")
		return
	}
	if api.APIDisabled {
		return
	}
	if api.commandHoldOff {
		log.Println("TeslaAPI - Holdoff")
		return
	}
	// Prevent spamming by holding off on another command for 2 minutes
	api.commandHoldOff = true
	time.AfterFunc(time.Minute*2, api.cancelCommandHoldoff)

	log.Println("TeslaAPI - Waking up Zoe")
	if !api.WakeCar() {
		api.SendMail("Tesla Charging Failed!", "Failed to stop the car from charging!!! HELP!!!!!")
		log.Println("Failed to wake up the car before sending the stop charging command.")
	}
	sCommand := fmt.Sprintf(EPStopCharging, api.vId)
	if api.verbose {
		fmt.Println(sCommand)
	}
	log.Println("TeslaAPI - Sending STOP - [", sCommand, "]")
	body, err := api.postCarCommand(sCommand)

	var teslaResponse TeslaResponse
	err = json.Unmarshal(body, &teslaResponse)
	if err != nil {
		log.Println("TeslaAPI - Response from Tesla could not be unmarshalled. - ", err, " - ", string(body))
		api.SendMail("Response from Tesla could not be unmarshalled.", err.Error()+" - "+string(body))
		return
	} else {
		if !teslaResponse.Response.Result {
			log.Println("TeslAPI - Failed - [", teslaResponse.Response.Reason, "]")
			api.SendMail("Response.Result from Tesla was FALSE", "Reason = "+teslaResponse.Response.Reason+" - "+string(body))
			log.Printf("Failed! - %s\n", teslaResponse.Response.Reason)
			return
		}
	}
	api.SendMail("Tesla Charging Stopped", "Stopped charging the Tesla.")
	return
}