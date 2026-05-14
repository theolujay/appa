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
	Type   MessageType   `json:"type"`
	Log    *LogMessage   `json:"log,omitempty"`
	Status *StatusUpdate `json:"status,omitempty"`
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
}

type InternalMessage struct {
	DeploymentID int64
	Event        Event
}

func New() *Hub {
	return &Hub{
		subscribers: make(map[int64]map[*Client]struct{}),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		broadcast:   make(chan InternalMessage),
	}
}

func NewClient(deploymentID int64, send chan Event) *Client {
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

func (h *Hub) PublishLog(deploymentID int64, log LogMessage) {
	h.broadcast <- InternalMessage{
		DeploymentID: deploymentID,
		Event: Event{
			Type: MessageTypeLog,
			Log:  &log,
		},
	}
}

func (h *Hub) PublishStatus(deploymentID int64, status string, url string) {
	h.broadcast <- InternalMessage{
		DeploymentID: deploymentID,
		Event: Event{
			Type: MessageTypeStatus,
			Status: &StatusUpdate{
				Status: status,
				URL:    url,
			},
		},
	}
}
