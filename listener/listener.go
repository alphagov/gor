// Listener capture TCP traffic using RAW SOCKETS.
// Note: it requires sudo or root access.
//
// Right now it supports only HTTP
package listener

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
)

// Debug enables logging only if "--verbose" flag passed
func Debug(v ...interface{}) {
	if Settings.Verbose {
		log.Print("\033[32mListener:")
		log.Print(v...)
		log.Println("\033[0m")
	}
}

// ReplayServer returns a connection to the replay server and error if some
func ReplayServer() (conn net.Conn, err error) {
	// Connection to replay server
	conn, err = net.Dial("tcp", Settings.ReplayAddress)

	if err != nil {
		log.Println("Connection error ", err, Settings.ReplayAddress)
	}

	return
}

// Run acts as `main` function of a listener
func Run() {
	if os.Getuid() != 0 {
		fmt.Println("Please start the listener as root or sudo!")
		fmt.Println("This is required since listener sniff traffic on given port.")
		os.Exit(1)
	}

	Settings.Parse()

	fmt.Println("Listening for HTTP traffic on", Settings.Address+":"+strconv.Itoa(Settings.Port))

	var messageLogger *log.Logger

	if Settings.FileToReplayPath != "" {

		file, err := os.OpenFile(Settings.FileToReplayPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0660)
		defer file.Close()

		if err != nil {
			log.Fatal("Cannot open file %q. Error: %s", Settings.FileToReplayPath, err)
		}

		messageLogger = log.New(file, "", 0)
	}

	if messageLogger == nil {
		fmt.Println("Forwarding requests to replay server:", Settings.ReplayAddress, "Limit:", Settings.ReplayLimit)
	} else {
		fmt.Println("Saving requests to file", Settings.FileToReplayPath)
	}

	// Sniffing traffic from given address
	listener := RAWTCPListen(Settings.Address, Settings.Port)

	currentTime := time.Now().UnixNano()
	currentRPS := 0

	for {
		// Receiving TCPMessage object
		m := listener.Receive()

		if Settings.ReplayLimit != 0 {
			if (time.Now().UnixNano() - currentTime) > time.Second.Nanoseconds() {
				currentTime = time.Now().UnixNano()
				currentRPS = 0
			}

			if currentRPS >= Settings.ReplayLimit {
				continue
			}

			currentRPS++
		}

		// HACK: This does not belong here.
		if Settings.GetOnly {
			buf := bytes.NewBuffer(m.Bytes())
			reader := bufio.NewReader(buf)

			request, err := http.ReadRequest(reader)

			if request.Method != "GET" || err != nil {
				continue
			}
		}

		if messageLogger != nil {
			go func() {
				messageBuffer := new(bytes.Buffer)
				messageWriter := bufio.NewWriter(messageBuffer)

				fmt.Fprintf(messageWriter, "%v\n", time.Now().UnixNano())
				fmt.Fprintf(messageWriter, "%s", string(m.Bytes()))

				messageWriter.Flush()
				messageLogger.Println(messageBuffer.String())
			}()
		} else {
			go sendMessage(m)
		}
	}
}

func sendMessage(m *TCPMessage) {
	conn, err := ReplayServer()

	if err != nil {
		log.Println("Failed to send message. Replay server not respond.")
		return
	} else {
		defer conn.Close()
	}

	// For debugging purpose
	// Usually request parsing happens in replay part
	if Settings.Verbose {
		buf := bytes.NewBuffer(m.Bytes())
		reader := bufio.NewReader(buf)

		request, err := http.ReadRequest(reader)

		if err != nil {
			Debug("Error while parsing request:", err, string(m.Bytes()))
		} else {
			request.ParseMultipartForm(32 << 20)
			Debug("Forwarding request:", request)
		}
	}

	_, err = conn.Write(m.Bytes())

	if err != nil {
		log.Println("Error while sending requests", err)
	}
}
