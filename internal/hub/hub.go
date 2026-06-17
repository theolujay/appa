// Package hub implements a WebSocket broadcast hub that fans out
// deployment log and status events to all connected clients for a
// given deployment. It exposes Client, Event, and publish methods
// (PublishLog, PublishStatus). The hub uses the single-goroutine
// ownership pattern to avoid mutex contention. All state mutations
// go through the run loop via channels.
package hub

type MessageType string

const (
	MessageTypeLog    MessageType = "log"
	MessageTypeStatus MessageType = "status"
)

type LogMessage struct {
	ID   int64  `json:"id"`
	Line string `json:"line"`
}

type StatusUpdate struct {
	Status string `json:"status"`
	URL    string `json:"url,omitempty"`
}

type Event struct {
	Type   MessageType  `json:"type"`
	Log    LogMessage   `json:"log,omitempty"`
	Status StatusUpdate `json:"status,omitempty"`
}

type Client struct {
	DeploymentID int64
	Send         chan Event
}

type Hub struct {
	subscribers map[int64]map[*Client]struct{}
	register    chan *Client
	unregister  chan *Client
	broadcast   chan InternalMessage
	shutdown    chan struct{}
}

type InternalMessage struct {
	DeploymentID int64
	Event        Event
}

// New returns an initialized Hub ready to have clients register and
// receive published events. The broadcast channel is buffered to
// avoid blocking publishers.
func New() *Hub {
	return &Hub{
		subscribers: make(map[int64]map[*Client]struct{}),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan InternalMessage, 256),
		shutdown:    make(chan struct{}),
	}
}

// NewClient creates a new Client bound to a specific deployment ID
// with the provided send channel. The client can then be registered
// with the Hub to receive events for that deployment.
func NewClient(deploymentID int64, send chan Event) *Client {
	return &Client{DeploymentID: deploymentID, Send: send}
}

// Register enqueues a client to be added to the hub's subscriber
// list. Registration is processed by the hub's single goroutine in
// Run, avoiding the need for external synchronization.
func (h *Hub) Register(c *Client) {
	h.register <- c
}

// Unregister requests removal of a client from the hub's subscriber
// list. If the hub is already shutting down the request is dropped.
func (h *Hub) Unregister(c *Client) {
	select {
	case h.unregister <- c:
	case <-h.shutdown:
	}
}

// Stop initiates a graceful shutdown of the hub by closing the
// shutdown channel. The Run loop observes this and will close all
// client send channels before returning.
func (h *Hub) Stop() {
	close(h.shutdown)
}

// Run is the hub's main event loop and must be run in a single
// goroutine. It serializes all state mutations (registering,
// unregistering, and broadcasting) and is responsible for closing
// client channels on shutdown or when a client cannot receive
// messages.
func (h *Hub) Run() {
	for {
		select {
		case <-h.shutdown:
			for _, clients := range h.subscribers {
				for c := range clients {
					close(c.Send)
				}
			}
			return
		case c := <-h.register:
			if h.subscribers[c.DeploymentID] == nil {
				h.subscribers[c.DeploymentID] = make(map[*Client]struct{})
			}
			h.subscribers[c.DeploymentID][c] = struct{}{}
		case c := <-h.unregister:
			if clients, ok := h.subscribers[c.DeploymentID]; ok {
				delete(clients, c)
				close(c.Send)
			}
		case msg := <-h.broadcast:
			for c := range h.subscribers[msg.DeploymentID] {
				select {
				case c.Send <- msg.Event:
				default:
					delete(h.subscribers[msg.DeploymentID], c)
					close(c.Send)
				}
			}
		}
	}
}

// PublishLog broadcasts a log Event for the specified deployment to
// all subscribed clients. This call enqueues the message on the
// internal broadcast channel and returns immediately (subject to
// channel buffering/backpressure).
func (h *Hub) PublishLog(deploymentID int64, log LogMessage) {
	h.broadcast <- InternalMessage{
		DeploymentID: deploymentID,
		Event: Event{
			Type: MessageTypeLog,
			Log:  log,
		},
	}
}

// PublishStatus broadcasts a status update Event for the specified
// deployment to all subscribed clients. The URL field is optional
// and may be empty.
func (h *Hub) PublishStatus(deploymentID int64, status string, url string) {
	h.broadcast <- InternalMessage{
		DeploymentID: deploymentID,
		Event: Event{
			Type: MessageTypeStatus,
			Status: StatusUpdate{
				Status: status,
				URL:    url,
			},
		},
	}
}
