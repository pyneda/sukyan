package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/pkg/http_utils"

	"crypto/tls"
	"crypto/x509"

	"github.com/elazarl/goproxy"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"gorm.io/datatypes"
)

// Proxy represents configuration for a proxy
type Proxy struct {
	Host                  string
	Port                  int
	Verbose               bool
	LogOutOfScopeRequests bool
	WorkspaceID           uint
	wsConnections         sync.Map
}

type WebSocketConnectionInfo struct {
	Connection *db.WebSocketConnection
	Created    time.Time
}

func setCA(caCert, caKey []byte) error {
	goproxyCa, err := tls.X509KeyPair(caCert, caKey)
	if err != nil {
		return err
	}
	if goproxyCa.Leaf, err = x509.ParseCertificate(goproxyCa.Certificate[0]); err != nil {
		return err
	}
	goproxy.GoproxyCa = goproxyCa
	goproxy.OkConnect = &goproxy.ConnectAction{Action: goproxy.ConnectAccept, TLSConfig: goproxy.TLSConfigFromCA(&goproxyCa)}
	goproxy.MitmConnect = &goproxy.ConnectAction{Action: goproxy.ConnectMitm, TLSConfig: goproxy.TLSConfigFromCA(&goproxyCa)}
	return nil
}

func (p *Proxy) SetCA() error {
	certPath := viper.GetString("server.cert.file")
	keyPath := viper.GetString("server.key.file")
	caCertPath := viper.GetString("server.caCert.file")
	caKeyPath := viper.GetString("server.caKey.file")

	_, _, err := lib.EnsureCertificatesExist(certPath, keyPath, caCertPath, caKeyPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load or generate certificates")
	}

	// Load CA certificate and key
	caCert, err := os.ReadFile(caCertPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read CA certificate")
	}

	caKey, err := os.ReadFile(caKeyPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read CA key")
	}
	return setCA(caCert, caKey)
}

func (p *Proxy) Run() {
	err := p.SetCA()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to set CA")
		return
	}
	listenAddress := fmt.Sprintf("%s:%d", p.Host, p.Port)
	log.Info().Str("address", listenAddress).Uint("workspace", p.WorkspaceID).Msg("Proxy starting up")
	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = p.Verbose

	proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)
	proxy.OnRequest(goproxy.DstHostIs("sukyan")).DoFunc(
		func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			if r.URL.Path == "/ca" {
				caCertPath := viper.GetString("server.caCert.file")
				caCert, err := os.ReadFile(caCertPath)
				if err != nil {
					log.Error().Err(err).Msg("Could not read CA certificate")
					return nil, goproxy.NewResponse(r, "application/octet-stream", http.StatusInternalServerError, "Internal Server Error")
				}
				resp := goproxy.NewResponse(r, "application/octet-stream", http.StatusOK, string(caCert))
				resp.Header.Set("Content-Disposition", `attachment; filename="sukyan-proxy-ca.pem"`)
				return nil, resp

			}
			return nil, goproxy.NewResponse(r, goproxy.ContentTypeHtml, http.StatusOK, proxyHomepageHtml)
		},
	)

	proxy.OnRequest().DoFunc(
		func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			log.Info().Msg("Proxy sending request")
			
			if headerContains(r.Header, "Connection", "Upgrade") && headerContains(r.Header, "Upgrade", "websocket") {
				if connInfo, exists := p.wsConnections.Load(r.URL.String()); exists {
					if wsConnInfo, ok := connInfo.(*WebSocketConnectionInfo); ok {
						interceptor := NewWebSocketInterceptor(wsConnInfo.Connection, p.WorkspaceID)
						ctx.UserData = interceptor
						log.Info().Str("url", r.URL.String()).Msg("WebSocket interceptor set for connection")
					}
				}
			}
			
			return r, nil
		})
	proxy.OnResponse().DoFunc(
		func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
			if resp == nil {
				return nil
			}
			log.Info().Str("url", resp.Request.URL.String()).Msg("Proxy received response")
			isWebSocketUpgrade := resp.StatusCode == http.StatusSwitchingProtocols && 
				headerContains(resp.Header, "Connection", "Upgrade") &&
				headerContains(resp.Header, "Upgrade", "websocket")

			options := http_utils.HistoryCreationOptions{
				Source:              db.SourceProxy,
				WorkspaceID:         p.WorkspaceID,
				TaskID:              0,
				CreateNewBodyStream: true,
				IsWebSocketUpgrade:  isWebSocketUpgrade,
			}

			history, err := http_utils.ReadHttpResponseAndCreateHistory(ctx.Resp, options)
			if err != nil {
				log.Error().Err(err).Msg("Failed to create history record")
			} else if isWebSocketUpgrade {
				go p.createWebSocketConnection(resp, history)
			}
			return resp
		},
	)
	if err := http.ListenAndServe(listenAddress, proxy); err != nil {
		log.Fatal().Err(err).Msg("Proxy startup failed")
	}
}

func headerContains(header http.Header, name string, value string) bool {
	for _, v := range header[name] {
		for _, s := range strings.Split(v, ",") {
			if strings.EqualFold(value, strings.TrimSpace(s)) {
				return true
			}
		}
	}
	return false
}

func (p *Proxy) createWebSocketConnection(resp *http.Response, history *db.History) {
	requestHeaders, _ := json.Marshal(resp.Request.Header)
	responseHeaders, _ := json.Marshal(resp.Header)

	connection := &db.WebSocketConnection{
		URL:              resp.Request.URL.String(),
		RequestHeaders:   datatypes.JSON(requestHeaders),
		ResponseHeaders:  datatypes.JSON(responseHeaders),
		StatusCode:       resp.StatusCode,
		StatusText:       resp.Status,
		WorkspaceID:      &p.WorkspaceID,
		Source:           db.SourceProxy,
		UpgradeRequestID: &history.ID,
	}

	err := db.Connection().CreateWebSocketConnection(connection)
	if err != nil {
		log.Error().Uint("workspace", p.WorkspaceID).Err(err).Str("url", connection.URL).Msg("Failed to create WebSocket connection")
		return
	}
	
	connInfo := &WebSocketConnectionInfo{
		Connection: connection,
		Created:    time.Now(),
	}
	p.wsConnections.Store(resp.Request.URL.String(), connInfo)
	
	log.Info().Uint("workspace", p.WorkspaceID).Str("url", connection.URL).Uint("id", connection.ID).Msg("Created WebSocket connection")
}
