package http_utils

import (
	"crypto/tls"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
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

func CreateHttpTransport() *http.Transport {
	transport := &http.Transport{
		Proxy: getProxyFunc(),
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
			DualStack: true,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	return transport
}

func CreateHttpClient() *http.Client {
	transport := CreateHttpTransport()
	client := &http.Client{
		Transport: transport,
		// Timeout:   time.Duration(viper.GetInt("navigation.timeout")) * time.Second,
	}
	return client
}
