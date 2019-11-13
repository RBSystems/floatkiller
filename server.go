package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/byuoitav/common"
	"github.com/labstack/echo"
)

type roomDetails struct {
	oldIpAddress string
	hostname     string
	newIpAddress string
	designation  string
}
type tokens struct {
	RefreshToken string `json:"refresh_token"`
	AccessToken  string `json:"access_token"`
	IDToken      string `json:"id_token"`
}

func main() {
	port := ":7444"
	router := common.NewRouter()
	router.GET("/detailsSet", detailsSet)
	router.GET("/dnsLookup/:hostname", dnsLookup)
	router.GET("/getDHCP", getCurrentIP)
	router.GET("/getdata/:hostname", getAllData)
	router.GET("/designation/:designation", setDesignation)
	router.GET("/", floatShip)
	server := http.Server{
		Addr:           port,
		MaxHeaderBytes: 1024 * 10,
	}
	go func() {
		//get onboard on a pi
		_, err := exec.Command("sudo apt install onboard").Output()
		if err != nil {
			fmt.Printf("%s", err)
		}
		fmt.Println("Command Successfully Executed")
		_, err = exec.Command("onboard").Output()
		if err != nil {
			fmt.Printf("%s", err)
		}
		fmt.Println("Command Successfully Executed")
		//If the hostname and IP have been set then direct them to the right page
		hostname, err := os.Hostname()
		if err != nil {
			panic(err)
		}
		if hostname != "pi" {
			openBrowser("http://localhost:7444/detailsSet")
		} else {
			openBrowser("http://localhost:7444")
		}

	}()
	router.StartServer(&server)

}

var Details roomDetails

func openBrowser(url string) bool {
	var args []string
	switch runtime.GOOS {
	case "darwin":
		args = []string{"open"}
	case "windows":
		args = []string{"cmd", "/c", "start"}
	default:
		args = []string{"xdg-open"}
	}
	cmd := exec.Command(args[0], append(args[1:], url)...)
	return cmd.Start() == nil
}
func floatShip(c echo.Context) error {
	authCode := c.QueryParam("code")
	fmt.Printf("%s\n", authCode)
	//designation := ""
	if authCode != "" {
		//has the auth code now get the token
		token, err := doTokenRequest(authCode)
		if err != nil {
			return c.String(http.StatusInternalServerError, fmt.Sprintf("failure: %s", err))
		}
		err = deploy(token.AccessToken)
		if err == nil {
			return c.String(http.StatusOK, "Successful")
		}
		return c.String(http.StatusInternalServerError, fmt.Sprintf("failure: %s", err))
	} else {
		//return the root
		dat, err := ioutil.ReadFile("./index.html")
		if err != nil {
			return c.String(http.StatusInternalServerError, "failure to read file")
		}
		return c.HTMLBlob(http.StatusOK, dat)
	}
	return c.String(http.StatusInternalServerError, "failure")

}

func detailsSet(c echo.Context) error {
	hostname, err := os.Hostname()
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Unable to get Host Name: %s", err))
	}
	ipAddr := getCurrentIPHelper()
	Details = roomDetails{
		hostname:     hostname,
		newIpAddress: ipAddr,
	}
	indexHTML := fmt.Sprintf(`<html>
    <head>
        <title>Pi Setup</title>
        <link rel="stylesheet" href="https://stackpath.bootstrapcdn.com/bootstrap/4.3.1/css/bootstrap.min.css" integrity="sha384-ggOyR0iXCbMQv3Xipma34MD+dH/1fQ784/j6cY/iJTQUOhcWr7x9JvoRxT2MZw1T" crossorigin="anonymous"/>
        <script src="https://ajax.googleapis.com/ajax/libs/jquery/3.4.1/jquery.min.js"></script>
        <script type='text/javascript'>
            function myFunction() {
                var x = document.getElementById("mySelect").value;
                $.ajax({
                    url : "http://localhost:7444/designation/"+x,
                    type : 'GET',
                    success: function(data) {
                        console.log("Success");
                    }
                }); 
            }
            $(document).ready(function(){
                               
                $('#deploybutton').click(function() {
                    window.location = "https://api.byu.edu/authorize?response_type=code&client_id=nkvyVWVBiqOKs_o7dLkUF2KHv2Ya&redirect_uri=http://localhost:7444&scope=openid";
                }); 
            });
        </script>
    </head>

    <body>
        <div class="container">
            <div class="row">
                <div class="col-sm">
                    <span id="success">
                            <h1>Device Details</h1>
                            <label>Host Name:</label><br />
                            <input type="text" value=%s disabled><br />
                            <label>IP Address:</label><br />
                            <input type="text" value=%s disabled><br />
                            </br>
                    </span>
                </div>
                <div id="deploy" class="col-sm">
                    <select id="mySelect" onchange="myFunction()">
                        <option value="prd">Production
                        <option value="stg">Stage
                        <option value="dev">Development
                    </select>
                    <br>
                    <br>
                    <br>
                    <button id="deploybutton" class="btn btn-primary btn-lg" style="margin:50%">Deploy</button>
                </div>
            </div>

        </div>
    </body>
</html>`, Details.hostname, Details.newIpAddress)
	return c.HTML(http.StatusOK, indexHTML)
}

func deploy(authCode string) error {

	req, err := http.NewRequest("GET", fmt.Sprintf("https://api.byu.edu/domains/av/flight-deck/%v/webhook_device/%v", Details.designation, Details.hostname), nil)
	if err != nil {
		return fmt.Errorf("couldn't make request: %v", err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %v", authCode))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't perform request: %v", err)
	}

	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("couldn't read the response body: %v", err)
	}

	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("non-200 status code: %v - %s", resp.StatusCode, body)
	}

	fmt.Printf("Deployment successful\n")
	return nil
}

func dnsLookup(c echo.Context) error {
	hostname := c.Param("hostname")
	ipaddress, err := dnsLookupHelper(hostname)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Not able to get new address"))
	}
	return c.String(http.StatusOK, ipaddress)
}
func setDesignation(c echo.Context) error {
	designation := c.Param("designation")
	Details.designation = designation
	fmt.Printf("\nDESIGNATION: %s\n", Details.designation)
	return c.String(http.StatusOK, "designation set")
}

func getCurrentIP(c echo.Context) error {
	ipaddress := getCurrentIPHelper()
	return c.String(http.StatusOK, ipaddress)
}
func getCurrentIPHelper() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "Not found"
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {

				return ipnet.IP.String()
			}
		}
	}
	return "Not found"
}

func getAllData(c echo.Context) error {
	hostname := c.Param("hostname")
	fmt.Printf("%v", hostname)
	oldIpAddr := getCurrentIPHelper()
	newIpAddr, err := dnsLookupHelper(hostname)
	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("Not able to get new address"))
	}
	Details := roomDetails{
		hostname:     hostname,
		oldIpAddress: oldIpAddr,
		newIpAddress: newIpAddr,
	}
	fmt.Printf("Hostname: %s\n", Details.hostname)
	fmt.Printf("oldIP: %s\n", Details.oldIpAddress)
	fmt.Printf("newIP: %s\n", Details.newIpAddress)

	resp := fmt.Sprintf(`
		<label>Host Name:</label><br/>
		<input type='text' value='%s'><br/>
		<label>IP Address:</label><br />
		<input type='text' value='%s'><br />`, Details.hostname, Details.newIpAddress)

	return c.JSON(http.StatusOK, resp)
}

func dnsLookupHelper(hostname string) (string, error) {
	ips, err := net.LookupIP(hostname + ".byu.edu")
	if err != nil {
		return "", fmt.Errorf("Unable to get IP: %v", err)
	}
	return ips[0].String(), nil
}

func doTokenRequest(auth string) (tokens, error) {
	ret := tokens{}
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", auth)
	data.Set("redirect_uri", "http://localhost:7444")

	req, err := http.NewRequest("POST", "https://api.byu.edu/token", strings.NewReader(data.Encode()))
	if err != nil {
		return ret, fmt.Errorf("unable to build request: %s", err)
	}

	req.SetBasicAuth("nkvyVWVBiqOKs_o7dLkUF2KHv2Ya", "HR_ssS_Kv1q_9xq1j_wJr1F8Fn0a")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ret, fmt.Errorf("unable to make request: %s", err)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return ret, fmt.Errorf("unable to read response: %s", err)
	}

	if resp.StatusCode != 200 {
		return ret, fmt.Errorf("non-200 response: %s", body)
	}

	err = json.Unmarshal(body, &ret)
	if err != nil {
		return ret, fmt.Errorf("unable to unmarshal response: %s", err)
	}

	return ret, nil
}
