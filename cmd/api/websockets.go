package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/theolujay/appa/internal/data"
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
func (app *application) streamLogsHandler(w http.ResponseWriter, r *http.Request) {
	app.logger.Info("starting streamLogsHandler")
	user := app.contextGetUser(r)

	id, err := app.readIDParam(r)
	if err != nil || id < 1 {
		app.badRequestResponse(w, r, err)
		return
	}

	deployment, err := app.models.Deployments.Get(id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	if deployment.UserID != nil && *deployment.UserID != user.ID {
		app.logger.Warn("user not permitted to view logs", "user_id", user.ID, "deployment_id", id, "owner_id", deployment.UserID)
		app.notPermittedResponse(w, r)
		return
	}

	app.logger.Info("upgrading connection to websocket", "deployment_id", id)
	// HTTP -> WS
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		app.logger.Error("websocket upgrade failed", "error", err)
		return
	}
	app.logger.Info("websocket upgrade successful", "deployment_id", id)

	// Replay all historical logs before switching to live streaming.
	logs, err := app.models.Deployments.GetLogs(id)
	if err != nil {
		app.logger.Error(fmt.Sprintf("failed to fetch historical logs for %d: %v", id, err))
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
	app.logger.Debug("sending historical logs", "count", len(logs), "deployment_id", id)
	for _, l := range logs {
		event := hub.Event{
			Type: hub.MessageTypeLog,
			Log:  hub.LogMessage{ID: l.ID, Line: l.Line},
		}
		if err := conn.WriteJSON(event); err != nil {
			app.logger.Debug("failed to send historical log", "log_id", l.ID, "error", err)
			app.hub.Unregister(c)
			conn.Close()
			return
		}
	}
	app.logger.Debug("historical logs sent successfully", "deployment_id", id)

	app.background(func() {
		app.logger.Debug("starting writePump", "deployment_id", id)
		writePump(conn, send, lastHistoryID)
		app.logger.Debug("writePump finished", "deployment_id", id)
	})
	app.logger.Debug("starting readPump", "deployment_id", id)
	app.readPump(conn, c)
	app.logger.Debug("readPump finished, handler exiting", "deployment_id", id)
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
			if event.Type == hub.MessageTypeLog && event.Log.ID <= lastHistoryID {
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
func (app *application) readPump(conn *websocket.Conn, c *hub.Client) {
	defer func() {
		app.hub.Unregister(c)
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
		_, _, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				app.logger.Debug("websocket read error", "error", err)
			}
			return
		}
	}
}
