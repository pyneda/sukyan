package http_utils

import (
	"crypto/tls"

	"github.com/quic-go/quic-go/http3"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"net"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/http2"
)

func getProxyFunc() func(*http.Request) (*url.URL, error) {
	proxy := viper.GetString("navigation.proxy")
	if proxy == "" {
		return http.ProxyFromEnvironment
	}
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		log.Error().Err(err).Str("proxy", proxy).Msg("Error parsing proxy url, using environment proxy")
		return http.ProxyFromEnvironment
	}
	return http.ProxyURL(proxyURL)
}

// CreateHttpTransport creates an HTTP transport with no pre-defined http version.
func CreateHttpTransport() *http.Transport {
	transport := &http.Transport{
		Proxy: getProxyFunc(),
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       100,
		DisableKeepAlives:     false,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			Renegotiation:      tls.RenegotiateOnceAsClient,
			InsecureSkipVerify: true,
		},
	}
	return transport
}

// CreateHttp2Transport creates an HTTP/2 transport.
func CreateHttp2Transport() *http2.Transport {
	return &http2.Transport{
		// Ensure the connection uses only HTTP/2 without falling back.
		AllowHTTP: false,
		DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
			if cfg == nil {
				cfg = &tls.Config{}
			}
			cfg.NextProtos = []string{"h2"} // Enforce HTTP/2.0
			return tls.DialWithDialer(&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}, network, addr, cfg)
		},
		TLSClientConfig: &tls.Config{
			Renegotiation:      tls.RenegotiateOnceAsClient,
			InsecureSkipVerify: true,
		},
	}
}

// CreateHttpClient creates a regular HTTP client.
func CreateHttpClient() *http.Client {
	transport := CreateHttpTransport()
	client := &http.Client{
		Transport: transport,
	}
	return client
}

// CreateHttp2Client creates an HTTP/2 client.
func CreateHttp2Client() *http.Client {
	transport := CreateHttp2Transport()
	client := &http.Client{
		Transport: transport,
	}
	return client
}

// CreateHttp3Transport creates an HTTP/3 transport.
func CreateHttp3Transport() *http3.RoundTripper {
	return &http3.RoundTripper{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
		DisableCompression: false,
		EnableDatagrams:    true,
	}
}

// CreateHttp3Client creates an HTTP/3 client.
func CreateHttp3Client() *http.Client {
	transport := CreateHttp3Transport()
	return &http.Client{
		Transport: transport,
	}
}

type HTTPClientConfig struct {
	Timeout             *int
	MaxIdleConns        *int
	MaxIdleConnsPerHost *int
	MaxConnsPerHost     *int
	DisableKeepAlives   *bool
}

func CreateHTTPClientFromConfig(cfg HTTPClientConfig) *http.Client {
	transport := CreateHttpTransport()

	if cfg.MaxIdleConns != nil {
		transport.MaxIdleConns = *cfg.MaxIdleConns
	} else {
		transport.MaxIdleConns = viper.GetInt("http.client.max_idle_conns")
	}

	if cfg.MaxIdleConnsPerHost != nil {
		transport.MaxIdleConnsPerHost = *cfg.MaxIdleConnsPerHost
	} else {
		transport.MaxIdleConnsPerHost = viper.GetInt("http.client.max_idle_conns_per_host")
	}

	if cfg.MaxConnsPerHost != nil {
		transport.MaxConnsPerHost = *cfg.MaxConnsPerHost
	} else {
		transport.MaxConnsPerHost = viper.GetInt("http.client.max_conns_per_host")
	}

	if cfg.DisableKeepAlives != nil {
		transport.DisableKeepAlives = *cfg.DisableKeepAlives
	} else {
		transport.DisableKeepAlives = viper.GetBool("http.client.disable_keep_alives")
	}

	timeout := viper.GetDuration("http.client.timeout")
	if cfg.Timeout != nil {
		timeout = time.Duration(*cfg.Timeout) * time.Second
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}
