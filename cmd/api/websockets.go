package main

import (
	"log"
	"net/http"
	"strconv"
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
func (app *application) StreamLogs(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id < 1 {
		http.Error(w, "invalid deployment id", http.StatusBadRequest)
		return
	}

	// HTTP -> WS
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	// Replay all historical logs before switching to live streaming.
	logs, err := app.models.Deployments.GetLogs(id)
	if err != nil {
		log.Printf("failed to fetch historical logs for %d: %v", id, err)
	}

	var lastHistoryID int64
	if len(logs) > 0 {
		lastHistoryID = logs[len(logs)-1].ID
	}

	// Register for live logs by creating a buffered channel for this client.
	// Buffer size of 256 means the hub can queue up to 256 unsent log lines
	// before considering the cient too slow and dropping it
	send := make(chan hub.Event, 256)
	c := hub.NewClient(id, send)
	app.hub.Register(c)
	// Send history to client as "log" events
	for _, l := range logs {
		event := hub.Event{
			Type: hub.MessageTypeLog,
			Log:  &hub.LogMessage{ID: l.ID, Line: l.Line},
		}
		if err := conn.WriteJSON(event); err != nil {
			app.hub.Unregister(c)
			conn.Close()
			return
		}
	}

	go writePump(conn, send, lastHistoryID)
	readPump(conn, app.hub, c)
}

// drains the send channel and writes each line to the WebSocket.
// It also sends periodic pings to detect dead connections.
func writePump(conn *websocket.Conn, send <-chan hub.Event, lastHistoryID int64) {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop() // this isn't necessary (in Go 1.23+), but call it anyway to be sure
		conn.Close()
	}()

	for {
		select {
		case event, ok := <-send:
			if !ok {
				conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			// Skip logs already sent as part of history
			if event.Type == hub.MessageTypeLog && event.Log != nil && event.Log.ID <= lastHistoryID {
				continue
			}
			if err := conn.WriteJSON(event); err != nil {
				return
			}
		case <-ticker.C:
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// reads from the WebSocket to handle pongs and detect disconnection.
// When the clinet disconnects, it unregisters them from the hub and returns
func readPump(conn *websocket.Conn, app *hub.Hub, c *hub.Client) {
	defer func() {
		app.Unregister(c)
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

	// Discard any messages the client sends
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}
