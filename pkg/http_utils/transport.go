package http_utils

import (
	"crypto/tls"
	"github.com/quic-go/quic-go/http3"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"

	"golang.org/x/net/http2"
	"net"
	"net/http"
	"net/url"
	"time"
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
		// Timeout:   time.Duration(viper.GetInt("navigation.timeout")) * time.Second,
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
