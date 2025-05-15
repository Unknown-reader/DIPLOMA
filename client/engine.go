package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"diploma/commons"

	"diploma/crdt"

	"diploma/client/editor"

	"github.com/gorilla/websocket"
	"github.com/nsf/termbox-go"
	"github.com/sirupsen/logrus"
)

const (
	OperationInsert = iota
	OperationDelete
)

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func getTermboxChan() chan termbox.Event {
	termboxChan := make(chan termbox.Event)

	go func() {
		for {
			termboxChan <- termbox.PollEvent()
		}
	}()

	return termboxChan
}

func getMsgChan(conn *websocket.Conn) chan commons.Message {
	messageChan := make(chan commons.Message)
	go func() {
		for {
			var msg commons.Message

			err := conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logger.Errorf("websocket error: %v", err)
				}
				e.IsConnected = false
				e.StatusChan <- "lost connection!"
				break
			}

			logger.Infof("message received: %+v\n", msg)

			messageChan <- msg

		}
	}()
	return messageChan
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func handleTermboxEvent(ev termbox.Event, conn *websocket.Conn) error {
	if ev.Type == termbox.EventKey {
		switch ev.Key {

		// exit session
		case termbox.KeyEsc, termbox.KeyCtrlC:
			// Return an error with the prefix "pairpad", so that it gets treated as an exit "event".
			return errors.New("pairpad: exiting")

		// save file contents
		case termbox.KeyCtrlS:
			if fileName == "" {
				fileName = "content.txt"
			}

			err := crdt.Save(fileName, &doc)
			if err != nil {
				logrus.Errorf("Failed to save to %s", fileName)
				e.StatusChan <- fmt.Sprintf("Failed to save to %s", fileName)
				return err
			}

			e.StatusChan <- fmt.Sprintf("Saved document to %s", fileName)

		// The default key for loading content from a file is Ctrl+L.
		/* case termbox.KeyCtrlL:
		if fileName != "" {
			logger.Log(logrus.InfoLevel, "LOADING DOCUMENT")
			newDoc, err := crdt.Load(fileName)
			if err != nil {
				logrus.Errorf("failed to load file %s", fileName)
				e.StatusChan <- fmt.Sprintf("Failed to load %s", fileName)
				return err
			}
			e.StatusChan <- fmt.Sprintf("Loading %s", fileName)
			doc = newDoc
			e.SetX(0)
			e.SetText(crdt.Content(doc))

			logger.Log(logrus.InfoLevel, "SENDING DOCUMENT")
			docMsg := commons.Message{Type: commons.DocSyncMessage, Document: doc}
			_ = conn.WriteJSON(&docMsg)
		} else {
			e.StatusChan <- "No file to load!"
		} */

		// move cursor
		case termbox.KeyArrowLeft, termbox.KeyCtrlB:
			e.MoveCursor(-1, 0)

		case termbox.KeyArrowRight, termbox.KeyCtrlF:
			e.MoveCursor(1, 0)

		case termbox.KeyArrowUp, termbox.KeyCtrlP:
			e.MoveCursor(0, -1)

		case termbox.KeyArrowDown, termbox.KeyCtrlN:
			e.MoveCursor(0, 1)

		// Home key
		case termbox.KeyHome:
			e.SetX(0)

		// End key
		case termbox.KeyEnd:
			e.SetX(len(e.Text))

		// delete symbol
		case termbox.KeyBackspace, termbox.KeyBackspace2:
			performOperation(OperationDelete, ev, conn)
		case termbox.KeyDelete:
			performOperation(OperationDelete, ev, conn)

		// Tab key
		case termbox.KeyTab:
			for i := 0; i < 4; i++ {
				ev.Ch = ' '
				performOperation(OperationInsert, ev, conn)
			}

		// Enter key
		case termbox.KeyEnter:
			ev.Ch = '\n'
			performOperation(OperationInsert, ev, conn)

		// Space key
		case termbox.KeySpace:
			ev.Ch = ' '
			performOperation(OperationInsert, ev, conn)

		// insert symbol
		default:
			if ev.Ch != 0 {
				performOperation(OperationInsert, ev, conn)
			}
		}
	}

	e.SendDraw()
	return nil
}

func handleMsg(msg commons.Message, conn *websocket.Conn) {
	switch msg.Type {
	// recieve current doc
	case commons.DocSyncMessage:
		logger.Infof("DOCSYNC RECEIVED, updating local doc %+v\n", msg.Document)
		doc = msg.Document
		e.SetText(crdt.Content(doc))

	// send current doc
	case commons.DocReqMessage:
		logger.Infof("DOCREQ RECEIVED, sending local document to %v\n", msg.ID)
		docMsg := commons.Message{Type: commons.DocSyncMessage, Document: doc, ID: msg.ID}
		_ = conn.WriteJSON(&docMsg)

	// recieve unique ID
	case commons.SiteIDMessage:
		siteID, err := strconv.Atoi(msg.Text)
		if err != nil {
			logger.Errorf("failed to set siteID, err: %v\n", err)
		}
		crdt.SiteID = siteID
		logger.Infof("SITE ID %v, INTENDED SITE ID: %v", crdt.SiteID, siteID)

	// recieve new user info message
	case commons.JoinMessage:
		e.StatusChan <- fmt.Sprintf("%s has joined the session!", msg.Username)

	// recieve list of current users
	case commons.UsersMessage:
		e.StatusMu.Lock()
		e.Users = strings.Split(msg.Text, ",")
		e.StatusMu.Unlock()

	default:
		switch msg.Operation.Type {
		// recieve insert from other user
		case "insert":
			_, err := doc.Insert(msg.Operation.Position, msg.Operation.Value)
			if err != nil {
				logger.Errorf("failed to insert, err: %v\n", err)
			}

			e.SetText(crdt.Content(doc))
			if msg.Operation.Position-1 <= e.Cursor {
				e.MoveCursor(len(msg.Operation.Value), 0)
			}
			logger.Infof("REMOTE INSERT: %s at position %v\n", msg.Operation.Value, msg.Operation.Position)

			color := editor.GetColorForUsername(msg.Username, e.Users)
			e.UsersPos[msg.Username] = editor.CursorColPos{Pos: msg.Operation.Position - 1, Col: color}
			for name, user := range e.UsersPos {
				if name != msg.Username && msg.Operation.Position < user.Pos {
					e.UsersPos[name] = editor.CursorColPos{Pos: user.Pos + 1, Col: user.Col}
				}
			}

		// recieve delete from other user
		case "delete":
			_ = doc.Delete(msg.Operation.Position)
			e.SetText(crdt.Content(doc))
			if msg.Operation.Position-1 <= e.Cursor {
				e.MoveCursor(-len(msg.Operation.Value), 0)
			}
			logger.Infof("REMOTE DELETE: position %v\n", msg.Operation.Position)

			color := editor.GetColorForUsername(msg.Username, e.Users)
			e.UsersPos[msg.Username] = editor.CursorColPos{Pos: msg.Operation.Position - 2, Col: color}
			for name, user := range e.UsersPos {
				if name != msg.Username && msg.Operation.Position < user.Pos {
					e.UsersPos[name] = editor.CursorColPos{Pos: user.Pos - 1, Col: user.Col}
				}
			}
		}
	}

	printDoc(doc)
	e.SendDraw()
}

func handleStatusMsg() {
	for msg := range e.StatusChan {
		// write message to StatusBar
		e.StatusMu.Lock()
		e.StatusMsg = msg
		e.ShowMsg = true
		e.StatusMu.Unlock()

		logger.Infof("got status message: %s", e.StatusMsg)

		e.SendDraw()
		time.Sleep(6 * time.Second)

		// write base StatusBar back
		e.StatusMu.Lock()
		e.ShowMsg = false
		e.StatusMu.Unlock()

		e.SendDraw()
	}

}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func performOperation(opType int, ev termbox.Event, conn *websocket.Conn) {
	ch := string(ev.Ch)

	var msg commons.Message

	switch opType {
	case OperationInsert:
		logger.Infof("LOCAL INSERT: %s at cursor position %v\n", ch, e.Cursor)

		text, err := doc.Insert(e.Cursor+1, ch)
		if err != nil {
			e.SetText(text)
			logger.Errorf("CRDT error: %v\n", err)
		}
		e.SetText(text)

		e.MoveCursor(1, 0)
		msg = commons.Message{Username: e.Username, Type: "operation", Operation: commons.Operation{Type: "insert", Position: e.Cursor, Value: ch}}

		for name, user := range e.UsersPos {
			if name != e.Username && e.Cursor < user.Pos {
				e.UsersPos[name] = editor.CursorColPos{Pos: user.Pos + 1, Col: user.Col}
			}
		}

	case OperationDelete:
		logger.Infof("LOCAL DELETE: cursor position %v\n", e.Cursor)

		if e.Cursor-1 < 0 {
			e.Cursor = 0
		}

		text := doc.Delete(e.Cursor)
		e.SetText(text)

		for name, user := range e.UsersPos {
			if name != e.Username && e.Cursor < user.Pos {
				e.UsersPos[name] = editor.CursorColPos{Pos: user.Pos - 1, Col: user.Col}
			}
		}

		msg = commons.Message{Username: e.Username, Type: "operation", Operation: commons.Operation{Type: "delete", Position: e.Cursor}}
		e.MoveCursor(-1, 0)
	}

	if e.IsConnected {
		err := conn.WriteJSON(msg)
		if err != nil {
			e.IsConnected = false
			e.StatusChan <- "lost connection!"
		}
	}
}

func drawLoop() {
	for {
		<-e.DrawChan
		e.Draw()
	}
}
