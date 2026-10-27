package websocket

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/yegors/co-atc/pkg/logger"
)

// New message types for aircraft streaming
const (
	MessageTypeAircraftAdded          = "aircraft_added"
	MessageTypeAircraftUpdate         = "aircraft_update"
	MessageTypeAircraftRemoved        = "aircraft_removed"
	MessageTypeAircraftPredictedState = "aircraft_predicted_state"
	MessageTypeAircraftBulkRequest    = "aircraft_bulk_request"  // Client requests bulk data
	MessageTypeAircraftBulkResponse   = "aircraft_bulk_response" // Server sends bulk data
	MessageTypeFilterUpdate           = "filter_update"          // Client sends filter preferences
	MessageTypeFrequencyStatus        = "frequency_status"       // Frequency connection status changes
	MessageTypeFrequenciesChanged     = "frequencies_changed"    // A source was added or removed while running
	MessageTypeFrequencyActivity      = "frequency_activity"     // Someone is speaking on a frequency, and the level
)

// Deadlines on the connection. A client that stops reading -- a phone put to
// sleep, a VPN that switches routes -- used to block its writer for as long as
// the kernel kept the TCP connection, with its lock held, and every broadcast,
// the ADS-B loop's included, waited on that lock: about 0.5 MB after it stopped
// reading, measured in a test. Now a write that takes longer than writeWait
// drops the client, and one that answers no ping within pongWait is dropped too.
const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = pongWait / 2
)

// maxMessageBytes bounds one message from a client. gorilla reads a whole
// message into memory and sets no bound of its own: a signed-in client
// could send gigabytes in one. The page sends one kind, a bulk request of
// about 80 bytes (www/store/feed.js); a larger message closes the
// connection (1009, message too big).
const maxMessageBytes = 64 << 10

// Message represents a WebSocket message
type Message struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

// AircraftBulkRequest represents client request for bulk aircraft data
type AircraftBulkRequest struct {
	Filters map[string]interface{} `json:"filters"` // Filter parameters
}

// MessageHandler defines the interface for handling incoming WebSocket messages
type MessageHandler interface {
	HandleMessage(client *Client, messageType string, data map[string]interface{}) error
}

// Client represents a WebSocket client
type Client struct {
	conn      *websocket.Conn
	send      chan *Message
	server    *Server
	mu        sync.Mutex
	closed    bool
	closeChan chan struct{}

	// Who it is and why it left, for the log line written when it goes: a
	// page hears a lost connection (owner, 15/10) and the server used to drop
	// clients without a word.
	remote string
	agent  string
	since  time.Time
	reason string

	// What this client is sent of each message; nil, everything. See
	// HandleConnectionFiltered.
	filter func(*Message) *Message

	// The account it was opened by, "" without sign-in: what CloseTagged
	// closes when the owner cuts that account off.
	tag string
}

// leave records the first reason the client goes; later ones are consequences.
func (c *Client) leave(reason string) {
	c.mu.Lock()
	if c.reason == "" {
		c.reason = reason
	}
	c.mu.Unlock()
}

// sendQueue is a client's queue of messages not yet written. A client whose
// queue is full when a message comes is dropped, not the message.
const sendQueue = 256

// Server represents a WebSocket server
type Server struct {
	clients        map[*Client]bool
	register       chan *Client
	unregister     chan *Client
	broadcast      chan *Message
	upgrader       websocket.Upgrader
	logger         *logger.Logger
	mu             sync.RWMutex
	messageHandler MessageHandler // Handler for incoming messages

	// Which pages may open a connection; see SetOriginCheck.
	originCheck atomic.Pointer[func(*http.Request) bool]
	refusedMu   sync.Mutex
	refused     map[string]bool // origins already logged
}

// NewServer creates a new WebSocket server
func NewServer(logger *logger.Logger) *Server {
	s := &Server{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client, 32),   // Buffered to prevent goroutine blocking
		unregister: make(chan *Client, 32),   // Buffered to prevent goroutine blocking
		broadcast:  make(chan *Message, 512), // Buffered for high-throughput broadcasting
		logger:     logger.Named("web-socket"),
		refused:    map[string]bool{},
	}
	s.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     s.checkOrigin,
	}
	return s
}

// SetOriginCheck says which pages may open a connection. A browser sends the
// session cookie with the handshake whatever page asks -- the same-origin
// policy does not apply to WebSockets -- so until 13/10, when every origin
// was accepted, any page the signed-in user opened could read the aircraft
// in their name. Without a check set, the rule is gorilla's own: no Origin
// (a client that is not a browser), or one naming the host the request came
// to.
func (s *Server) SetOriginCheck(allowed func(*http.Request) bool) {
	s.originCheck.Store(&allowed)
}

func (s *Server) checkOrigin(r *http.Request) bool {
	ok := sameOrigin(r)
	if f := s.originCheck.Load(); f != nil {
		ok = (*f)(r)
	}
	if !ok {
		// Once per origin, bounded: a refused page that keeps reconnecting
		// must not fill the log.
		origin := r.Header.Get("Origin")
		s.refusedMu.Lock()
		first := !s.refused[origin] && len(s.refused) < 64
		if first {
			s.refused[origin] = true
		}
		s.refusedMu.Unlock()
		if first {
			s.logger.Warn("Refused a WebSocket from another origin; add it to server.allowed_origins if it is yours",
				String("origin", origin), String("remote_addr", r.RemoteAddr))
		}
	}
	return ok
}

// sameOrigin is gorilla's default check, which an explicit CheckOrigin replaces.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && strings.EqualFold(u.Host, r.Host)
}

// SetMessageHandler sets the message handler for incoming WebSocket messages
func (s *Server) SetMessageHandler(handler MessageHandler) {
	s.messageHandler = handler
}

// Run starts the WebSocket server
func (s *Server) Run() {
	s.logger.Info("Starting WebSocket server")

	for {
		select {
		case client := <-s.register:
			s.mu.Lock()
			s.clients[client] = true
			clientCount := len(s.clients)
			s.mu.Unlock()
			s.logger.Debug("Client registered", String("client_count", fmt.Sprintf("%d", clientCount)))

		case client := <-s.unregister:
			s.mu.Lock()
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				// Mark client as closed first to prevent new messages
				client.mu.Lock()
				client.closed = true
				client.mu.Unlock()
				// Then close the channel
				close(client.send)
				s.logLeft(client)
			}
			clientCount := len(s.clients)
			s.mu.Unlock()
			s.logger.Debug("Client unregistered", String("client_count", fmt.Sprintf("%d", clientCount)))

		case message := <-s.broadcast:
			s.mu.RLock()
			clientsToRemove := make([]*Client, 0)
			for client := range s.clients {
				// Send to all clients - filtering is done client-side. SendMessage
				// checks and sends under the client's lock: checking first and
				// sending after could send on a channel closed in between.
				if !client.SendMessage(message) {
					clientsToRemove = append(clientsToRemove, client)
				}
			}
			s.mu.RUnlock()

			// Clean up failed clients
			if len(clientsToRemove) > 0 {
				s.mu.Lock()
				for _, client := range clientsToRemove {
					if _, ok := s.clients[client]; ok {
						delete(s.clients, client)
						client.mu.Lock()
						if !client.closed {
							client.closed = true
							close(client.send)
						}
						client.mu.Unlock()
						s.logLeft(client)
					}
				}
				s.mu.Unlock()
			}
		}
	}
}

// HandleConnection handles a WebSocket connection
func (s *Server) HandleConnection(w http.ResponseWriter, r *http.Request) {
	s.HandleConnectionFiltered(w, r, nil)
}

// HandleConnectionFiltered is HandleConnection for a client that is not sent
// everything: each message to it goes through filter first, which returns the
// message to send -- a copy where it changes something, since every client is
// handed the same message -- or nil to send nothing.
func (s *Server) HandleConnectionFiltered(w http.ResponseWriter, r *http.Request, filter func(*Message) *Message) {
	s.HandleConnectionTagged(w, r, filter, "")
}

// HandleConnectionTagged is HandleConnectionFiltered for a client opened by a
// signed-in account, named by tag, so that it can be closed when the owner
// cuts that account off (CloseTagged).
func (s *Server) HandleConnectionTagged(w http.ResponseWriter, r *http.Request, filter func(*Message) *Message, tag string) {
	s.logger.Info("Handling new WebSocket connection request",
		String("remote_addr", r.RemoteAddr),
		String("user_agent", r.UserAgent()))

	// Upgrade HTTP connection to WebSocket
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("Failed to upgrade connection",
			Error(err),
			String("remote_addr", r.RemoteAddr))
		return
	}

	s.logger.Debug("Successfully upgraded connection to WebSocket",
		String("remote_addr", r.RemoteAddr))

	// Create client
	client := &Client{
		conn:      conn,
		send:      make(chan *Message, sendQueue),
		server:    s,
		closeChan: make(chan struct{}),
		remote:    r.RemoteAddr,
		agent:     r.UserAgent(),
		since:     time.Now(),
		filter:    filter,
		tag:       tag,
	}

	// Register client
	s.register <- client

	// Start client goroutines
	go client.readPump()
	go client.writePump()
}

// CloseTagged closes every client opened by the account tag, and says how
// many. Each leaves as any closed connection does: its read fails, and the
// server lets it go.
func (s *Server) CloseTagged(tag string) int {
	if tag == "" {
		return 0
	}
	s.mu.RLock()
	var found []*Client
	for c := range s.clients {
		if c.tag == tag {
			found = append(found, c)
		}
	}
	s.mu.RUnlock()
	for _, c := range found {
		c.leave("access cut by the owner")
		c.Close()
	}
	return len(found)
}

// logLeft writes why a client went, once, when the server lets it go.
func (s *Server) logLeft(c *Client) {
	c.mu.Lock()
	reason := c.reason
	c.mu.Unlock()
	if reason == "" {
		reason = "unknown"
	}
	s.logger.Info("WebSocket client left",
		String("reason", reason),
		String("after", time.Since(c.since).Round(time.Second).String()),
		String("remote_addr", c.remote),
		String("user_agent", c.agent))
}

// Broadcast sends a message to all connected clients
func (s *Server) Broadcast(message *Message) {
	//s.logger.Debug("Broadcasting message to all clients",
	//	String("message_type", message.Type),
	//	String("client_count", fmt.Sprintf("%d", len(s.clients))))

	// Aircraft movement updates should be dispatched immediately (no server-side queueing)
	if message.Type == MessageTypeAircraftAdded || message.Type == MessageTypeAircraftUpdate || message.Type == MessageTypeAircraftRemoved || message.Type == MessageTypeAircraftPredictedState {
		s.broadcastImmediate(message)
		return
	}

	s.broadcast <- message
}

// broadcastImmediate sends a message directly to all clients without using the broadcast queue.
func (s *Server) broadcastImmediate(message *Message) {
	s.mu.RLock()
	clientsToRemove := make([]*Client, 0)
	for client := range s.clients {
		client.mu.Lock()
		if client.closed {
			clientsToRemove = append(clientsToRemove, client)
			client.mu.Unlock()
			continue
		}
		client.mu.Unlock()

		if !client.SendMessage(message) {
			clientsToRemove = append(clientsToRemove, client)
		}
	}
	s.mu.RUnlock()

	if len(clientsToRemove) > 0 {
		s.mu.Lock()
		for _, client := range clientsToRemove {
			if _, ok := s.clients[client]; ok {
				delete(s.clients, client)
				client.mu.Lock()
				if !client.closed {
					client.closed = true
					close(client.send)
				}
				client.mu.Unlock()
				s.logLeft(client)
			}
		}
		s.mu.Unlock()
	}
}

// readPump pumps messages from the WebSocket connection to the hub
func (c *Client) readPump() {
	defer func() {
		c.mu.Lock()
		if !c.closed {
			c.closed = true
		}
		c.mu.Unlock()

		c.server.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageBytes)
	// Browsers answer pings by themselves, even in a background tab.
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		// Check if client is closed
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			break
		}
		c.mu.Unlock()

		// Read message
		_, messageBytes, err := c.conn.ReadMessage()
		if err != nil {
			var ne net.Error
			var ce *websocket.CloseError
			switch {
			case errors.As(err, &ce):
				// 1001 going away, 1005 no status: a page reloaded or closed.
				c.leave(fmt.Sprintf("the page closed it (%d)", ce.Code))
			case errors.As(err, &ne) && ne.Timeout():
				c.leave(fmt.Sprintf("no answer to pings for %s", pongWait))
			default:
				c.leave("reading failed: " + err.Error())
			}
			// 1005, no status: what a browser sends when the page is reloaded or closed.
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure,
				websocket.CloseNormalClosure, websocket.CloseNoStatusReceived) {
				c.server.logger.Error("WebSocket read error", Error(err))
			}
			break
		}

		// Parse incoming message
		var message struct {
			Type string                 `json:"type"`
			Data map[string]interface{} `json:"data"`
		}

		if err := json.Unmarshal(messageBytes, &message); err != nil {
			c.server.logger.Error("Failed to parse WebSocket message", Error(err))
			continue
		}

		c.server.logger.Debug("Received WebSocket message",
			String("type", message.Type),
			String("client", c.conn.RemoteAddr().String()))

		// Handle message if handler is set
		if c.server.messageHandler != nil {
			if err := c.server.messageHandler.HandleMessage(c, message.Type, message.Data); err != nil {
				c.server.logger.Error("Failed to handle WebSocket message",
					Error(err),
					String("type", message.Type))
			}
		}
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
//
// It is the connection's only writer, and it never holds the client's lock
// while writing: the lock guards the closed flag and the send channel, which
// broadcasts take for every client, and a write can wait on the network.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.mu.Lock()
		if !c.closed {
			c.closed = true
		}
		c.mu.Unlock()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Channel closed
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.mu.Lock()
			closed := c.closed
			c.mu.Unlock()
			if closed {
				return
			}

			// Marshal message to JSON
			data, err := json.Marshal(message)
			if err != nil {
				c.server.logger.Error("Failed to marshal message", Error(err))
				continue
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				c.leave(fmt.Sprintf("a write failed (deadline %s): %v", writeWait, err))
				return
			}

		case <-ticker.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				c.leave(fmt.Sprintf("a ping could not be written (deadline %s): %v", writeWait, err))
				return
			}

		case <-c.closeChan:
			return
		}
	}
}

// Close closes the client connection
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	close(c.closeChan)
	c.conn.Close()
}

// SendMessage sends a message to this specific client
func (c *Client) SendMessage(message *Message) bool {
	if c.filter != nil {
		if message = c.filter(message); message == nil {
			return true
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if client is closed
	if c.closed {
		return false
	}

	// Try to send message with non-blocking select
	select {
	case c.send <- message:
		return true
	default:
		// The queue is full: the client did not read fast enough, and the
		// caller drops it.
		if c.reason == "" {
			c.reason = fmt.Sprintf("its queue was full (%d messages not yet written): it did not read fast enough", sendQueue)
		}
		return false
	}
}

// Import logger functions
var (
	String = logger.String
	Error  = logger.Error
)
