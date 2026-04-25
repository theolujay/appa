package api

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/theolujay/appa/internal/hub"
)

const (
	// How long to wait for the client to respond to a ping before
	// declaring the connection dead and closing it.
	pongWait = 60 * time.Second
	// How often to send a ping to the client to keep the connection alive.
	// Must be less than pongWait so there's time to detect a missed pong.
	pingPeriod = (pongWait * 9) / 10
	// Maximum size of message to read from client.
	// Shouldn't be large, but a limit is needed.
	maxMessageSize = 512
)

// converts a plain HTTP connection into a Websocket connection.
// CheckOrigin always return true here (for now, since there's no auth) -
// ideally, check Origin header to prevent cross-site Websocket hijacking.
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// upgrades the conn to WS, replays historical logs for the given deployment
// then streams live log lines until client disconnects or pipeline finishes
func (h *Handler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		http.Error(w, "missing deployment id", http.StatusBadRequest)
		return
	}
	// HTTP -> WS
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	// Create a buffered channel for this client. Buffer size of 256 means
	// the hub can queue up to 256 unsent log lines before considering the
	// cient too slow and dropping it
	send := make(chan string, 256)
	// Register this client with the hub so it starts receiving live log lines.
	c := hub.NewClient(id, send)
	h.hub.Register(c)
	// Replay all historical log lines from the database before switching to
	// live streaming.
	logs, err := h.store.GetLogs(id)
	if err != nil {
		log.Printf("failed to fetch historical logs for %s: %v", id, err)
	}
	for _, line := range logs {
		send <- line
	}
	// Run the write pump in a goroutine and the read pump in this goroutine.
	// We keep the read pump on the main goroutine because it is the one that
	// drives connections lifecycle -- when it returns, we know the client is gone
	go writePump(conn, send)
	readPump(conn, send, h.hub, c)
}

// drains the send channel and writes each line to the WebSocket.
// It also sends periodic pings to detect dead connections.
func writePump(conn *websocket.Conn, send <-chan string) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop() // this isn't necessary (Go 1.23+), but call it anyway to be sure
		conn.Close()
	}()

	for {
		select {
		case line, ok := <-send:
			if !ok {
				// The hub closed the channel -- the deployment is done or the
				// client was removed. Send close frame and exit.
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(line)); err != nil {
				return
			}
		case <-ticker.C:
			// Send a ping. If the client doesn't respond with a pong within pongWait,
			// the read pump will detect the deadline exceeded an exit.
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// reads from the WebSocket to handle pongs and detect disconnection.
// When the clinet disconnects, it unregisters them from the hub and returns
func readPump(conn *websocket.Conn, send chan string, h *hub.Hub, c *hub.Client) {
	defer func() {
		h.Unregister(c)
		conn.Close()
	}()

	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))

	// reset the deadline every time we receive a pong... this is the heartbeat
	// mechanism. No pong within pongWait means the client is gone.
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Block here, reading indefinitely. We discard any messages the client sends,
	// as we only care about the pong responses and connection errors.
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
