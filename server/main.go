package main

import (
	"flag"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"diploma/commons"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

type client struct {
	Conn     *websocket.Conn
	SiteID   string
	id       uuid.UUID
	Username string

	writeMu sync.Mutex
	mu      sync.Mutex
}

type Clients struct {
	list map[uuid.UUID]*client

	mu sync.RWMutex

	deleteRequests     chan deleteRequest
	readRequests       chan readRequest
	addRequests        chan *client
	nameUpdateRequests chan nameUpdate
}

func NewClients() *Clients {
	return &Clients{
		list:               make(map[uuid.UUID]*client),
		mu:                 sync.RWMutex{},
		deleteRequests:     make(chan deleteRequest),
		readRequests:       make(chan readRequest, 10000),
		addRequests:        make(chan *client),
		nameUpdateRequests: make(chan nameUpdate),
	}
}

type deleteRequest struct {
	id   uuid.UUID
	done chan int
}

type readRequest struct {
	readAll bool
	id      uuid.UUID
	resp    chan *client
}

type nameUpdate struct {
	id      uuid.UUID
	newName string
}

var (
	siteID = 0
	mu     sync.Mutex

	upgrader = websocket.Upgrader{}

	messageChan = make(chan commons.Message)
	syncChan    = make(chan commons.Message)

	clients = NewClients()
)

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func handleConn(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		color.Red("Error upgrading connection to websocket: %v\n", err)
		conn.Close()
		return
	}
	defer conn.Close()

	clientID := uuid.New()

	// assign uuid
	mu.Lock()
	siteID++
	client := &client{
		Conn:    conn,
		SiteID:  strconv.Itoa(siteID),
		id:      clientID,
		writeMu: sync.Mutex{},
		mu:      sync.Mutex{},
	}
	mu.Unlock()

	// add new user to server's clients list
	clients.add(client)

	// send client his unique ID
	siteIDMsg := commons.Message{
		Type: commons.SiteIDMessage,
		Text: client.SiteID,
		ID:   clientID}
	clients.broadcastOne(siteIDMsg, clientID)

	// ask other users to provide document
	docReq := commons.Message{
		Type: commons.DocReqMessage,
		ID:   clientID}
	clients.broadcastOneExcept(docReq, clientID)

	// send new list of users
	clients.sendUsernames()

	for {
		var msg commons.Message

		// read message
		if err := client.read(&msg); err != nil {
			color.Red("Failed to read message. closing client connection with %s. Error: %s", client.Username, err)
			return
		}

		// sync message
		if msg.Type == commons.DocSyncMessage {
			syncChan <- msg
			continue
		}

		// join or operation message
		msg.ID = clientID
		messageChan <- msg
	}
}

func handleMsg() {
	for {
		msg := <-messageChan

		// get time and log message to server's stdout
		t := time.Now().Format(time.ANSIC)
		if msg.Type == commons.JoinMessage {
			clients.updateName(msg.ID, msg.Username)
			color.Green("%s >> %s %s (ID: %s)\n", t, msg.Username, msg.Text, msg.ID)
			clients.sendUsernames()
		} else if msg.Type == "operation" {
			color.Green("operation >> %+v from ID=%s\n", msg.Operation, msg.ID)
		} else {
			color.Green("%s >> unknown message type:  %v\n", t, msg)
			clients.sendUsernames()
			continue
		}

		clients.broadcastAllExcept(msg, msg.ID)
	}
}

func handleSync() {
	for {
		syncMsg := <-syncChan
		switch syncMsg.Type {
		case commons.DocSyncMessage:
			clients.broadcastOne(syncMsg, syncMsg.ID)

		case commons.UsersMessage:
			color.Blue("usernames: %s", syncMsg.Text)
			clients.broadcastAll(syncMsg)
		}
	}
}

func (c *Clients) handle() {
	for {
		select {
		case req := <-c.deleteRequests:
			c.close(req.id)
			req.done <- 1
			close(req.done)

		case req := <-c.readRequests:
			if req.readAll {
				for _, client := range c.list {
					req.resp <- client
				}
				close(req.resp)
			} else {
				req.resp <- c.list[req.id]
				close(req.resp)
			}

		case client := <-c.addRequests:
			c.mu.Lock()
			c.list[client.id] = client
			c.mu.Unlock()

		case n := <-c.nameUpdateRequests:
			c.list[n.id].mu.Lock()
			c.list[n.id].Username = n.newName
			c.list[n.id].mu.Unlock()
		}
	}
}

func (c *Clients) close(id uuid.UUID) {
	c.mu.RLock()
	client, ok := c.list[id]
	if ok {
		if err := client.Conn.Close(); err != nil {
			color.Red("Error closing connection: %s\n", err)
		}
	} else {
		color.Red("Couldn't close connection: client not in list")
		return
	}
	color.Red("Removing %v from client list.\n", c.list[id].Username)
	c.mu.RUnlock()

	c.mu.Lock()
	delete(c.list, id)
	c.mu.Unlock()
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func (c *Clients) getAll() chan *client {
	c.mu.RLock()
	resp := make(chan *client, len(c.list))
	c.mu.RUnlock()
	c.readRequests <- readRequest{readAll: true, resp: resp}
	return resp
}

func (c *Clients) get(id uuid.UUID) chan *client {
	resp := make(chan *client)

	c.readRequests <- readRequest{readAll: false, id: id, resp: resp}
	return resp
}

func (c *Clients) add(client *client) {
	c.addRequests <- client
}

func (c *Clients) delete(id uuid.UUID) {
	req := deleteRequest{id, make(chan int)}
	c.deleteRequests <- req
	<-req.done
	c.sendUsernames()
}

func (c *Clients) updateName(id uuid.UUID, newName string) {
	c.nameUpdateRequests <- nameUpdate{id, newName}
}

func (c *Clients) sendUsernames() {
	var users string
	for client := range c.getAll() {
		users += client.Username + ","
	}

	syncChan <- commons.Message{Text: users, Type: commons.UsersMessage}
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func (c *Clients) broadcastAll(msg commons.Message) {
	color.Blue("sending message to all users. Text: %s", msg.Text)
	for client := range c.getAll() {
		if err := client.send(msg); err != nil {
			color.Red("ERROR: %s", err)
			c.delete(client.id)
		}
	}
}

func (c *Clients) broadcastAllExcept(msg commons.Message, except uuid.UUID) {
	for client := range c.getAll() {
		if client.id == except {
			continue
		}
		if err := client.send(msg); err != nil {
			color.Red("ERROR: %s", err)
			c.delete(client.id)
		}
	}
}

func (c *Clients) broadcastOne(msg commons.Message, dst uuid.UUID) {
	client := <-c.get(dst)
	if err := client.send(msg); err != nil {
		color.Red("ERROR: %s", err)
		c.delete(client.id)
	}
}

func (c *Clients) broadcastOneExcept(msg commons.Message, except uuid.UUID) {
	for client := range c.getAll() {
		if client.id == except {
			continue
		}
		if err := client.send(msg); err != nil {
			color.Red("ERROR: %s", err)
			c.delete(client.id)
			continue
		}
		break
	}
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func (c *client) read(msg *commons.Message) error {
	err := c.Conn.ReadJSON(msg)

	c.mu.Lock()
	name := c.Username
	c.mu.Unlock()

	if err != nil {
		if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			color.Red("Failed to read message from client %s: %v", name, err)
		}
		color.Red("client %v disconnected", name)
		clients.delete(c.id)
		return err
	}
	return nil
}

func (c *client) send(v interface{}) error {
	c.writeMu.Lock()
	err := c.Conn.WriteJSON(v)
	c.writeMu.Unlock()
	return err
}

// ////////////////////////////////////////////////////////////////////
// ////////////////////////////////////////////////////////////////////
func main() {
	addr := flag.String("addr", ":8080", "Server's network address")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleConn)

	go clients.handle()
	go handleMsg()
	go handleSync()

	server := &http.Server{
		Addr:         *addr,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		Handler:      mux,
	}

	log.Printf("Starting server on %s", *addr)
	err := server.ListenAndServe()
	if err != nil {
		log.Fatal("Error starting server, exiting.", err)
	}
}
