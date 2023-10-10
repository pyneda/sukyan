package proxy

import (
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"
	"net/http"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type WSMessage struct {
	Action   string             `json:"action"`
	Modified InterceptedRequest `json:"request"`
	Actual   *http.Request      `json:"-"`
}

func (p *Proxy) InterceptEndpoint(w http.ResponseWriter, r *http.Request) {
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upgrade to WebSocket")
		return
	}
	defer ws.Close()

	for {
		// Wait for a request from the proxy interception
		req := <-p.ReqChannel

		// Convert the request to InterceptedRequest
		interceptedReq, err := ConvertToInterceptedRequest(req)
		log.Info().Str("url", interceptedReq.URL).Str("method", interceptedReq.Method).Msg("Got an intercepted request")
		if err != nil {
			log.Error().Err(err).Msg("Failed to convert to InterceptedRequest")
			continue
		}

		// Send that intercepted request to the client through the WebSocket
		err = ws.WriteJSON(interceptedReq)
		if err != nil {
			log.Error().Err(err).Msg("Failed to send request to WebSocket")
			continue
		}
		log.Info().Str("url", interceptedReq.URL).Str("method", interceptedReq.Method).Msg("Sent an intercepted request to client")

		// Read the decision (and potentially a modified request) from the WebSocket client
		var msg WSMessage
		err = ws.ReadJSON(&msg)
		if err != nil {
			log.Error().Err(err).Msg("Failed to read message from WebSocket")
			continue
		}

		if msg.Action == "forward" && msg.Modified.URL != "" && msg.Modified.Method != "" {
			log.Info().Str("action", msg.Action).Str("url", msg.Modified.URL).Str("method", msg.Modified.Method).Msg("Got an intercept result from client")
			actualReq, err := ConvertFromInterceptedRequest(&msg.Modified)
			if err != nil {
				log.Error().Err(err).Msg("Failed to convert from InterceptedRequest")
				continue
			}
			msg.Actual = actualReq
		}

		// Send the decision back to the proxy interception for processing
		p.DecisionChannel <- msg
	}
}
