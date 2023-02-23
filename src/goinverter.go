package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bearsh/hid"
)

var usbResponse string
var debug bool
var lastUsbUpdate, usbUpdateInterval int64

const httpIp = "" // leave blank for 0.0.0.0, else use a specific interface or localhost for more security
const httpPort = "8088"

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

// Checks for arguments to teh program on start
func checkArgs() bool {
	if len(os.Args) > 1 { // if we have arguments parse them
		arguments := os.Args
		for i := 0; i < len(arguments); i++ {
			if arguments[i] == "-d" {
				fmt.Println("Debug flag set")
				debug = true
			} else if arguments[i] == "-i" {
				i++
				updateTime, err := strconv.Atoi(arguments[i])
				usbUpdateInterval = int64(updateTime)
				if err != nil {
					fmt.Println("Error with interval argument parameter, check if it is a number")
				}
			}
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
		fmt.Fprintf(w, "%s\n", PrettyString(usbResponse)) // pretty print for webpage display
		//fmt.Fprintf(w, "%s\n", usbResponse)
	} else if strings.Contains(r.URL.Path, "raw") {
		// run command and send response to web
		query := r.URL.Query()
		rawResponse := sendToInverterAndRetry(query.Get("cmd"))
		fmt.Fprintf(w, "%s\n", rawResponse) // and send response to console also
	}
}

// Send command to inverter and retry if response was not correct
func sendToInverterAndRetry(cmd string) string {
	var response string
	devices := hid.Enumerate(0, 0)
	inverterDev := &devices[0] // the first device will usually be the one we want

	if cmd == "QPIRI" || cmd == "QPIGS" {
		var bytesRead int
		for bytesRead < 104 {
			response, bytesRead = writeToInverter(inverterDev, &cmd)
			if debug && bytesRead < 104 {
				fmt.Println("Less than 104 byte response... resending command to inverter")
				time.Sleep(2500 * time.Millisecond)
			}
		}
	} else { // else this isn't a query that returns a long response
		response, _ = writeToInverter(inverterDev, &cmd)
		//response = fmt.Sprintf("{\"msg\": \"%s\"}", response) // JSON-ify
	}

	return response
}

// Sends QPIGS then QPIRI to the USB and returns the result of both
func doStatusUpdate() {
	qr := &QueryResponse{}

	cmd := "QPIGS"
	response := sendToInverterAndRetry(cmd)
	responseParser(cmd, &response, qr)

	cmd = "QPIRI"
	response = sendToInverterAndRetry(cmd)
	responseParser(cmd, &response, qr)

	// turn response into JSON and stringify it
	jsonStruct, _ := json.Marshal(qr)

	usbResponse = string(jsonStruct)
	if debug {
		fmt.Println("Status update complete\n")
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
	// set defaults incase not set by program
	usbUpdateInterval = 10

	SetupCloseHandler()
	checkArgs()

	http.HandleFunc("/status", handleHttpStatus) // health check (for k8s readiness probes)
	http.HandleFunc("/health", handleHttpStatus) // health check (for k8s readiness probes)
	http.HandleFunc("/query", handleHttpRaw)
	http.HandleFunc("/raw", handleHttpRaw)

	go handleUSBTraffic()

	fmt.Println("Server started at port " + httpPort)
	http.ListenAndServe(httpIp+":"+httpPort, nil)
}
