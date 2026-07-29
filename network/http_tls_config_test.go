package network

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPServerTLSConfigAdvertisesHTTP2AndHTTP1(t *testing.T) {
	config := NewHTTPServerTLSConfig(nil)
	if len(config.NextProtos) != 2 || config.NextProtos[0] != "h2" || config.NextProtos[1] != "http/1.1" {
		t.Fatalf("unexpected ALPN protocols: %#v", config.NextProtos)
	}
}

func TestHTTPServerTLSConfigNegotiatesHTTP2(t *testing.T) {
	seed := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	certificate := seed.TLS.Certificates[0]
	seed.Close()

	rawListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen failed: %v", err)
	}
	config := NewHTTPServerTLSConfig(func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
		return &certificate, nil
	})
	server := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}),
		TLSConfig: config.Clone(),
	}
	tlsListener := tls.NewListener(rawListener, config)
	serveDone := make(chan error, 1)
	go func() { serveDone <- server.Serve(tlsListener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
		<-serveDone
	})

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig:   &tls.Config{InsecureSkipVerify: true}, // test-only certificate
		ForceAttemptHTTP2: true,
	}}
	resp, err := client.Get("https://" + rawListener.Addr().String())
	if err != nil {
		t.Fatalf("HTTP/2 request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 2 {
		t.Fatalf("unexpected negotiated protocol: %s", resp.Proto)
	}
}
