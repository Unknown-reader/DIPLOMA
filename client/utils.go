package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"diploma/crdt"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/writer"
)

type Flags struct {
	Server string
	Secure bool
	Login  bool
	File   string
	Debug  bool
	Scroll bool
}

func parseFlags() Flags {
	serverAddr := flag.String("server", "localhost:8080", "The network address of the server")

	useSecureConn := flag.Bool("secure", false, "Enable a secure WebSocket connection (wss://)")

	enableDebug := flag.Bool("debug", false, "Enable debugging mode to show more verbose logs")

	enableLogin := flag.Bool("login", false, "Enable the login prompt for the server")

	file := flag.String("file", "", "The file to load the pairpad content from")

	enableScroll := flag.Bool("scroll", true, "Enable scrolling with the cursor")

	flag.Parse()

	return Flags{
		Server: *serverAddr,
		Secure: *useSecureConn,
		Debug:  *enableDebug,
		Login:  *enableLogin,
		File:   *file,
		Scroll: *enableScroll,
	}
}

func createConn(flags Flags) (*websocket.Conn, *http.Response, error) {
	var u url.URL
	if flags.Secure {
		u = url.URL{Scheme: "wss", Host: flags.Server, Path: "/"}
	} else {
		u = url.URL{Scheme: "ws", Host: flags.Server, Path: "/"}
	}

	// get WebSocket connection
	dialer := websocket.Dialer{
		HandshakeTimeout: 2 * time.Minute,
	}

	return dialer.Dial(u.String(), nil)
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func setupLogger(logger *logrus.Logger) (*os.File, *os.File, error) {
	logPath := "log.log"
	debugLogPath := "log-debug.log"

	// open log files for writing
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) // skipcq: GSC-G302
	if err != nil {
		fmt.Printf("Logger error, exiting: %s\n", err)
		return nil, nil, err
	}

	debugLogFile, err := os.OpenFile(debugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) // skipcq: GSC-G302
	if err != nil {
		fmt.Printf("Logger error, exiting: %s\n", err)
		return nil, nil, err
	}

	// configure logger to discard default output
	logger.SetOutput(io.Discard)
	logger.SetFormatter(&logrus.JSONFormatter{})

	// hook for warnings and errors
	logger.AddHook(&writer.Hook{
		Writer: logFile,
		LogLevels: []logrus.Level{
			logrus.WarnLevel,
			logrus.ErrorLevel,
			logrus.FatalLevel,
			logrus.PanicLevel,
		},
	})

	// hook for debug/info logs
	logger.AddHook(&writer.Hook{
		Writer: debugLogFile,
		LogLevels: []logrus.Level{
			logrus.TraceLevel,
			logrus.DebugLevel,
			logrus.InfoLevel,
		},
	})

	return logFile, debugLogFile, nil
}

func closeLogFiles(logFile, debugLogFile *os.File) {
	if err := logFile.Close(); err != nil {
		fmt.Printf("Failed to close log file: %s", err)
		return
	}

	if err := debugLogFile.Close(); err != nil {
		fmt.Printf("Failed to close debug log file: %s", err)
		return
	}
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func printDoc(doc crdt.Document) {
	if flags.Debug {
		logger.Infof("---DOCUMENT STATE---")
		for i, c := range doc.Characters {
			logger.Infof("index: %v  value: %s  ID: %v  IDPrev: %v  IDNext: %v  ", i, c.Value, c.ID, c.IDPrevious, c.IDNext)
		}
	}
}
