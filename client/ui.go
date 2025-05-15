package main

import (
	"diploma/client/editor"

	"diploma/crdt"

	"github.com/gorilla/websocket"
	"github.com/nsf/termbox-go"
)

type UIConfig struct {
	EditorConfig editor.EditorConfig
}

func mainLoop(conn *websocket.Conn) error {
	termboxChan := getTermboxChan()
	msgChan := getMsgChan(conn)

	for {
		select {
		case termboxEvent := <-termboxChan:
			err := handleTermboxEvent(termboxEvent, conn)
			if err != nil {
				return err
			}
		case msg := <-msgChan:
			handleMsg(msg, conn)
		}
	}
}

func initUI(conn *websocket.Conn, conf UIConfig) error {
	err := termbox.Init()
	if err != nil {
		return err
	}
	defer termbox.Close()

	e = editor.NewEditor(conf.EditorConfig)
	e.SetSize(termbox.Size())
	e.SetText(crdt.Content(doc))
	e.SendDraw()
	e.IsConnected = true

	go handleStatusMsg()

	go drawLoop()

	err = mainLoop(conn)
	if err != nil {
		return err
	}

	return nil
}
