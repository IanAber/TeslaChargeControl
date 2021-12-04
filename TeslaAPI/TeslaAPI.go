package TeslaAPI

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/smtp"
	"net/url"
	"os"
	"time"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/oauth2"
)

//const APICLIENTID = "81527cff06843c8634fdc09e8ac0abefb46ac849f38fe1e431c2ef2106796384"
//const APICLIENTSECRET = "c7257eb71a564034f9419ee651c7d0e5f7aa6bfbd18bafb5c5c033b093bb2fa3"
const APIAUTHSCHEME = "https"
const APIAUTHHOST = "auth.tesla.com"
const APIAUTHROOT = APIAUTHSCHEME + "://" + APIAUTHHOST

//const APIAUTHCLIENTREDIRECTPATH = "/void/callback"
//const APIAUTHORIZEPATH = "/oauth2/v3/authorize"
const APIAUTHCAPTCHAPATH = "/captcha"

//const APIMULTIFACTORPATH = "/oauth2/v3/authorize/mfa/factors"
//const APIAUTHEXCHANGEPATH = "/oauth2/v3/token"

//const APIAUTHCLIENTREDIRECTURL = APIAUTHROOT + APIAUTHCLIENTREDIRECTPATH
//const APIAUTHORIZEURL = APIAUTHROOT + APIAUTHORIZEPATH
const APIAUTHCAPTCHAURL = APIAUTHROOT + APIAUTHCAPTCHAPATH

//const APIMULTIFACTORURL = APIAUTHROOT + APIMULTIFACTORPATH
//const APIAUTHEXCHANGEURL = APIAUTHROOT + APIAUTHEXCHANGEPATH
//const APIAUTHSCOPE = "openid email offline_access"

const APISCHEME = "https"
const APIHOST = "owner-api.teslamotors.com"
const APIROOT = APISCHEME + "://" + APIHOST

//const APITOKENPATH = "/oauth/token"
const APIEPVEHICLESPATH = "/api/1/vehicles"

//const APITOKENURL = APIROOT + APITOKENPATH
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
	bytes, err := ioutil.ReadFile(APIKEYFILE)
	t.lastEmail = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	jar, _ := cookiejar.New(nil)
	t.client = http.Client{Jar: jar, Transport: &http.Transport{}}

	fmt.Println("Setting up the Tesla API client.")
	if err == nil {
		err = json.Unmarshal(bytes, &t.teslaToken)
		if err == nil {
			//			if t.teslaToken.AccessToken != "" {
			if t.teslaToken.Valid() {
				t.teslaClient = t.oauthConfig.Client(t.ctx, t.teslaToken)
				t.oauthConfig.Client(t.ctx, t.teslaToken)
				fmt.Println("Getting the vehicle ID")
				err = t.GetVehicleId()
				if err != nil {
					err = fmt.Errorf("TeslaAPI failed to get the Tesla vehicle ID - %s", err.Error())
					fmt.Println(err.Error())
				} else {
					fmt.Println("TeslaAPI Expires - ", t.teslaToken.Expiry)
				}
			} else {
				err = fmt.Errorf("AccessToken not found!--- %s", string(bytes))
				fmt.Println(err.Error())
			}
		}
	} else {
		fmt.Println("Error reading the API Key File - ", err.Error())
	}
	return t, err
}

func (api *TeslaAPI) cancelCommandHoldoff() {
	api.commandHoldOff = false
	fmt.Println("TeslaAPI - Holdoff cancelled")
}

// SendMail
// Send email to the administrator. Change the parameters for the email server etc. here.
func (api *TeslaAPI) SendMail(subject string, body string) error {
	fmt.Println("Sending mail to Ian : subject = ", subject, "\nbody : ", body)
	err := smtp.SendMail("mail.cedartechnology.com:587",
		smtp.PlainAuth("", "pi@cedartechnology.com", "7444561", "mail.cedartechnology.com"),
		"pi@cedartechnology.com", []string{"ian.abercrombie@cedartechnology.com"}, []byte(`From: Aberhome1
To: Ian.Abercrombie@cedartechnology.com
Subject: `+subject+`
`+body))
	return err
}

//func (api *TeslaAPI) ShowloginPage(w http.ResponseWriter, _ *http.Request) {
//	_, err := fmt.Fprint(w, `<html><head><title>Tesla API Login</title>
//</head>
//<body>
//	<form action="/getTeslaKeys" method="POST">
//		<label for="email">Email :</label><input id="email" type="text" name="identity" style="width:300px" value="Tesla@CedarTechnology.com" /><br>
//		<label for="password">Password :</label><input id="password" type="password" name="credential" style="width:300px" value="" /><br>
//		<button type="text" type="submit">log in to Tesla</button>
//	</form>
//</body>
//</html>`)
//	if err != nil {
//		fmt.Println(err)
//	}
//}

func (api *TeslaAPI) ShowloginPage(w http.ResponseWriter, _ *http.Request) {
	_, err := fmt.Fprint(w, `<html><head><title>Tesla API Keys</title>
	</head>
	<body>
		<form action="/getTeslaKeys" method="POST">
			<label for="access_key">Access Key :</label>
			<input id="access_key" type="text" name="access_key" style="width:800px" value="" /><br>
			<label for="refresh_key">Refresh Key :</label>
			<input id="refresh_key" name="refresh_key" style="width:800px" value="" /><br>
			<button type="text" type="submit">log in to Tesla</button>
		</form>
	</body>
	</html>`)
	if err != nil {
		fmt.Println(err)
	}
}

func (api *TeslaAPI) loginCompletePage() string {
	return `<html><head><title>Tesla keys - Success</title></head><body><h1>Tesla login Successful</h1><br />The Tesla API keys have been retrieved and recorded.</body></html>`
}

//func randomString(len int) string {
//
//	bytes := make([]byte, len)
//
//	for i := 0; i < len; i++ {
//		bytes[i] = byte(randInt(97, 122))
//	}
//
//	return string(bytes)
//}

//func randInt(min int, max int) int {
//
//	return min + rand.Intn(max-min)
//}

//func Base64Encode(src []byte) string {
//	return base64.RawURLEncoding.EncodeToString(src)
//}

//func getHiddenInputFields(doc *goquery.Document) url.Values {
//	//	values := make(map[string][]string)
//	values := url.Values{}
//	doc.Find("#form").Find("input").Each(func(i int, s *goquery.Selection) {
//		// For each hidden input field found add it to the form
//
//		inputName, foundName := s.Attr("name")
//		inputValue, _ := s.Attr("value")
//		inputType, foundType := s.Attr("type")
//		if foundType && foundName && (inputType == "hidden") {
//			values.Add(inputName, inputValue)
//			log.Println("Hidden field %s = %s", inputName, inputValue)
//		}
//	})
//
//	return values
//}

func (api *TeslaAPI) CheckForCaptcha(w http.ResponseWriter, doc *goquery.Document, form url.Values) bool {
	var hiddenFields = ""
	if doc.Find("[name|='captcha']").Length() > 0 {
		doc.Find("input").Each(func(i int, s *goquery.Selection) {
			fieldName, _ := s.Attr("name")
			fieldValue, _ := s.Attr("value")
			if fieldName != "captcha" {
				hiddenFields = hiddenFields + `<input type="hidden" name="` + fieldName
				if fieldName != "cancel" {
					if fieldName != "credential" {
						hiddenFields = hiddenFields + `" value="` + fieldValue
					} else {
						hiddenFields = hiddenFields + `" value="` + form["credential"][0]
					}
				}
				hiddenFields = hiddenFields + `" />`
			}
		})
		if len(form["code_verifier"]) > 0 {
			hiddenFields = hiddenFields + `<input type="hidden" name="code_verifier" value="` + form["code_verifier"][0] + `" />`
		}
		if len(form["state"]) > 0 {
			hiddenFields = hiddenFields + `<input type="hidden" name="state" value="` + form["state"][0] + `" />`
		}
		captcha, err := api.client.Get(APIAUTHCAPTCHAURL)
		if err != nil {
			_, _ = fmt.Fprint(w, err.Error())
			return false
		}
		graphic, err := ioutil.ReadAll(captcha.Body)
		err = captcha.Body.Close()
		if err != nil {
			log.Println(err)
		}
		_, err = fmt.Fprint(w,
			`<html>
	<head>
		<title>Captcha</title>
	</head>
	</body>`,
			string(graphic),
			`<br />Type in the charcters in the image<br />
		<form method='POST' action='/captcha'>
			<input type='text' name='captcha' /><br />
			<input type='submit' value='Submit' />`,
			hiddenFields,
			`</form>
	</body>
</html>`)
		if err != nil {
			log.Println(err)
		}
		return true
	} else {
		return false
	}
}

//func makeCodeChallenge(codeVerifier string) string {
//	hash := sha256.Sum256([]byte(codeVerifier))
//	return Base64Encode(hash[:])
//}

//func loginFailureMessageWithBody(w http.ResponseWriter, err error, body []byte) {
//	fmt.Println(err)
//	_, err = fmt.Fprint(w, `<html><head><title>Tesla API login Failure</title><head><body><h1>Error!</h1><br/><br/>`, html.EscapeString(err.Error()),
//		`<br /><iframe width=100% src=data:text/html;charset=utf-8,`, url.PathEscape(string(body)), ` /></body></html>`)
//	if err != nil {
//		fmt.Println(err)
//	}
//}

//func loginFailureMessage(w http.ResponseWriter, err error) {
//	fmt.Println(err)
//	_, err = fmt.Fprint(w, `<html><head><title>Tesla API login Failure</title><head><body><h1>Error!</h1><br/>`, err, `</body></html>`)
//	if err != nil {
//		fmt.Println(err)
//	}
//}

//// Called when /getTeslaKeys is requested. Login ID and Password for Tesla are provided in the form data.
//func (api *TeslaAPI) HandleTeslaLogin(w http.ResponseWriter, r *http.Request) {
//	var QueryParams = make(url.Values)
//	var getResponse *http.Response
//
//	rand.Seed(time.Now().UTC().UnixNano())	// Initialise the random number generator
//	codeVerifier := randomString(86)	// Create a random string for the codeVerifier
//	state := randomString(10)			// Create a random state string
//
//	err := r.ParseForm()					// Parse the form to get the login ID and password
//	if err != nil {
//		fmt.Println(err)
//	}
//	var captcha = ""
//	email := r.Form["identity"][0]
//	password := r.Form["credential"][0]
//	if len(r.Form["captcha"]) != 0 {
//		captcha = r.Form["captcha"][0]
//		codeVerifier = r.Form["code_verifier"][0]
//		state = r.Form["state"][0]
//	}
//	form := r.Form
//
//	// If no captcha, start by getting the login page.
//	if len(captcha) == 0 {
//		log.Println("No Captcha so getting login page")
//		QueryParams.Add("client_id", "ownerapi")
//		QueryParams.Add("code_challenge", makeCodeChallenge(codeVerifier))
//		QueryParams.Add("code_challenge_method", "S256")
//		QueryParams.Add("redirect_uri", APIAUTHCLIENTREDIRECTURL)
//		QueryParams.Add("response_type", "code")
//		QueryParams.Add("scope", APIAUTHSCOPE)
//		QueryParams.Add("state", state)
//		QueryParams.Add("login_hint", "tesla@cedartechnology.com")
//		strParams := QueryParams.Encode()
//
//		// Need to trap any redirect.
//		log.Println("Check redirect")
//		api.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
//			// Is this the expected redirect?
//			log.Println("req.URL.Path = ", req.URL.Path)
//			if req.URL.Path == APIAUTHCLIENTREDIRECTPATH {
//				// Look for the code in the query parameters
//				log.Println(req.URL)
//				api.code = req.URL.Query().Get("code")
//				if api.code != "" {
//					// We got the code as part of the redirect, so we can stop here.
//					// The page we are being sent to, does not exist any way.
//					return http.ErrUseLastResponse
//				}
//				// If we didn't get the code we should let this fail by letting it go to the redirect URL
//			}
//			// If this wwas not the expected redirect to get the code then simply allow it to be followed
//			if len(via) > 9 {
//				// Maximum of 10 redirects allowed
//				return errors.New("too many redirects")
//			}
//			return nil
//		}
//
//		log.Println("Getting login page <", APIAUTHORIZEURL + "?" + strParams + ">")
//		loginRequest, err := http.NewRequest("GET", APIAUTHORIZEURL + "?" + strParams, nil)
//		loginRequest.Header.Add("authority", "auth.tesla.com")
//		loginRequest.Header.Add("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")
//		loginRequest.Header.Add("accept-encoding", "gzip, deflate, br")
//		loginRequest.Header.Add("accept-language", "en-GB,en-US;q=0.9,en;q=0.8")
//		loginRequest.Header.Add("upgrade-insecure-requests", "1")
//		loginRequest.Header.Add("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/95.0.4638.69 Safari/537.36")
//		loginRequest.Header.Add("sec-ch-ua", `Google Chrome";v="95", "Chromium";v="95", ";Not A Brand";v="99"`)
//		loginRequest.Header.Add("sec-ch-ua-mobile", "?0")
//		loginRequest.Header.Add("sec-ch-ua-platform", `"macOS"`)
//		loginRequest.Header.Add("sec-fetch-dest", "document")
//		loginRequest.Header.Add("sec-fetch-mode", "navigate")
//		loginRequest.Header.Add("sec-fetch0-site", "none")
//		loginRequest.Header.Add("sec-fetch-user", "?1")
//		getResponse, err = api.client.Do(loginRequest)
//		log.Println("Response cookies = ", getResponse.Cookies())
////		getResponse, err := api.client.Get(APIAUTHORIZEURL + "?" + strParams)
//		defer func() {
//			err := getResponse.Body.Close()
//			if err != nil {
//				fmt.Println(err)
//			}
//		}()
//		if err != nil {
//			log.Println("Login page failure - ", err)
//			loginFailureMessage(w, err)
//			return
//		}
//
//		log.Println("Got initial login page - status = ", getResponse.StatusCode)
//		if getResponse.StatusCode != 200 {
//			body, err2 := ioutil.ReadAll(getResponse.Body)
//			err3 := getResponse.Body.Close()
//			if err3 != nil {
//				log.Println(err3)
//			}
//			if err2 != nil {
//				loginFailureMessage(w, err2)
//				return
//			}
//
//			loginFailureMessage(w, fmt.Errorf("Tesla login page returned %s\n", string(body)))
//			return
////		} else {
////			body, _ := ioutil.ReadAll(getResponse.Body)
////			getResponse.Body.Close()
//////			w.Header().Add("content-type", "text/html; charset=UTF-8")
////			fmt.Fprint(w, string(body))
////			return
//		}
//
//		//		for _, cookie := range getResponse.Cookies() {
//		//			if cookie.Name == "tesla-auth.sid" {
//		//				api.sessionCookie = *cookie
//		//			}
//		//		}
//		//		if api.sessionCookie.Value == "" {
//		//			loginFailureMessage(w, fmt.Errorf("TeslaAPI The session cookie was missing"))
//		//			return
//		//		}
//
//		//QueryParams = make(url.Values)
//		//QueryParams.Set("client_id", "ownerapi")
//		//QueryParams.Add("code_challenge", makeCodeChallenge(codeVerifier))
//		//QueryParams.Add("code_challenge_method", "S256")
//		//QueryParams.Add("redirect_uri", APIAUTHCLIENTREDIRECTURL)
//		//QueryParams.Add("response_type", "code")
//		//QueryParams.Add("scope", APIAUTHSCOPE)
//		//QueryParams.Add("state", state)
//		//QueryParams.Add("login_hint", "tesla@cedartechnology.com")
//
//		// Load the HTML document
//
//		doc, err := goquery.NewDocumentFromReader(getResponse.Body)
//		if err != nil {
//			loginFailureMessage(w, err)
//			return
//		}
//
//		form = getHiddenInputFields(doc)
//		form.Add("identity", email)
//		form.Add("credential", password)
////		form.Add("code_verifier", codeVerifier)
////		form.Add("state", state)
//		log.Println("Login form created - ", form)
//	} else {
//		log.Println("Captcha found login page")
//		codeVerifier = form["code_verifier"][0]
//		state = form["state"][0]
//	}
//	// Post the login form
//	//	request, err := http.NewRequest(http.MethodPost, APIAuthorizeURL+"?"+QueryParams.Encode(), strings.NewReader(form.Encode()))
////	fmt.Println(`=====>`, APIAUTHORIZEURL + "?" + QueryParams.Encode())
//
//	// Set the code to an empty string then send the post to log in
//	api.code = ""
//	postURL := APIAUTHORIZEURL+"?"+QueryParams.Encode()
//	log.Println("Post to <", postURL, "> | [", form, "]" )
//	log.Println(api.client)
//	req, err := http.NewRequest("POST", postURL, strings.NewReader(form.Encode()))
//	req.Header.Add("content-type", "application/x-www-form-urlencoded")
//	req.Header.Add("origin","https://auth.tesla.com")
////	req.Header.Add("referer", )
//	req.Header.Add("authority", "auth.tesla.com")
//	req.Header.Add("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")
//	req.Header.Add("accept-encoding", "gzip, deflate, br")
//	req.Header.Add("accept-language", "en-GB,en-US;q=0.9,en;q=0.8")
//	req.Header.Add("upgrade-insecure-requests", "1")
//	req.Header.Add("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/95.0.4638.69 Safari/537.36")
//	req.Header.Add("sec-ch-ua", `Google Chrome";v="95", "Chromium";v="95", ";Not A Brand";v="99"`)
//	req.Header.Add("sec-ch-ua-mobile", "?0")
//	req.Header.Add("sec-ch-ua-platform", `"macOS"`)
//	req.Header.Add("sec-fetch-dest", "document")
//	req.Header.Add("sec-fetch-mode", "navigate")
//	req.Header.Add("sec-fetch0-site", "none")
//	req.Header.Add("sec-fetch-user", "?1")
//	log.Println("headers : ", req.Header)
//	for _, c := range getResponse.Cookies() {
//		req.AddCookie(c)
//	}
//	log.Println("Response Cookies = ", getResponse.Cookies())
//	log.Println("Request Cookies = ", req.Cookies())
//	response, err := api.client.Do(req)
//	defer func() {
//		if response != nil {
//			err := response.Body.Close()
//			if err != nil {
//				loginFailureMessage(w, err)
//				return
//			}
//		}
//	}()
//
//	log.Println("Post returned ", response.StatusCode, " : ", response.Status)
//
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//
//	//	fmt.Println("Initial request returned status = %d", response.StatusCode)
//	if api.code == "" {
//		if response.StatusCode == 200 {
//			doc, err := goquery.NewDocumentFromReader(response.Body)
//			if err != nil {
//				loginFailureMessage(w, err)
//				return
//			}
//			log.Println("Check for captcha")
//			if api.CheckForCaptcha(w, doc, form) {
//				return
//			}
//			log.Println("captcha required...")
//			respBody, _ := doc.Html()
//			_, err2 := fmt.Fprint(w, respBody)
//			if err2 != nil {
//				log.Println(err2)
//			}
//			return
//		} else {
//			loginFailureMessage(w, fmt.Errorf(" Tesla login returned %d - %s", response.StatusCode, response.Status))
//		}
//		return
//	}
//
//	var requestParams = TokenRequestParams{
//		GrantType:    "authorization_code",
//		ClientID:     "ownerapi",
//		Code:         api.code,
//		CodeVerifier: codeVerifier,
//		RedirectUri:  APIAUTHCLIENTREDIRECTURL,
//		Scope:        APIAUTHSCOPE,
//	}
//
//	requestParams.GrantType = "authorization_code"
//	requestParams.ClientID = "ownerapi"
//	requestParams.Code = api.code
//	requestParams.CodeVerifier = codeVerifier
//	requestParams.RedirectUri = APIAUTHCLIENTREDIRECTURL
//
//	// Body should contain the request parameters in application/json format
//	requestBody, err := json.Marshal(requestParams)
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//	fmt.Println(string(requestBody))
//
//	response, err = api.client.Post(APIAUTHEXCHANGEURL, "application/json", strings.NewReader(string(requestBody)))
//	//	http.Post(APIAUTHEXCHANGEURL, "application/json", strings.NewReader(string(requestBody)))
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//	body, err := ioutil.ReadAll(response.Body)
//	closeError := response.Body.Close()
//	if closeError != nil {
//		log.Println(closeError)
//	}
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//	if response.StatusCode != 200 {
//		fmt.Println("Status = ", response.Status)
//	}
//
//	var tokenRead TokenRead
//	err = json.Unmarshal(body, &tokenRead)
//	if err != nil {
//		loginFailureMessageWithBody(w, err, body)
//		return
//	}
//
//	// Now we need to exchange the bearer token for the access token
//	var tokenExchangeParams = TokenExchangeParams{"urn:ietf:params:oauth:grant-type:jwt-bearer", APICLIENTID, APICLIENTSECRET}
//	tokenExchangeParams.GrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"
//	tokenExchangeParams.ClientID = APICLIENTID
//	tokenExchangeParams.ClientSecret = APICLIENTSECRET
//	requestBody, err = json.Marshal(tokenExchangeParams)
//
//	tokenRequest, err := http.NewRequest("POST", APITOKENURL, strings.NewReader(string(requestBody)))
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//
//	tokenRequest.Header.Add("Content-Type", "application/json")
//	tokenRequest.Header.Add("Authorization", "Bearer "+tokenRead.AccessToken)
//
//	response, err = http.DefaultClient.Do(tokenRequest)
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//
//	body, err = ioutil.ReadAll(response.Body)
//	err2 := response.Body.Close()
//	if err2 != nil {
//		log.Println(err2)
//	}
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//	if response.StatusCode != 200 {
//		loginFailureMessage(w, fmt.Errorf("Status returned when fetching a token = %s\n %s ", response.Status, tokenRequest.Header.Get("Authorization")))
//		_, err = fmt.Fprint(w, string(body), "\n\n")
//		if err != nil {
//			fmt.Println(err)
//		}
//		_, err = fmt.Fprint(w, string(requestBody))
//		if err != nil {
//			fmt.Println(err)
//		}
//		return
//	}
//
//	err = json.Unmarshal(body, &tokenRead)
//	if err != nil {
//		loginFailureMessageWithBody(w, err, body)
//		return
//	}
//
//	api.teslaToken.AccessToken = tokenRead.AccessToken
//	api.teslaToken.RefreshToken = tokenRead.RefreshToken
//	api.teslaToken.TokenType = tokenRead.TokenType
//	api.teslaToken.Expiry = time.Unix(tokenRead.CreatedAt, 0).Add(time.Second * time.Duration(tokenRead.ExpiresIn))
//	fmt.Println("Created - ", tokenRead.CreatedAt)
//	fmt.Println("Expires In - ", tokenRead.ExpiresIn)
//
//	newToken, err2 := json.Marshal(api.teslaToken)
//	if err2 != nil {
//		fmt.Println(err2)
//	} else {
//		err = ioutil.WriteFile(APIKEYFILE, newToken, os.ModePerm)
//		if err != nil {
//			fmt.Println(err)
//		}
//	}
//
//	api.teslaClient = api.oauthConfig.Client(api.ctx, api.teslaToken)
//
//	err = api.GetVehicleId()
//	if err != nil {
//		_, err = fmt.Fprint(w, `<html><head><title>Tesla Credentials Updated</title></head><body><h1>The Tesla API credentials have been updated.</h1><br>Error retrieving the Vehicle ID : `, err.Error(), `</body></html>`)
//	} else {
//		_, err = fmt.Fprint(w, `<html><head><title>Tesla Credentials Updated</title></head><body><h1>The Tesla API credentials have been updated.</h1><br>Vehicle ID = `, api.vId, `<br />`, CHARGINGLINKS, `</body></html>`)
//	}
//	if err != nil {
//		fmt.Println(err)
//	}
//
//}

//// Called when /getTeslaKeys is requested. Login ID and Password for Tesla are provided in the form data.
//func (api *TeslaAPI) HandleTeslaLogin(w http.ResponseWriter, r *http.Request) {
//	var QueryParams = make(url.Values)
//	var getResponse *http.Response
//
//	rand.Seed(time.Now().UTC().UnixNano())	// Initialise the random number generator
//	codeVerifier := randomString(86)	// Create a random string for the codeVerifier
//	state := randomString(10)			// Create a random state string
//
//	err := r.ParseForm()					// Parse the form to get the login ID and password
//	if err != nil {
//		fmt.Println(err)
//	}
//	var captcha = ""
//	email := r.Form["identity"][0]
//	password := r.Form["credential"][0]
//	if len(r.Form["captcha"]) != 0 {
//		captcha = r.Form["captcha"][0]
//		codeVerifier = r.Form["code_verifier"][0]
//		state = r.Form["state"][0]
//	}
//	form := r.Form
//
//	// If no captcha, start by getting the login page.
//	if len(captcha) == 0 {
//		log.Println("No Captcha so getting login page")
//		QueryParams.Add("client_id", "ownerapi")
//		QueryParams.Add("code_challenge", makeCodeChallenge(codeVerifier))
//		QueryParams.Add("code_challenge_method", "S256")
//		QueryParams.Add("redirect_uri", APIAUTHCLIENTREDIRECTURL)
//		QueryParams.Add("response_type", "code")
//		QueryParams.Add("scope", APIAUTHSCOPE)
//		QueryParams.Add("state", state)
//		QueryParams.Add("login_hint", "tesla@cedartechnology.com")
//		strParams := QueryParams.Encode()
//
//		// Need to trap any redirect.
//		log.Println("Check redirect")
//		api.client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
//			// Is this the expected redirect?
//			log.Println("req.URL.Path = ", req.URL.Path)
//			if req.URL.Path == APIAUTHCLIENTREDIRECTPATH {
//				// Look for the code in the query parameters
//				log.Println(req.URL)
//				api.code = req.URL.Query().Get("code")
//				if api.code != "" {
//					// We got the code as part of the redirect, so we can stop here.
//					// The page we are being sent to, does not exist any way.
//					return http.ErrUseLastResponse
//				}
//				// If we didn't get the code we should let this fail by letting it go to the redirect URL
//			}
//			// If this wwas not the expected redirect to get the code then simply allow it to be followed
//			if len(via) > 9 {
//				// Maximum of 10 redirects allowed
//				return errors.New("too many redirects")
//			}
//			return nil
//		}
//
//		log.Println("Getting login page <", APIAUTHORIZEURL + "?" + strParams + ">")
//		loginRequest, err := http.NewRequest("GET", APIAUTHORIZEURL + "?" + strParams, nil)
//		loginRequest.Header.Add("authority", "auth.tesla.com")
//		loginRequest.Header.Add("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")
//		loginRequest.Header.Add("accept-encoding", "gzip, deflate, br")
//		loginRequest.Header.Add("accept-language", "en-GB,en-US;q=0.9,en;q=0.8")
//		loginRequest.Header.Add("upgrade-insecure-requests", "1")
//		loginRequest.Header.Add("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/95.0.4638.69 Safari/537.36")
//		loginRequest.Header.Add("sec-ch-ua", `Google Chrome";v="95", "Chromium";v="95", ";Not A Brand";v="99"`)
//		loginRequest.Header.Add("sec-ch-ua-mobile", "?0")
//		loginRequest.Header.Add("sec-ch-ua-platform", `"macOS"`)
//		loginRequest.Header.Add("sec-fetch-dest", "document")
//		loginRequest.Header.Add("sec-fetch-mode", "navigate")
//		loginRequest.Header.Add("sec-fetch0-site", "none")
//		loginRequest.Header.Add("sec-fetch-user", "?1")
//		getResponse, err = api.client.Do(loginRequest)
//		log.Println("Response cookies = ", getResponse.Cookies())
////		getResponse, err := api.client.Get(APIAUTHORIZEURL + "?" + strParams)
//		defer func() {
//			err := getResponse.Body.Close()
//			if err != nil {
//				fmt.Println(err)
//			}
//		}()
//		if err != nil {
//			log.Println("Login page failure - ", err)
//			loginFailureMessage(w, err)
//			return
//		}
//
//		log.Println("Got initial login page - status = ", getResponse.StatusCode)
//		if getResponse.StatusCode != 200 {
//			body, err2 := ioutil.ReadAll(getResponse.Body)
//			err3 := getResponse.Body.Close()
//			if err3 != nil {
//				log.Println(err3)
//			}
//			if err2 != nil {
//				loginFailureMessage(w, err2)
//				return
//			}
//
//			loginFailureMessage(w, fmt.Errorf("Tesla login page returned %s\n", string(body)))
//			return
////		} else {
////			body, _ := ioutil.ReadAll(getResponse.Body)
////			getResponse.Body.Close()
//////			w.Header().Add("content-type", "text/html; charset=UTF-8")
////			fmt.Fprint(w, string(body))
////			return
//		}
//
//		//		for _, cookie := range getResponse.Cookies() {
//		//			if cookie.Name == "tesla-auth.sid" {
//		//				api.sessionCookie = *cookie
//		//			}
//		//		}
//		//		if api.sessionCookie.Value == "" {
//		//			loginFailureMessage(w, fmt.Errorf("TeslaAPI The session cookie was missing"))
//		//			return
//		//		}
//
//		//QueryParams = make(url.Values)
//		//QueryParams.Set("client_id", "ownerapi")
//		//QueryParams.Add("code_challenge", makeCodeChallenge(codeVerifier))
//		//QueryParams.Add("code_challenge_method", "S256")
//		//QueryParams.Add("redirect_uri", APIAUTHCLIENTREDIRECTURL)
//		//QueryParams.Add("response_type", "code")
//		//QueryParams.Add("scope", APIAUTHSCOPE)
//		//QueryParams.Add("state", state)
//		//QueryParams.Add("login_hint", "tesla@cedartechnology.com")
//
//		// Load the HTML document
//
//		doc, err := goquery.NewDocumentFromReader(getResponse.Body)
//		if err != nil {
//			loginFailureMessage(w, err)
//			return
//		}
//
//		form = getHiddenInputFields(doc)
//		form.Add("identity", email)
//		form.Add("credential", password)
////		form.Add("code_verifier", codeVerifier)
////		form.Add("state", state)
//		log.Println("Login form created - ", form)
//	} else {
//		log.Println("Captcha found login page")
//		codeVerifier = form["code_verifier"][0]
//		state = form["state"][0]
//	}
//	// Post the login form
//	//	request, err := http.NewRequest(http.MethodPost, APIAuthorizeURL+"?"+QueryParams.Encode(), strings.NewReader(form.Encode()))
////	fmt.Println(`=====>`, APIAUTHORIZEURL + "?" + QueryParams.Encode())
//
//	// Set the code to an empty string then send the post to log in
//	api.code = ""
//	postURL := APIAUTHORIZEURL+"?"+QueryParams.Encode()
//	log.Println("Post to <", postURL, "> | [", form, "]" )
//	log.Println(api.client)
//	req, err := http.NewRequest("POST", postURL, strings.NewReader(form.Encode()))
//	req.Header.Add("content-type", "application/x-www-form-urlencoded")
//	req.Header.Add("origin","https://auth.tesla.com")
////	req.Header.Add("referer", )
//	req.Header.Add("authority", "auth.tesla.com")
//	req.Header.Add("accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9")
//	req.Header.Add("accept-encoding", "gzip, deflate, br")
//	req.Header.Add("accept-language", "en-GB,en-US;q=0.9,en;q=0.8")
//	req.Header.Add("upgrade-insecure-requests", "1")
//	req.Header.Add("user-agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/95.0.4638.69 Safari/537.36")
//	req.Header.Add("sec-ch-ua", `Google Chrome";v="95", "Chromium";v="95", ";Not A Brand";v="99"`)
//	req.Header.Add("sec-ch-ua-mobile", "?0")
//	req.Header.Add("sec-ch-ua-platform", `"macOS"`)
//	req.Header.Add("sec-fetch-dest", "document")
//	req.Header.Add("sec-fetch-mode", "navigate")
//	req.Header.Add("sec-fetch0-site", "none")
//	req.Header.Add("sec-fetch-user", "?1")
//	log.Println("headers : ", req.Header)
//	for _, c := range getResponse.Cookies() {
//		req.AddCookie(c)
//	}
//	log.Println("Response Cookies = ", getResponse.Cookies())
//	log.Println("Request Cookies = ", req.Cookies())
//	response, err := api.client.Do(req)
//	defer func() {
//		if response != nil {
//			err := response.Body.Close()
//			if err != nil {
//				loginFailureMessage(w, err)
//				return
//			}
//		}
//	}()
//
//	log.Println("Post returned ", response.StatusCode, " : ", response.Status)
//
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//
//	//	fmt.Println("Initial request returned status = %d", response.StatusCode)
//	if api.code == "" {
//		if response.StatusCode == 200 {
//			doc, err := goquery.NewDocumentFromReader(response.Body)
//			if err != nil {
//				loginFailureMessage(w, err)
//				return
//			}
//			log.Println("Check for captcha")
//			if api.CheckForCaptcha(w, doc, form) {
//				return
//			}
//			log.Println("captcha required...")
//			respBody, _ := doc.Html()
//			_, err2 := fmt.Fprint(w, respBody)
//			if err2 != nil {
//				log.Println(err2)
//			}
//			return
//		} else {
//			loginFailureMessage(w, fmt.Errorf(" Tesla login returned %d - %s", response.StatusCode, response.Status))
//		}
//		return
//	}
//
//	var requestParams = TokenRequestParams{
//		GrantType:    "authorization_code",
//		ClientID:     "ownerapi",
//		Code:         api.code,
//		CodeVerifier: codeVerifier,
//		RedirectUri:  APIAUTHCLIENTREDIRECTURL,
//		Scope:        APIAUTHSCOPE,
//	}
//
//	requestParams.GrantType = "authorization_code"
//	requestParams.ClientID = "ownerapi"
//	requestParams.Code = api.code
//	requestParams.CodeVerifier = codeVerifier
//	requestParams.RedirectUri = APIAUTHCLIENTREDIRECTURL
//
//	// Body should contain the request parameters in application/json format
//	requestBody, err := json.Marshal(requestParams)
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//	fmt.Println(string(requestBody))
//
//	response, err = api.client.Post(APIAUTHEXCHANGEURL, "application/json", strings.NewReader(string(requestBody)))
//	//	http.Post(APIAUTHEXCHANGEURL, "application/json", strings.NewReader(string(requestBody)))
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//	body, err := ioutil.ReadAll(response.Body)
//	closeError := response.Body.Close()
//	if closeError != nil {
//		log.Println(closeError)
//	}
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//	if response.StatusCode != 200 {
//		fmt.Println("Status = ", response.Status)
//	}
//
//	var tokenRead TokenRead
//	err = json.Unmarshal(body, &tokenRead)
//	if err != nil {
//		loginFailureMessageWithBody(w, err, body)
//		return
//	}
//
//	// Now we need to exchange the bearer token for the access token
//	var tokenExchangeParams = TokenExchangeParams{"urn:ietf:params:oauth:grant-type:jwt-bearer", APICLIENTID, APICLIENTSECRET}
//	tokenExchangeParams.GrantType = "urn:ietf:params:oauth:grant-type:jwt-bearer"
//	tokenExchangeParams.ClientID = APICLIENTID
//	tokenExchangeParams.ClientSecret = APICLIENTSECRET
//	requestBody, err = json.Marshal(tokenExchangeParams)
//
//	tokenRequest, err := http.NewRequest("POST", APITOKENURL, strings.NewReader(string(requestBody)))
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//
//	tokenRequest.Header.Add("Content-Type", "application/json")
//	tokenRequest.Header.Add("Authorization", "Bearer "+tokenRead.AccessToken)
//
//	response, err = http.DefaultClient.Do(tokenRequest)
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//
//	body, err = ioutil.ReadAll(response.Body)
//	err2 := response.Body.Close()
//	if err2 != nil {
//		log.Println(err2)
//	}
//	if err != nil {
//		loginFailureMessage(w, err)
//		return
//	}
//	if response.StatusCode != 200 {
//		loginFailureMessage(w, fmt.Errorf("Status returned when fetching a token = %s\n %s ", response.Status, tokenRequest.Header.Get("Authorization")))
//		_, err = fmt.Fprint(w, string(body), "\n\n")
//		if err != nil {
//			fmt.Println(err)
//		}
//		_, err = fmt.Fprint(w, string(requestBody))
//		if err != nil {
//			fmt.Println(err)
//		}
//		return
//	}
//
//	err = json.Unmarshal(body, &tokenRead)
//	if err != nil {
//		loginFailureMessageWithBody(w, err, body)
//		return
//	}
//
//	api.teslaToken.AccessToken = tokenRead.AccessToken
//	api.teslaToken.RefreshToken = tokenRead.RefreshToken
//	api.teslaToken.TokenType = tokenRead.TokenType
//	api.teslaToken.Expiry = time.Unix(tokenRead.CreatedAt, 0).Add(time.Second * time.Duration(tokenRead.ExpiresIn))
//	fmt.Println("Created - ", tokenRead.CreatedAt)
//	fmt.Println("Expires In - ", tokenRead.ExpiresIn)
//
//	newToken, err2 := json.Marshal(api.teslaToken)
//	if err2 != nil {
//		fmt.Println(err2)
//	} else {
//		err = ioutil.WriteFile(APIKEYFILE, newToken, os.ModePerm)
//		if err != nil {
//			fmt.Println(err)
//		}
//	}
//
//	api.teslaClient = api.oauthConfig.Client(api.ctx, api.teslaToken)
//
//	err = api.GetVehicleId()
//	if err != nil {
//		_, err = fmt.Fprint(w, `<html><head><title>Tesla Credentials Updated</title></head><body><h1>The Tesla API credentials have been updated.</h1><br>Error retrieving the Vehicle ID : `, err.Error(), `</body></html>`)
//	} else {
//		_, err = fmt.Fprint(w, `<html><head><title>Tesla Credentials Updated</title></head><body><h1>The Tesla API credentials have been updated.</h1><br>Vehicle ID = `, api.vId, `<br />`, CHARGINGLINKS, `</body></html>`)
//	}
//	if err != nil {
//		fmt.Println(err)
//	}
//
//}

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

// Called when /getTeslaKeys is requested. Login ID and Password for Tesla are provided in the form data.
func (api *TeslaAPI) HandleTeslaLogin(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm() // Parse the form to get the keys
	if err != nil {
		log.Println(err)
		_, err := fmt.Fprint(w, "<html><title>Failed to get keys</title><head></head><body><h1>Failed to get the keys</h1>", err.Error(), "</body></html>")
		if err != nil {
			log.Println(err)
		}
	}

	api.teslaToken.AccessToken = r.Form["access_key"][0]
	api.teslaToken.RefreshToken = r.Form["refresh_key"][0]
	api.teslaToken.TokenType = "bearer"
	api.teslaToken.Expiry = time.Now().Add(time.Hour * 5)
	api.teslaToken.IDToken = ""

	newToken, err2 := json.Marshal(api.teslaToken)
	if err2 != nil {
		fmt.Println(err2)
	} else {
		err = ioutil.WriteFile(APIKEYFILE, newToken, os.ModePerm)
		if err != nil {
			fmt.Println(err)
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
		fmt.Println(err)
	}
}

/*
func (api *TeslaAPI) CheckMultiFactorAuthentication(transaction_id string) string {
	params := url.Values{"transaction_id": {transaction_id}}
	request, err := http.NewRequest(http.MethodGet, APIMultiFactorURL+"?"+params.Encode(), nil)
	if err != nil {
		return ("New request failed in CheckMultiFactorAuthentication - " + err.Error())
	}
	response, err := api.client.Transport.RoundTrip(request)
	//	getResponse, err := http.Get(APIMultiFactorURL + "?" + params.Encode())
	if err != nil {
		return ("MFA query returned " + err.Error())
	}
	body, err := ioutil.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		fmt.Println(err)
	}
	return string(body)
}
*/

func (api *TeslaAPI) postCarCommand(sCommand string) ([]byte, error) {
	response, err := api.teslaClient.Post(sCommand, "application/json", nil)
	if err != nil {
		fmt.Println(err)
	} else {
		defer func() {
			err := response.Body.Close()
			if err != nil {
				fmt.Println(err)
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
			fmt.Println(err)
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
			return err
		} else {
			defer func() {
				err := response.Body.Close()
				if err != nil {
					fmt.Println(err)
				}
			}()
			body, err := ioutil.ReadAll(response.Body)
			errorClose := response.Body.Close()
			if errorClose != nil {
				log.Println(errorClose)
			}
			if err != nil {
				return err
			} else {
				if response.StatusCode == 401 {
					return fmt.Errorf("TeslaAPI returned unauthorised. Token = %s", api.teslaToken.AccessToken)
				}
				if response.StatusCode == 404 {
					return fmt.Errorf("TeslaAPI returned %s when looking for vehicle IDs", response.Status)
				}
				fmt.Println("TeslaAPI - Get Vehicles returned status : ", response.StatusCode, " : ", response.Status)
				err = json.Unmarshal(body, &vehicles)
				if err != nil {
					return err
				} else {
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
	time.AfterFunc(time.Minute*2, api.cancelCommandHoldoff)

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
	time.AfterFunc(time.Minute*2, api.cancelCommandHoldoff)

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
