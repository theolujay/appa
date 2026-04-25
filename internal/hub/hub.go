package hub

type Client struct {
	DeploymentID string
	Send         chan string
}

type Hub struct {
	// subscribers maps a deployment ID to the set of clients watching it.
	// The inner map uses the Client pointer as a key, acting as a set.
	subscribers map[string]map[*Client]struct{}
	// These three channels are how the outside world communicate with the hub
	register   chan *Client
	unregister chan *Client
	broadcast  chan Message
}

// What the pipeline sends when it produces a log line.
type Message struct {
	DeploymentID string
	Line         string
}

func New() *Hub {
	return &Hub{
		subscribers: make(map[string]map[*Client]struct{}),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan Message),
	}
}

func NewClient(deploymentID string, send chan string) *Client {
	return &Client{DeploymentID: deploymentID, Send: send}
}

func (h *Hub) Register(c *Client) {
	h.register <- c
}

func (h *Hub) Unregister(c *Client) {
	h.unregister <- c
}

func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			// Ensure the inner set exists for this deployment
			if h.subscribers[c.DeploymentID] == nil {
				h.subscribers[c.DeploymentID] = make(map[*Client]struct{})
			}
			h.subscribers[c.DeploymentID][c] = struct{}{}
		case c := <-h.unregister:
			if clients, ok := h.subscribers[c.DeploymentID]; ok {
				delete(clients, c)
				close(c.Send) // signal the write goroutine to stop
			}
		case msg := <-h.broadcast:
			// Send the log line to every Client subscribed to this deployment.
			for c := range h.subscribers[msg.DeploymentID] {
				// Non-blocking Send -- if the Client's buffer is full,
				// we drop the message rather than block the entire hub.
				select {
				case c.Send <- msg.Line:
				default:
					delete(h.subscribers[msg.DeploymentID], c)
					close(c.Send)
				}
			}
		}
	}
}

func (h *Hub) Publish(DeploymentID, line string) {
	h.broadcast <- Message{DeploymentID: DeploymentID, Line: line}
}
