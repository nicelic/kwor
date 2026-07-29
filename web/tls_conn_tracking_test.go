package web

import (
	"crypto/tls"
	"net"
	"net/http"
	"testing"

	"github.com/alireza0/s-ui/network"
	"github.com/alireza0/s-ui/service"
)

func TestTrackTLSConnUnwrapsOuterTLSConnection(t *testing.T) {
	server := NewServer()
	server.tlsGeneration.Store(7)
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()

	managed := network.NewManagedTLSConn(left)
	autoHTTPS := network.NewAutoHttpsConn(managed)
	outer := tls.Server(autoHTTPS, &tls.Config{})

	server.trackTLSConn(outer, http.StateNew)
	if managed.Generation() != 7 {
		t.Fatalf("managed generation mismatch: got=%d want=7", managed.Generation())
	}
	if _, ok := server.tlsConns[managed]; !ok {
		t.Fatal("managed connection was not tracked")
	}

	server.bindTLSSelection(managed, service.PanelCertificateBalanceSelection{
		ListenerKey:         "listener|panel|9443",
		SNIBucket:           "certset:11,22",
		CertificateRecordID: 11,
	})
	server.trackTLSConn(outer, http.StateClosed)
	if _, ok := server.tlsConns[managed]; ok {
		t.Fatal("closed managed connection remained tracked")
	}
	if _, ok := server.tlsSelections[managed]; ok {
		t.Fatal("closed managed connection retained its certificate selection")
	}
}
