package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"github.com/elazarl/goproxy"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"io/ioutil"
)

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
	caCert, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read CA certificate")
	}

	caKey, err := ioutil.ReadFile(caKeyPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to read CA key")
	}
	return setCA(caCert, caKey)
}
