package network

import "crypto/tls"

func NewHTTPServerTLSConfig(getCertificate func(*tls.ClientHelloInfo) (*tls.Certificate, error)) *tls.Config {
	return &tls.Config{
		GetCertificate: getCertificate,
		NextProtos:     []string{"h2", "http/1.1"},
	}
}
