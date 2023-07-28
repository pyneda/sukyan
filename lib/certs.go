package lib

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"github.com/spf13/viper"
	"io/ioutil"
	"math/big"
	"net"
	"os"
	"time"
)

func EnsureCertificatesExist(certPath, keyPath, caCertPath, caKeyPath string) (*tls.Config, *tls.Config, error) {
	_, err := os.Stat(certPath)
	certExist := !os.IsNotExist(err)

	_, err = os.Stat(keyPath)
	keyExist := !os.IsNotExist(err)

	var serverTLSConf, clientTLSConf *tls.Config

	if !certExist || !keyExist {
		organization := viper.GetString("server.cert.organization")
		country := viper.GetString("server.cert.country")
		locality := viper.GetString("server.cert.locality")
		streetAddress := viper.GetString("server.cert.street_address")
		postalCode := viper.GetString("server.cert.postal_code")

		serverTLSConf, clientTLSConf, err = GenerateCertificates(certPath, keyPath, caCertPath, caKeyPath, organization, country, locality, streetAddress, postalCode)
		if err != nil {
			return nil, nil, err
		}

	} else {
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, nil, err
		}

		caCert, err := ioutil.ReadFile(caCertPath)
		if err != nil {
			return nil, nil, err
		}

		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(caCert)

		serverTLSConf = &tls.Config{
			Certificates: []tls.Certificate{cert},
		}

		clientTLSConf = &tls.Config{
			RootCAs: caCertPool,
		}
	}

	return serverTLSConf, clientTLSConf, nil
}

func GenerateCertificates(certPath, keyPath, caCertPath, caKeyPath, organization, country, locality, streetAddress, postalCode string) (*tls.Config, *tls.Config, error) {
	// set up our CA certificate
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(2019),
		Subject: pkix.Name{
			Organization:  []string{organization},
			Country:       []string{country},
			Province:      []string{""},
			Locality:      []string{locality},
			StreetAddress: []string{streetAddress},
			PostalCode:    []string{postalCode},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	// create our private and public key
	caPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}

	// create the CA
	caBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &caPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	// pem encode
	caPEM := new(bytes.Buffer)
	pem.Encode(caPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: caBytes,
	})

	caPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(caPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(caPrivKey),
	})

	// set up our server certificate
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(2019),
		Subject: pkix.Name{
			Organization:  []string{organization},
			Country:       []string{country},
			Province:      []string{""},
			Locality:      []string{locality},
			StreetAddress: []string{streetAddress},
			PostalCode:    []string{postalCode},
		},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().AddDate(10, 0, 0),
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca, &certPrivKey.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, err
	}

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	certPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})

	err = ioutil.WriteFile(certPath, certPEM.Bytes(), 0600)
	if err != nil {
		return nil, nil, err
	}

	err = ioutil.WriteFile(keyPath, certPrivKeyPEM.Bytes(), 0600)
	if err != nil {
		return nil, nil, err
	}

	err = ioutil.WriteFile(caCertPath, caPEM.Bytes(), 0600)
	if err != nil {
		return nil, nil, err
	}

	err = ioutil.WriteFile(caKeyPath, caPrivKeyPEM.Bytes(), 0600)
	if err != nil {
		return nil, nil, err
	}

	// Create *tls.Config for server and client
	serverCert, err := tls.X509KeyPair(certPEM.Bytes(), certPrivKeyPEM.Bytes())
	if err != nil {
		return nil, nil, err
	}

	serverTLSConf := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caPEM.Bytes())

	clientTLSConf := &tls.Config{
		RootCAs: caCertPool,
	}

	return serverTLSConf, clientTLSConf, nil
}
