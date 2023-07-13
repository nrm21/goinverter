package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bearsh/hid"
)

var debug bool
var lastUsbUpdate, usbUpdateInterval, batteryChargeShift, batteryDischargeShift int64
var lastQuery *QueryResponse
var measurementName, httpPort string

// Make it possible to kill program by typing ctrl-C
func SetupCloseHandler() {
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\nCtrl+C pressed in Terminal")
		os.Exit(0)
	}()
}

// Checks for arguments to the program on start
func CheckArgs(arguments []string) bool {
	for i := 0; i < len(arguments); i++ {
		if arguments[i] == "-d" { // [debug]
			fmt.Println("Debug flag set")
			debug = true
		} else if arguments[i] == "-i" { // [interval <time>]
			i++
			updateTime, err := strconv.Atoi(arguments[i])
			usbUpdateInterval = int64(updateTime)
			if err != nil {
				fmt.Println("Error with interval argument parameter, check if it is a number")
			}
		} else if arguments[i] == "-p" { // [port <portnum>]
			i++
			httpPort = arguments[i]
		}
	}
	return false
}

// Pretty print JSON data
func PrettyString(str string) string {
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, []byte(str), "", "    "); err != nil {
		return ""
	}
	return prettyJSON.String()
}

// Handle http to print status
func handleHttpStatus(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Client connected from %s with URI %s\n", strings.Split(r.RemoteAddr, ":")[0], r.URL.Path)
	fmt.Fprintf(w, "status: \"ok\"\n")
}

// Handle http to run raw commands
func handleHttpRaw(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if strings.Contains(r.URL.Path, "query") {
		// add to the struct the last time we updated in seconds
		lastUpdatedSecsAgo := time.Now().Unix() - lastUsbUpdate
		lastQuery.LastUpdated = lastUpdatedSecsAgo

		// turn response into JSON and pretty print for webpage display
		jsonStruct, _ := json.Marshal(lastQuery)
		fmt.Fprintf(w, "%s\n", PrettyString(string(jsonStruct)))
	} else if strings.Contains(r.URL.Path, "raw") {
		// run command and send response to web
		lastQuery := r.URL.Query()
		rawResponse := sendToInverterAndRetry(lastQuery.Get("cmd"))
		fmt.Fprintf(w, "%s\n", rawResponse) // and send response to console also
	}
}

// Send command to inverter and retry if response was not correct
func sendToInverterAndRetry(cmd string) string {
	var response string
	devices := hid.Enumerate(1637, 20833) // this is the vendor and product id of my inverter
	inverterDev := &devices[0]            // the first device will usually be the one we want

	if cmd == "QMOD" {
		var bytesRead int
		for bytesRead != 8 {
			response, bytesRead = writeToInverter(inverterDev, &cmd)
			if bytesRead != 8 {
				fmt.Println("Not an 8 byte response... resending command to inverter")
				time.Sleep(2000 * time.Millisecond)
			}
		}
	} else if cmd == "QPIRI" || cmd == "QPIGS" {
		var bytesRead int
		for bytesRead < 104 {
			response, bytesRead = writeToInverter(inverterDev, &cmd)
			if bytesRead < 104 {
				fmt.Println("Less than 104 byte response... resending command to inverter")
				time.Sleep(2000 * time.Millisecond)
			}
		}
	} else { // else this isn't a query that returns a long response
		response, _ = writeToInverter(inverterDev, &cmd)
		//response = fmt.Sprintf("{\"msg\": \"%s\"}", response) // send to web as JSON
	}

	return response
}

// Sends QPIGS then QPIRI to the USB and returns the result of both
func doStatusUpdate() {
	newQuery := &QueryResponse{}

	cmds := strings.Split("QMOD|QPIGS|QPIRI", "|")
	for _, cmd := range cmds {
		responseParser(cmd, sendToInverterAndRetry(cmd), newQuery)
	}
	newQuery.PV_in_watts = newQuery.SCC_voltage * newQuery.PV_in_current
	newQuery.PV_in_watthour = newQuery.PV_in_watts / (3600 / float64(usbUpdateInterval))
	newQuery.PV_in_watthour = math.Round(newQuery.PV_in_watthour*100) / 100 // 2 digit round

	// only if in battery mode do we want to calculate, otherwise leave at 0
	if newQuery.Inverter_mode_str == "B" {
		newQuery.Load_watthour = newQuery.Load_watts / (3600 / float64(usbUpdateInterval))
		newQuery.Load_watthour = math.Round(newQuery.Load_watthour*100) / 100 // 2 digit round
	}

	// shift battery current up/down by this since the inverter lies to us slightly about actual draw
	if newQuery.Battery_charge_current > 1 {
		newQuery.Battery_charge_current += batteryChargeShift
	} else if newQuery.Battery_discharge_current > 1 {
		newQuery.Battery_discharge_current += batteryDischargeShift
	}

	// sets the measurement name (only matters for InfluxDB)
	newQuery.Measurement = measurementName

	// Rather than access the global var directly, lets leave the previous data there
	// and only overwrite it once we have all the data we need from our queries to the
	// USB device which can take some time.  This way the user never sees a lapse in data
	// when going to the webpage.
	lastQuery = newQuery

	if debug {
		fmt.Println("Status update complete")
	}
}

// Runs in a loop handling any misreads and doing them over again as needed
func handleUSBTraffic() {
	// loop forever until user breaks since we are web serving
	// and waiting for user response over and over again
	for {
		if lastUsbUpdate+usbUpdateInterval < time.Now().Unix() {
			doStatusUpdate() // saves data to usbResponse var
			lastUsbUpdate = time.Now().Unix()
		} else {
			time.Sleep(1 * time.Second)
		}
	}
}

func main() {
	// set global var defaults in case not set by program parameters
	usbUpdateInterval = 12
	httpPort = "8088"
	measurementName = "exec_solar" // override the measurement name (for InfluxDB backend)
	batteryChargeShift = 0
	batteryDischargeShift = 1

	SetupCloseHandler()
	if len(os.Args) > 1 { // if we have arguments parse them
		CheckArgs(os.Args)
	}

	http.HandleFunc("/status", handleHttpStatus) // health check (for k8s readiness probes)
	http.HandleFunc("/health", handleHttpStatus) // health check (for k8s readiness probes)
	http.HandleFunc("/query", handleHttpRaw)
	http.HandleFunc("/raw", handleHttpRaw)

	go handleUSBTraffic()

	fmt.Println("Server started at port " + httpPort)
	log.Fatal(http.ListenAndServe(":"+httpPort, nil))
}
