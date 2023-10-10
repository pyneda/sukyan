package proxy

import (
	"fmt"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"net/http"

	"github.com/elazarl/goproxy"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"io/ioutil"
)

// Proxy represents configuration for a proxy
type Proxy struct {
	Host                  string
	Port                  int
	Verbose               bool
	LogOutOfScopeRequests bool
	WorkspaceID           uint
	Intercept             bool
	ReqChannel            chan *http.Request
	DecisionChannel       chan WSMessage
}

func (p *Proxy) Run() {
	err := p.SetCA()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to set CA")
		return
	}
	listenAddress := fmt.Sprintf("%s:%d", p.Host, p.Port)
	apiAddress := fmt.Sprintf("%s:%d", p.Host, p.Port+1)
	log.Info().Str("address", listenAddress).Uint("workspace", p.WorkspaceID).Msg("Proxy starting up")
	mux := http.NewServeMux()
	mux.HandleFunc("/intercept/api", p.InterceptEndpoint)

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = p.Verbose

	p.ReqChannel = make(chan *http.Request)
	p.DecisionChannel = make(chan WSMessage)
	proxy.OnRequest().HandleConnect(goproxy.AlwaysMitm)
	proxy.OnRequest(goproxy.DstHostIs("sukyan")).DoFunc(
		func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			if r.URL.Path == "/ca" {
				caCertPath := viper.GetString("server.caCert.file")
				caCert, err := ioutil.ReadFile(caCertPath)
				if err != nil {
					log.Error().Err(err).Msg("Could not read CA certificate")
					return nil, goproxy.NewResponse(r, "application/octet-stream", http.StatusInternalServerError, "Internal Server Error")
				}
				resp := goproxy.NewResponse(r, "application/octet-stream", http.StatusOK, string(caCert))
				resp.Header.Set("Content-Disposition", `attachment; filename="sukyan-proxy-ca.pem"`)
				return nil, resp

			}

			if r.URL.Path == "/intercept" {
				resp := goproxy.NewResponse(r, goproxy.ContentTypeHtml, http.StatusOK, generateProxyInterceptHtml(apiAddress))
				return nil, resp
			}
			return nil, goproxy.NewResponse(r, goproxy.ContentTypeHtml, http.StatusOK, proxyHomepageHtml)
		},
	)

	proxy.OnRequest().DoFunc(
		func(r *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			log.Info().Str("url", r.URL.String()).Msg("Proxy received request")
			if p.Intercept {
				log.Info().Str("url", r.URL.String()).Msg("Proxy intercepting request")
				p.ReqChannel <- r

				decision := <-p.DecisionChannel
				if decision.Action == "drop" {
					// Drop the request
					return r, goproxy.NewResponse(r, goproxy.ContentTypeText, http.StatusForbidden, "Request dropped by user")
				}

				// If decision is forward, use the actual request (if provided)
				if decision.Actual != nil {
					r = decision.Actual
				}
			}

			// r.Header.Set("X-sukyan", "yxorPoG-X")
			log.Info().Msg("Proxy sending request")
			return r, nil
		})
	proxy.OnResponse().DoFunc(
		func(resp *http.Response, ctx *goproxy.ProxyCtx) *http.Response {
			if resp == nil {
				return nil
			}
			log.Info().Str("url", resp.Request.URL.String()).Msg("Proxy received response")
			http_utils.ReadHttpResponseAndCreateHistory(ctx.Resp, "Proxy", p.WorkspaceID, 0, true)
			return resp
		},
	)

	mux.Handle("/", proxy)
	// if err := http.ListenAndServe(listenAddress, proxy); err != nil {
	// 	log.Fatal().Err(err).Msg("Proxy startup failed")
	// }
	go func() {
		log.Info().Str("address", listenAddress).Uint("workspace", p.WorkspaceID).Msg("Proxy starting up")
		if err := http.ListenAndServe(listenAddress, proxy); err != nil {
			log.Fatal().Err(err).Msg("Proxy startup failed")
		}
	}()

	log.Info().Str("address", apiAddress).Uint("workspace", p.WorkspaceID).Msg("WebSocket server starting up")
	if err := http.ListenAndServe(apiAddress, mux); err != nil {
		log.Fatal().Err(err).Msg("WebSocket server startup failed")
	}
}
