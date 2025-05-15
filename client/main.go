package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"diploma/client/editor"

	"diploma/commons"
	"diploma/crdt"

	"github.com/Pallinder/go-randomdata"
	"github.com/sirupsen/logrus"
)

var (
	doc      = crdt.New()
	logger   = logrus.New()
	e        *editor.Editor
	fileName string
	flags    Flags
)

func main() {
	flags = parseFlags()
	s := bufio.NewScanner(os.Stdin)

	var name string
	if flags.Login {
		fmt.Print("Enter your name: ")
		s.Scan()
		name = s.Text()
	} else {
		name = randomdata.SillyName()
	}

	conn, _, err := createConn(flags)
	if err != nil {
		fmt.Printf("Connection error, exiting: %s\n", err)
		return
	}
	defer conn.Close()

	msg := commons.Message{Username: name, Text: "has joined the session.", Type: commons.JoinMessage}
	_ = conn.WriteJSON(msg)

	logFile, debugLogFile, err := setupLogger(logger)
	if err != nil {
		fmt.Printf("Failed to setup logger, exiting: %s\n", err)
		return
	}
	defer closeLogFiles(logFile, debugLogFile)

	if flags.File != "" {
		if doc, err = crdt.Load(flags.File); err != nil {
			fmt.Printf("failed to load document: %s\n", err)
			return
		}
	}

	uiConfig := UIConfig{
		EditorConfig: editor.EditorConfig{
			ScrollEnabled: flags.Scroll,
			Username:      name,
		},
	}

	err = initUI(conn, uiConfig)
	if err != nil {
		// If error has the prefix "pairpad", then it was triggered by an event that wasn't an error, for example, exiting the editor.
		// It's a hacky solution since the UI returns an error only.
		if strings.HasPrefix(err.Error(), "pairpad") {
			fmt.Println("exiting session.")
			return
		}

		// This is printed when it's an actual error.
		fmt.Printf("TUI error, exiting: %s\n", err)
		return
	}
}
