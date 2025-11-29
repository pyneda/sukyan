package proxy

import (
	"encoding/json"
	"fmt"
	"io"
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

	// Use AlwaysAutoMitm to auto-detect TLS vs plain HTTP for CONNECT requests
	proxy.OnRequest().HandleConnect(goproxy.AlwaysAutoMitm)
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
			// Strip permessage-deflate to force uncompressed WebSocket frames
			// This avoids the need for complex stateful decompression in the proxy
			if headerContains(r.Header, "Connection", "Upgrade") &&
				headerContains(r.Header, "Upgrade", "websocket") {
				extensions := r.Header.Get("Sec-WebSocket-Extensions")
				if strings.Contains(strings.ToLower(extensions), "permessage-deflate") {
					// Simple approach: remove the header entirely if it only contains permessage-deflate
					// or just remove the string. For now, removing the header is safest as it's usually the only extension.
					// A more robust approach would be to split by comma and filter.
					var newExtensions []string
					for _, ext := range strings.Split(extensions, ",") {
						if !strings.Contains(strings.ToLower(ext), "permessage-deflate") {
							newExtensions = append(newExtensions, strings.TrimSpace(ext))
						}
					}
					if len(newExtensions) == 0 {
						r.Header.Del("Sec-WebSocket-Extensions")
					} else {
						r.Header.Set("Sec-WebSocket-Extensions", strings.Join(newExtensions, ", "))
					}
					log.Info().Str("original_extensions", extensions).Msg("Stripped permessage-deflate from WebSocket extensions")
				}
			}
			return r, nil
		})
	proxy.OnResponse().DoFunc(
		func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
			if resp == nil {
				return nil
			}
			log.Info().Str("url", resp.Request.URL.String()).Int("status", resp.StatusCode).Msg("Proxy received response")

			// Strip permessage-deflate from response as well to ensure client consistency
			extensions := resp.Header.Get("Sec-WebSocket-Extensions")
			if strings.Contains(strings.ToLower(extensions), "permessage-deflate") {
				var newExtensions []string
				for _, ext := range strings.Split(extensions, ",") {
					if !strings.Contains(strings.ToLower(ext), "permessage-deflate") {
						newExtensions = append(newExtensions, strings.TrimSpace(ext))
					}
				}
				if len(newExtensions) == 0 {
					resp.Header.Del("Sec-WebSocket-Extensions")
				} else {
					resp.Header.Set("Sec-WebSocket-Extensions", strings.Join(newExtensions, ", "))
				}
				log.Info().Str("original_extensions", extensions).Msg("Stripped permessage-deflate from WebSocket response extensions")
			}

			isWebSocketUpgrade := resp.StatusCode == http.StatusSwitchingProtocols &&
				headerContains(resp.Header, "Connection", "Upgrade") &&
				headerContains(resp.Header, "Upgrade", "websocket")

			// Log WebSocket upgrade detection details
			if headerContains(resp.Request.Header, "Upgrade", "websocket") {
				log.Info().
					Int("status", resp.StatusCode).
					Bool("is_upgrade", isWebSocketUpgrade).
					Str("connection_header", resp.Header.Get("Connection")).
					Str("upgrade_header", resp.Header.Get("Upgrade")).
					Str("url", resp.Request.URL.String()).
					Msg("WebSocket upgrade request detected")
			}

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
				// Create the WebSocket connection and set up the interceptor before proxyWebsocket runs
				connection := p.createWebSocketConnection(resp, history)
				if connection != nil {
					// Check if permessage-deflate compression is negotiated
					// The header value may include parameters like "permessage-deflate; client_no_context_takeover"
					wsExtensions := resp.Header.Get("Sec-WebSocket-Extensions")
					compressionEnabled := strings.Contains(strings.ToLower(wsExtensions), "permessage-deflate")
					log.Debug().
						Str("extensions", wsExtensions).
						Bool("compression_enabled", compressionEnabled).
						Msg("WebSocket extensions detected")
					interceptor := NewWebSocketInterceptor(connection, p.WorkspaceID, compressionEnabled)
					ctx.UserData = interceptor
					// Set the WebSocket close handler to clean up the interceptor when done
					ctx.WebSocketCloseHandler = func(_ *goproxy.ProxyCtx) {
						log.Debug().Uint("connection_id", connection.ID).Msg("WebSocket connection closed, cleaning up interceptor")
						interceptor.Close()
					}
					// Set the WebSocket copy handler to intercept all WebSocket traffic
					ctx.WebSocketCopyHandler = func(dst io.Writer, src io.Reader, direction goproxy.WebSocketDirection, _ *goproxy.ProxyCtx) (int64, error) {
						log.Debug().Int("direction", int(direction)).Msg("WebSocketCopyHandler called")
						var msgDirection db.MessageDirection
						if direction == goproxy.WebSocketClientToServer {
							msgDirection = db.MessageSent
						} else {
							msgDirection = db.MessageReceived
						}
						// Call the InterceptedCopy method of the interceptor
						written, err := interceptor.InterceptedCopy(dst, src, msgDirection)
						if err != nil {
							log.Error().Err(err).Int("direction", int(direction)).Msg("Error during WebSocket InterceptedCopy")
						}
						log.Debug().Int64("bytes_copied", written).Int("direction", int(direction)).Msg("WebSocketCopyHandler finished copying")
						return written, err
					}
					log.Info().Str("url", resp.Request.URL.String()).Uint("id", connection.ID).Bool("compression", compressionEnabled).Msg("WebSocket interceptor set for connection")
				}
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

func (p *Proxy) createWebSocketConnection(resp *http.Response, history *db.History) *db.WebSocketConnection {
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
		return nil
	}

	connInfo := &WebSocketConnectionInfo{
		Connection: connection,
		Created:    time.Now(),
	}
	p.wsConnections.Store(resp.Request.URL.String(), connInfo)

	log.Info().Uint("workspace", p.WorkspaceID).Str("url", connection.URL).Uint("id", connection.ID).Msg("Created WebSocket connection")
	return connection
}
