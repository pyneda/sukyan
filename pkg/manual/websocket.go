package manual

import (
	"bytes"
	"embed"
	"encoding/json"
	"text/template"

	"github.com/pyneda/sukyan/db"
)

//go:embed templates/*
var templates embed.FS

type cswhTemplateData struct {
	URL            string
	RequestHeaders string
	InteractionURL string
	Messages       []db.WebSocketMessage
}

// GenerateCrossSiteWebsocketHijackingPoC generates a PoC for CSWH
func GenerateCrossSiteWebsocketHijackingPoC(connection db.WebSocketConnection, interactionURL string) (bytes.Buffer, error) {
	var buf bytes.Buffer

	requestHeaders, err := json.Marshal(connection.RequestHeaders)
	if err != nil {
		return buf, err
	}

	tmpl, err := template.New("cswh.html").Funcs(template.FuncMap{
		"js": func(s string) string {
			b, _ := json.Marshal(s)
			return string(b)
		},
	}).ParseFS(templates, "templates/cswh.html")
	if err != nil {
		return buf, err
	}

	data := cswhTemplateData{
		URL:            connection.URL,
		RequestHeaders: string(requestHeaders),
		InteractionURL: interactionURL,
		Messages:       connection.Messages,
	}

	err = tmpl.Execute(&buf, data)
	if err != nil {
		return buf, err
	}

	return buf, nil
}
